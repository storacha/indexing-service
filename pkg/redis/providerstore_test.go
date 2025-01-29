package redis_test

import (
	"context"
	"testing"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/stretchr/testify/require"
)

func TestProviderStore(t *testing.T) {
	mockRedis := NewMockRedis()
	providerStore := redis.NewProviderStore(mockRedis)
	mh1, results1 := testutil.Must2(randomProviderResults(4))(t)
	mh2, results2 := testutil.Must2(randomProviderResults(4))(t)

	ctx := context.Background()
	_, err := providerStore.Add(ctx, mh1, results1...)
	require.NoError(t, err)
	_, err = providerStore.Add(ctx, mh2, results2...)
	require.NoError(t, err)

	returnedResults1 := testutil.Must(providerStore.Get(ctx, mh1))(t)
	returnedResults2 := testutil.Must(providerStore.Get(ctx, mh2))(t)
	require.Equal(t, results1, returnedResults1)
	require.Equal(t, results2, returnedResults2)
}

func randomProviderResults(num int) (multihash.Multihash, []model.ProviderResult, error) {
	randomHash := testutil.RandomCID().(cidlink.Link).Cid.Hash()
	providerResults := make([]model.ProviderResult, 0, num)
	for i := 0; i < num; i++ {
		providerResults = append(providerResults, testutil.RandomProviderResult())
	}

	return randomHash, providerResults, nil
}
