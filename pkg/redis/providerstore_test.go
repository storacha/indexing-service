package redis_test

import (
	"context"
	"testing"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/metadata"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestProviderStore(t *testing.T) {
	mockRedis := NewMockRedis()
	providerStore := redis.NewProviderStore(mockRedis)
	mh1, results1 := testutil.Must2(randomProviderResults(4))(t)
	mh2, results2 := testutil.Must2(randomProviderResults(4))(t)

	ctx := context.Background()
	require.NoError(t, providerStore.Set(ctx, mh1, results1, false))
	require.NoError(t, providerStore.Set(ctx, mh2, results2, true))

	returnedResults1 := testutil.Must(providerStore.Get(ctx, mh1))(t)
	returnedResults2 := testutil.Must(providerStore.Get(ctx, mh2))(t)
	require.Equal(t, results1, returnedResults1)
	require.Equal(t, results2, returnedResults2)
}

func randomProviderResults(num int) (multihash.Multihash, []model.ProviderResult, error) {
	randomHash := testutil.RandomCID().(cidlink.Link).Cid.Hash()
	aliceDid := testutil.Alice.DID()
	encodedContextID, err := types.ContextID{Space: &aliceDid, Hash: randomHash}.ToEncoded()
	if err != nil {
		return nil, nil, err
	}
	metadata, err := (&metadata.LocationCommitmentMetadata{
		ClaimCID: testutil.RandomCID().(cidlink.Link).Cid,
	}).MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	providerResults := make([]model.ProviderResult, 0, num)
	for i := 0; i < num; i++ {
		providerResults = append(providerResults, model.ProviderResult{
			ContextID: encodedContextID,
			Metadata:  metadata,
			Provider: &peer.AddrInfo{
				ID:    testutil.RandomPeer(),
				Addrs: []multiaddr.Multiaddr{testutil.RandomMultiaddr()},
			},
		})
	}

	return randomHash, providerResults, nil
}
