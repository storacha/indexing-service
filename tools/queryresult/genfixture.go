// Generates an archive of a query result that contains 1 index and 3 claims.
//
// The claims consist of 2 location claims, (for the index and a CAR blob) and
// 1 index claim, asserting the CID of the CAR blob is indexed by the index.
//
// The filename is for the content root multihash (the root of the DAG in the
// CAR blob).
package main

import (
	"bytes"
	crand "crypto/rand"
	"fmt"
	"io"
	"math/rand/v2"
	"net/url"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld/block"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
)

func main() {
	storageID := must(ed25519.Generate())
	uploadServiceID := must(ed25519.Generate())
	indexingServiceID := must(ed25519.Generate())
	space := must(ed25519.Generate())
	spaceDID := space.DID()

	blockBytes := randomBytes(32)
	blockDigest := must(multihash.Sum(blockBytes, multihash.SHA2_256, -1))
	blockLink := cidlink.Link{Cid: cid.NewCidV1(cid.Raw, blockDigest)}

	carRoots := []ipld.Link{blockLink}
	carBlock := block.NewBlock(blockLink, blockBytes)

	carBytes := must(io.ReadAll(car.Encode(carRoots, func(yield func(block.Block, error) bool) {
		yield(carBlock, nil)
	})))
	carDigest := must(multihash.Sum(carBytes, multihash.SHA2_256, -1))

	index := must(blobindex.FromShardArchives(blockLink, [][]byte{carBytes}))
	indexBytes := must(io.ReadAll(must(index.Archive())))
	indexDigest := must(multihash.Sum(indexBytes, multihash.SHA2_256, -1))
	indexLink := cidlink.Link{Cid: cid.NewCidV1(uint64(multicodec.Car), blockDigest)}

	carLocationURL := blobURL(randomURL(), carDigest)
	carLocationCommitment := must(assert.Location.Delegate(
		storageID,
		space,
		storageID.DID().String(),
		assert.LocationCaveats{
			Space:    space.DID(),
			Content:  assert.FromHash(carDigest),
			Location: []url.URL{*carLocationURL},
		},
	))

	indexLocationURL := blobURL(randomURL(), indexDigest)
	indexLocationCommitment := must(assert.Location.Delegate(
		storageID,
		space,
		storageID.DID().String(),
		assert.LocationCaveats{
			Space:    space.DID(),
			Content:  assert.FromHash(indexDigest),
			Location: []url.URL{*indexLocationURL},
		},
	))

	indexClaim := must(assert.Index.Delegate(
		uploadServiceID,
		indexingServiceID,
		indexingServiceID.DID().String(),
		assert.IndexCaveats{
			Content: blockLink,
			Index:   indexLink,
		},
		// delegation from indexing service to upload service allowing claims to be
		// registered.
		delegation.WithProof(
			delegation.FromDelegation(
				must(delegation.Delegate(
					indexingServiceID,
					uploadServiceID,
					[]ucan.Capability[ucan.NoCaveats]{
						ucan.NewCapability(
							assert.IndexAbility,
							indexingServiceID.DID().String(),
							ucan.NoCaveats{},
						),
					},
				)),
			),
		),
	))

	claims := map[cid.Cid]delegation.Delegation{
		carLocationCommitment.Link().(cidlink.Link).Cid:   carLocationCommitment,
		indexLocationCommitment.Link().(cidlink.Link).Cid: indexLocationCommitment,
		indexClaim.Link().(cidlink.Link).Cid:              indexClaim,
	}

	indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
	indexes.Set(
		must(types.ContextID{Space: &spaceDID, Hash: indexDigest}.ToEncoded()),
		index,
	)

	result := must(queryresult.Build(claims, indexes))
	resultBytes := must(io.ReadAll(car.Encode([]ipld.Link{result.Root().Link()}, result.Blocks())))

	// verify result can be read
	must(queryresult.Extract(bytes.NewReader(resultBytes)))

	filename := fmt.Sprintf("%s.queryresult.car", must(multibase.Encode(multibase.Base58BTC, blockDigest)))
	err := os.WriteFile(filename, resultBytes, 0644)
	if err != nil {
		panic(err)
	}
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

func randomBytes(size int) []byte {
	bytes := make([]byte, size)
	must(crand.Read(bytes))
	return bytes
}

func randomURL() *url.URL {
	port := 3000 + rand.IntN(1000)
	return must(url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port)))
}

func blobURL(base *url.URL, digest multihash.Multihash) *url.URL {
	return base.JoinPath("/blob/%s", must(multibase.Encode(multibase.Base58BTC, digest)))
}
