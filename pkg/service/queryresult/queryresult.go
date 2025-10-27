package queryresult

import (
	"fmt"
	"io"
	"iter"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	multihash "github.com/multiformats/go-multihash/core"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/bytemap"
	"github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/ipld/codec/cbor"
	"github.com/storacha/go-ucanto/core/ipld/hash/sha256"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/go-ucanto/validator"
	qdm "github.com/storacha/indexing-service/pkg/service/queryresult/datamodel"
	"github.com/storacha/indexing-service/pkg/types"
)

type queryResult struct {
	root ipld.Block
	data *qdm.QueryResultModel0_1
	blks blockstore.BlockReader
}

var _ types.QueryResult = (*queryResult)(nil)

func (q *queryResult) Blocks() iter.Seq2[block.Block, error] {
	return q.blks.Iterator()
}

func (q *queryResult) Claims() []datamodel.Link {
	return q.data.Claims
}

func (q *queryResult) Indexes() []datamodel.Link {
	var indexes []ipld.Link
	if q.data.Indexes != nil {
		for _, k := range q.data.Indexes.Keys {
			l, ok := q.data.Indexes.Values[k]
			if ok {
				indexes = append(indexes, l)
			}
		}
	}
	return indexes
}

func (q *queryResult) Root() block.Block {
	return q.root
}

func Extract(r io.Reader) (types.QueryResult, error) {
	roots, blocks, err := car.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("extracting car: %w", err)
	}

	if len(roots) != 1 {
		return nil, types.ErrWrongRootCount
	}

	blks, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(blocks))
	if err != nil {
		return nil, fmt.Errorf("reading blocks from car: %w", err)
	}
	root, has, err := blks.Get(roots[0])
	if err != nil {
		return nil, fmt.Errorf("reading root block: %w", err)
	}
	if !has {
		return nil, types.ErrNoRootBlock
	}

	var queryResultModel qdm.QueryResultModel
	err = block.Decode(root, &queryResultModel, qdm.QueryResultType(), cbor.Codec, sha256.Hasher)
	if err != nil {
		return nil, fmt.Errorf("decoding query result: %w", err)
	}
	return &queryResult{root, queryResultModel.Result0_1, blks}, nil
}

// Build generates a new encodable QueryResult
func Build(claims map[cid.Cid]delegation.Delegation, indexes bytemap.ByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView]) (types.QueryResult, error) {
	bs, err := blockstore.NewBlockStore()
	if err != nil {
		return nil, err
	}

	cls := []ipld.Link{}
	for _, claim := range claims {
		cls = append(cls, claim.Link())

		err := blockstore.WriteInto(claim, bs)
		if err != nil {
			return nil, err
		}
	}

	var indexesModel *qdm.IndexesModel
	if indexes.Size() > 0 {
		indexesModel = &qdm.IndexesModel{
			Keys:   make([]string, 0, indexes.Size()),
			Values: make(map[string]ipld.Link, indexes.Size()),
		}
		for contextID, index := range indexes.Iterator() {
			reader, err := index.Archive()
			if err != nil {
				return nil, err
			}
			bytes, err := io.ReadAll(reader)
			if err != nil {
				return nil, err
			}
			indexCid, err := cid.Prefix{
				Version:  1,
				Codec:    uint64(multicodec.Car),
				MhType:   multihash.SHA2_256,
				MhLength: -1,
			}.Sum(bytes)
			if err != nil {
				return nil, err
			}

			lnk := cidlink.Link{Cid: indexCid}
			err = bs.Put(block.NewBlock(lnk, bytes))
			if err != nil {
				return nil, err
			}
			indexesModel.Keys = append(indexesModel.Keys, string(contextID))
			indexesModel.Values[string(contextID)] = lnk
		}
	}

	queryResultModel := qdm.QueryResultModel{
		Result0_1: &qdm.QueryResultModel0_1{
			Claims:  cls,
			Indexes: indexesModel,
		},
	}

	rt, err := block.Encode(
		&queryResultModel,
		qdm.QueryResultType(),
		cbor.Codec,
		sha256.Hasher,
	)
	if err != nil {
		return nil, err
	}

	err = bs.Put(rt)
	if err != nil {
		return nil, err
	}

	return &queryResult{root: rt, data: queryResultModel.Result0_1, blks: bs}, nil
}

// BuildCompressed returns a QueryResult that, when there is a matching index entry for the
// targetMh, replaces the full index with a single location claim for the targetMh
func BuildCompressed(targetMh mh.Multihash, principal ucan.Signer, claims map[cid.Cid]delegation.Delegation, indexes bytemap.ByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView]) (types.QueryResult, error) {

	// our goal here is to remove indexes from the query result if there are any
	// if there are no indexes, we can just build the regular query result
	if indexes.Size() == 0 {
		return Build(claims, indexes)
	}

	for _, index := range indexes.Iterator() {
		for _, shard := range index.Shards().Iterator() {
			if shard.Has(targetMh) {
				pos := shard.Get(targetMh)
				hasLocation := false
				var locClaim assert.LocationCaveats
				var expiration *ucan.UTCUnixTimestamp
				for _, claim := range claims {
					match, err := assert.Location.Match(validator.NewSource(claim.Capabilities()[0], claim))
					if err != nil {
						continue
					}
					hasLocation = true

					locClaim = match.Value().Nb()
					expiration = claim.Expiration()
				}
				if !hasLocation {
					continue
				}

				newCaveats := assert.LocationCaveats{
					Content:  ctypes.FromHash(targetMh),
					Location: locClaim.Location,
					Range:    &assert.Range{Offset: locClaim.Range.Offset + pos.Length, Length: &pos.Length},
					Space:    locClaim.Space,
				}
				var opts = []delegation.Option{}
				if expiration != nil {
					opts = append(opts, delegation.WithExpiration(*expiration))
				}

				claim, err := assert.Location.Delegate(
					principal,
					principal,
					principal.DID().String(),
					newCaveats,
					opts...,
				)
				if err != nil {
					return nil, fmt.Errorf("delegating compressed location claim: %w", err)
				}

				newClaims := make(map[cid.Cid]delegation.Delegation)
				newClaims[claim.Link().(cidlink.Link).Cid] = claim
				return Build(newClaims, bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1))
			}
		}
	}
	// never found the MH in any index shard, just build the regular query result
	return Build(claims, indexes)
}
