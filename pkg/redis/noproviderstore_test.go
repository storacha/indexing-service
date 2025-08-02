package redis_test

import (
	"context"
	"math/rand/v2"
	"testing"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/stretchr/testify/require"
)

func TestNoProviderStore(t *testing.T) {
	mockRedis := NewMockRedis()
	noProviderStore := redis.NewNoProviderStore(mockRedis)
	mh1, results1 := randomCodecList(t, 4)
	mh2, results2 := randomCodecList(t, 4)

	ctx := context.Background()
	_, err := noProviderStore.Add(ctx, mh1, results1...)
	require.NoError(t, err)
	_, err = noProviderStore.Add(ctx, mh2, results2...)
	require.NoError(t, err)

	returnedResults1 := testutil.Must(noProviderStore.Members(ctx, mh1))(t)
	returnedResults2 := testutil.Must(noProviderStore.Members(ctx, mh2))(t)

	require.ElementsMatch(t, results1, returnedResults1)
	require.ElementsMatch(t, results2, returnedResults2)
}

func randomCodecList(t *testing.T, num int) (multihash.Multihash, []multicodec.Code) {
	randomHash := testutil.RandomCID(t).(cidlink.Link).Cid.Hash()
	codes := make([]multicodec.Code, 0, num)
	for i := 0; i < num; i++ {
		codes = append(codes, multicodec.Code(rand.Uint64()))
	}

	return randomHash, codes
}
