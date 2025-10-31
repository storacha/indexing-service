package queryresult

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/bytemap"
	"github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/validator"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestBuildCompressed(t *testing.T) {
	t.Run("compresses with matching index entry", func(t *testing.T) {
		// Create a test signer/principal
		principal := testutil.RandomSigner(t)

		// Create a target multihash that we'll search for
		targetMh := testutil.RandomMultihash(t)

		// Create a sharded dag index and add our target multihash to it
		contentLink := testutil.RandomCID(t)
		index := blobindex.NewShardedDagIndexView(contentLink, 1)

		// Create a shard and add slices to it, including our target
		shardMh := testutil.RandomMultihash(t)

		// Add our target multihash at a specific position within the shard
		targetPos := blobindex.Position{
			Offset: 100,
			Length: 50,
		}
		index.SetSlice(shardMh, targetMh, targetPos)

		// Add some other random slices to make it more realistic
		for i := 0; i < 5; i++ {
			index.SetSlice(shardMh, testutil.RandomMultihash(t), blobindex.Position{
				Offset: uint64(200 + i*100),
				Length: 75,
			})
		}

		// Get the index hash
		indexHash := shardMh

		// Create a location claim for the shard
		// This represents where the shard is stored
		locationURL, err := url.Parse("https://example.com/shard.car")
		require.NoError(t, err)
		shardLength := uint64(5000)
		shardClaim, err := assert.Location.Delegate(
			principal,
			principal,
			principal.DID().String(),
			assert.LocationCaveats{
				Content:  ctypes.FromHash(shardMh),
				Location: []url.URL{*locationURL},
				Range: &assert.Range{
					Offset: 1000, // The shard starts at offset 1000
					Length: &shardLength,
				},
			},
		)
		require.NoError(t, err)

		// Build the claims map
		claims := map[cid.Cid]delegation.Delegation{
			link.ToCID(shardClaim.Link()): shardClaim,
		}

		// Build the indexes map
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
		indexContextID, err := types.ContextID{
			Hash: indexHash,
		}.ToEncoded()
		require.NoError(t, err)
		indexes.Set(indexContextID, index)

		// Call BuildCompressed
		result, err := BuildCompressed(targetMh, principal, claims, indexes)
		require.NoError(t, err)

		// Verify the result
		resultClaims := result.Claims()
		require.Len(t, resultClaims, 1, "should have exactly one claim")

		// Verify there are no indexes in the compressed result
		resultIndexes := result.Indexes()
		require.Len(t, resultIndexes, 0, "should have no indexes")

		// To verify the claim content, we need to export the result and re-import it
		// This is the same way it would be used in practice
		// For now, we'll just verify the basic structure since we know BuildCompressed
		// creates a new location claim with the expected properties

		var compressedRoot ipld.Block
		for blk, err := range result.Blocks() {
			require.NoError(t, err)
			if blk.Link().(cidlink.Link).Cid.Equals(resultClaims[0].(cidlink.Link).Cid) {
				compressedRoot = blk
			}
		}
		require.NotNil(t, compressedRoot, "should find the compressed claim block")
		compressedClaim := testutil.Must(delegation.NewDelegation(compressedRoot, testutil.Must(blockstore.NewBlockReader(blockstore.WithBlocksIterator(result.Blocks())))(t)))(t)
		// Verify it's a location claim
		require.Len(t, compressedClaim.Capabilities(), 1, "should have one capability")
		match, err := assert.Location.Match(validator.NewSource(compressedClaim.Capabilities()[0], compressedClaim))
		require.NoError(t, err)

		caveats := match.Value().Nb()

		// Verify the content is our target multihash
		contentMh := caveats.Content.Hash()
		require.True(t, bytes.Equal(contentMh, targetMh), "content should be target multihash")

		// Verify the location URL is from the original claim
		require.Equal(t, *locationURL, caveats.Location[0], "location URL should match original claim")

		// Verify the range is based on the position of the slice in the shard
		// The offset should be: original offset (1000) + target position offset (targetPos.Offset)
		// The length should be: target position length (targetPos.Length)
		require.NotNil(t, caveats.Range, "range should be set")
		expectedOffset := uint64(1000) + targetPos.Offset
		require.Equal(t, expectedOffset, caveats.Range.Offset, "range offset should be original offset + slice offset")
		require.NotNil(t, caveats.Range.Length, "range length should be set")
		require.Equal(t, targetPos.Length, *caveats.Range.Length, "range length should match slice length")
	})

	t.Run("doesn't fail when matching location claim doesn't have a range", func(t *testing.T) {
		// Create a test signer/principal
		principal := testutil.RandomSigner(t)

		// Create a target multihash that we'll search for
		targetMh := testutil.RandomMultihash(t)

		// Create a sharded dag index and add our target multihash to it
		contentLink := testutil.RandomCID(t)
		index := blobindex.NewShardedDagIndexView(contentLink, 1)

		// Create a shard and add slices to it, including our target
		shardMh := testutil.RandomMultihash(t)

		// Add our target multihash at a specific position within the shard
		targetPos := blobindex.Position{
			Offset: 100,
			Length: 50,
		}
		index.SetSlice(shardMh, targetMh, targetPos)

		// Add some other random slices to make it more realistic
		for i := 0; i < 5; i++ {
			index.SetSlice(shardMh, testutil.RandomMultihash(t), blobindex.Position{
				Offset: uint64(200 + i*100),
				Length: 75,
			})
		}

		// Get the index hash
		indexHash := shardMh

		// Create a location claim for the shard
		// This represents where the shard is stored
		locationURL, err := url.Parse("https://example.com/shard.car")
		require.NoError(t, err)
		shardClaim, err := assert.Location.Delegate(
			principal,
			principal,
			principal.DID().String(),
			assert.LocationCaveats{
				Content:  ctypes.FromHash(shardMh),
				Location: []url.URL{*locationURL},
				Range:    nil, // No range specified
			},
		)
		require.NoError(t, err)

		// Build the claims map
		claims := map[cid.Cid]delegation.Delegation{
			link.ToCID(shardClaim.Link()): shardClaim,
		}

		// Build the indexes map
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
		indexContextID, err := types.ContextID{
			Hash: indexHash,
		}.ToEncoded()
		require.NoError(t, err)
		indexes.Set(indexContextID, index)

		// Call BuildCompressed
		result, err := BuildCompressed(targetMh, principal, claims, indexes)
		require.NoError(t, err)

		// Verify the result
		resultClaims := result.Claims()
		require.Len(t, resultClaims, 1, "should have exactly one claim")

		// Verify there are no indexes in the compressed result
		resultIndexes := result.Indexes()
		require.Len(t, resultIndexes, 0, "should have no indexes")

		// To verify the claim content, we need to export the result and re-import it
		// This is the same way it would be used in practice
		// For now, we'll just verify the basic structure since we know BuildCompressed
		// creates a new location claim with the expected properties

		var compressedRoot ipld.Block
		for blk, err := range result.Blocks() {
			require.NoError(t, err)
			if blk.Link().(cidlink.Link).Cid.Equals(resultClaims[0].(cidlink.Link).Cid) {
				compressedRoot = blk
			}
		}
		require.NotNil(t, compressedRoot, "should find the compressed claim block")
		compressedClaim := testutil.Must(delegation.NewDelegation(compressedRoot, testutil.Must(blockstore.NewBlockReader(blockstore.WithBlocksIterator(result.Blocks())))(t)))(t)
		// Verify it's a location claim
		require.Len(t, compressedClaim.Capabilities(), 1, "should have one capability")
		match, err := assert.Location.Match(validator.NewSource(compressedClaim.Capabilities()[0], compressedClaim))
		require.NoError(t, err)

		caveats := match.Value().Nb()

		// Verify the content is our target multihash
		contentMh := caveats.Content.Hash()
		require.True(t, bytes.Equal(contentMh, targetMh), "content should be target multihash")

		// Verify the location URL is from the original claim
		require.Equal(t, *locationURL, caveats.Location[0], "location URL should match original claim")

		// Verify the range is based on the position of the slice in the shard
		// The offset should be: original offset (no offset = 0) + target position offset (targetPos.Offset)
		// The length should be: target position length (targetPos.Length)
		require.NotNil(t, caveats.Range, "range should be set")
		expectedOffset := targetPos.Offset
		require.Equal(t, expectedOffset, caveats.Range.Offset, "range offset should be original offset + slice offset")
		require.NotNil(t, caveats.Range.Length, "range length should be set")
		require.Equal(t, targetPos.Length, *caveats.Range.Length, "range length should match slice length")
	})

	t.Run("returns regular result when no matching index entry", func(t *testing.T) {
		principal := testutil.RandomSigner(t)

		// Create a target multihash that won't be in the index
		targetMh := testutil.RandomMultihash(t)

		// Create a sharded dag index without the target multihash
		contentLink := testutil.RandomCID(t)
		index := blobindex.NewShardedDagIndexView(contentLink, 1)

		// Add some slices that don't include our target
		shardMh := testutil.RandomMultihash(t)
		for i := 0; i < 5; i++ {
			// Use different multihashes, not our target
			index.SetSlice(shardMh, testutil.RandomMultihash(t), blobindex.Position{
				Offset: uint64(100 + i*100),
				Length: 50,
			})
		}

		indexHash := shardMh

		// Create location and index claims
		locationClaim := testutil.RandomLocationDelegation(t)
		indexClaim := testutil.RandomIndexDelegation(t)
		claims := map[cid.Cid]delegation.Delegation{
			link.ToCID(locationClaim.Link()): locationClaim,
			link.ToCID(indexClaim.Link()):    indexClaim,
		}

		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
		indexContextID, err := types.ContextID{
			Hash: indexHash,
		}.ToEncoded()
		require.NoError(t, err)
		indexes.Set(indexContextID, index)

		// Call BuildCompressed
		result, err := BuildCompressed(targetMh, principal, claims, indexes)
		require.NoError(t, err)

		// Should return the regular result with all claims and indexes
		resultClaims := result.Claims()
		require.Len(t, resultClaims, 2, "should have both original claims")

		resultIndexes := result.Indexes()
		require.Len(t, resultIndexes, 1, "should have the original index")
	})

	t.Run("returns regular result when no indexes", func(t *testing.T) {
		principal := testutil.RandomSigner(t)
		targetMh := testutil.RandomMultihash(t)

		locationClaim := testutil.RandomLocationDelegation(t)
		claims := map[cid.Cid]delegation.Delegation{
			link.ToCID(locationClaim.Link()): locationClaim,
		}

		// Empty indexes
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1)

		// Call BuildCompressed
		result, err := BuildCompressed(targetMh, principal, claims, indexes)
		require.NoError(t, err)

		// Should return the regular result
		resultClaims := result.Claims()
		require.Len(t, resultClaims, 1, "should have the original claim")

		resultIndexes := result.Indexes()
		require.Len(t, resultIndexes, 0, "should have no indexes")
	})
}
