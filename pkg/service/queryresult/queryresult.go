package queryresult

import (
	"fmt"
	"io"
	"iter"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multicodec"
	multihash "github.com/multiformats/go-multihash/core"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/bytemap"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/ipld/codec/cbor"
	"github.com/storacha/go-ucanto/core/ipld/hash/sha256"
	qdm "github.com/storacha/indexing-service/pkg/service/queryresult/datamodel"
	"github.com/storacha/indexing-service/pkg/types"
)

type queryResult struct {
	root ipld.Block
	data *qdm.QueryResultModel0_2
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

func (q *queryResult) Messages() []string {
	return q.data.Messages
}

func (q *queryResult) Root() block.Block {
	return q.root
}

func Archive(qr types.QueryResult) io.Reader {
	return car.Encode([]datamodel.Link{qr.Root().Link()}, qr.Blocks())
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
	if queryResultModel.Result0_1 != nil {
		result := qdm.QueryResultModel0_2{
			Claims:  queryResultModel.Result0_1.Claims,
			Indexes: queryResultModel.Result0_1.Indexes,
		}
		return &queryResult{root, &result, blks}, nil
	}
	return &queryResult{root, queryResultModel.Result0_2, blks}, nil
}

type buildConfig struct {
	messages []string
}

type BuildOption func(cfg *buildConfig)

// WithMessage adds a message to the query result.
func WithMessage(message ...string) BuildOption {
	return func(cfg *buildConfig) {
		cfg.messages = append(cfg.messages, message...)
	}
}

// Build generates a new encodable QueryResult
func Build(claims map[cid.Cid]delegation.Delegation, indexes bytemap.ByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView], options ...BuildOption) (types.QueryResult, error) {
	var cfg buildConfig
	for _, o := range options {
		o(&cfg)
	}

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
		Result0_2: &qdm.QueryResultModel0_2{
			Claims:   cls,
			Indexes:  indexesModel,
			Messages: cfg.messages,
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

	return &queryResult{root: rt, data: queryResultModel.Result0_2, blks: bs}, nil
}
