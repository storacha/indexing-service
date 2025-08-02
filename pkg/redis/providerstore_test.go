package redis_test

import (
	"context"
	"testing"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/stretchr/testify/require"
)

func TestProviderStore(t *testing.T) {
	mockRedis := NewMockRedis()
	providerStore := redis.NewProviderStore(mockRedis)
	mh1, results1 := randomProviderResults(t, 4)
	mh2, results2 := randomProviderResults(t, 4)

	ctx := context.Background()
	_, err := providerStore.Add(ctx, mh1, results1...)
	require.NoError(t, err)
	_, err = providerStore.Add(ctx, mh2, results2...)
	require.NoError(t, err)

	returnedResults1 := testutil.Must(providerStore.Members(ctx, mh1))(t)
	returnedResults2 := testutil.Must(providerStore.Members(ctx, mh2))(t)

	require.ElementsMatch(t, results1, returnedResults1)
	require.ElementsMatch(t, results2, returnedResults2)
}

func randomProviderResults(t *testing.T, num int) (multihash.Multihash, []model.ProviderResult) {
	randomHash := testutil.RandomCID(t).(cidlink.Link).Cid.Hash()
	providerResults := make([]model.ProviderResult, 0, num)
	for i := 0; i < num; i++ {
		providerResults = append(providerResults, testutil.RandomProviderResult(t))
	}

	return randomHash, providerResults
}
