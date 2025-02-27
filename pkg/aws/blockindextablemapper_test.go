package aws

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-libstoracha/capabilities/assert"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestBlockIndexTableMapper(t *testing.T) {
	id := testutil.Service
	bucketURL := testutil.Must(url.Parse("https://test.bucket.example.com"))(t)

	fixtures := []struct {
		name   string
		digest multihash.Multihash
		record BlockIndexRecord
		// the expected location URL in the materlized claim
		expectedURL *url.URL
		// signals that no claim can be materialized from fixture
		noClaim bool
	}{
		{
			name:   "b32 multihash key",
			digest: testutil.Must(digestutil.Parse("zQmNUfyG3ynAkCzPFLsijsJwEFpPXqJZF1CJpT9GLYmgBBd"))(t),
			record: BlockIndexRecord{
				CarPath: "us-west-2/dotstorage-prod-1/raw/bafyreifvbqc4e5qphijgpj43qxk5ndw2vbfbhkzuuuvpo4cturr2dfk45e/315318734258474846/ciqd7nsjnsrsi6pqulv5j46qel7gw6oeo644o5ef3zopne37xad5oui.car",
				Offset:  128844,
				Length:  200,
			},
			expectedURL: bucketURL.JoinPath("/bagbaierah63es3fder47bixl2tz5aix6nn44i55zy52ilxs462jx7oah25iq/bagbaierah63es3fder47bixl2tz5aix6nn44i55zy52ilxs462jx7oah25iq.car"),
		},
		{
			name:   "b32 CAR CID key",
			digest: testutil.Must(digestutil.Parse("zQmNVL7AESETquhed2Sv7VRq8ujqiL8NiPVmTBMCoKioZXh"))(t),
			record: BlockIndexRecord{
				CarPath: "us-west-2/carpark-prod-0/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq.car",
				Offset:  9196818,
				Length:  262144,
			},
			expectedURL: bucketURL.JoinPath("/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq.car"),
		},
		{
			name:   "b32 root CID key",
			digest: testutil.Must(digestutil.Parse("zQmPc8FCfDtjgC5xB2EXArnuYs2d53vT5kbH7HJejkYwCz4"))(t),
			record: BlockIndexRecord{
				CarPath: "us-west-2/dotstorage-prod-1/complete/bafybeihya44jmfali7ret42wvhasnkacg6s5pfuxt4ydszdyp5ib4knzjm.car",
				Offset:  8029928,
				Length:  58,
			},
			expectedURL: nil,
			noClaim:     true,
		},
		{
			name:   "b58 multihash URL",
			digest: testutil.Must(digestutil.Parse("zQmaRyqqRHaGmqdRBAWTsbC1cezEgtbCmVftcNVyXFcJ4n6"))(t),
			record: BlockIndexRecord{
				CarPath: "https://carpark-prod-0.r2.w3s.link/zQmRYBmBVN28FpKprXj8FiRxE8KLSkQ96gNsBu8LtnK7sEe/zQmRYBmBVN28FpKprXj8FiRxE8KLSkQ96gNsBu8LtnK7sEe.blob",
				Offset:  5401120,
				Length:  36876,
			},
			expectedURL: testutil.Must(url.Parse("https://carpark-prod-0.r2.w3s.link/zQmRYBmBVN28FpKprXj8FiRxE8KLSkQ96gNsBu8LtnK7sEe/zQmRYBmBVN28FpKprXj8FiRxE8KLSkQ96gNsBu8LtnK7sEe.blob"))(t),
		},
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			mockStore := newMockBlockIndexStore()
			mockStore.data.Set(f.digest, []BlockIndexRecord{f.record})
			bitMapper, err := NewBlockIndexTableMapper(id, mockStore, bucketURL.String(), time.Hour)
			require.NoError(t, err)

			claimCids, err := bitMapper.GetClaims(context.Background(), f.digest)
			require.NoError(t, err)

			if f.noClaim {
				require.Empty(t, claimCids)
				return
			}

			require.Equal(t, 1, len(claimCids))

			dh, err := multihash.Decode(claimCids[0].Hash())
			require.NoError(t, err)

			claim, err := delegation.Extract(dh.Digest)
			require.NoError(t, err)

			require.Equal(t, id.DID().String(), claim.Issuer().DID().String())
			require.Equal(t, cassert.LocationAbility, claim.Capabilities()[0].Can())
			require.NotNil(t, claim.Expiration())

			nb, err := cassert.LocationCaveatsReader.Read(claim.Capabilities()[0].Nb())
			require.NoError(t, err)
			require.Equal(t, f.digest, nb.Content.Hash())
			require.Equal(t, 1, len(nb.Location))
			require.Equal(t, f.expectedURL.String(), nb.Location[0].String())
			require.Equal(t, f.record.Offset, nb.Range.Offset)
			require.Equal(t, f.record.Length, *nb.Range.Length)
		})
	}

	t.Run("returns ErrKeyNotFound when block index errors with not found", func(t *testing.T) {
		mockStore := newMockBlockIndexStore()
		bitMapper, err := NewBlockIndexTableMapper(id, mockStore, bucketURL.String(), time.Hour)
		require.NoError(t, err)

		_, err = bitMapper.GetClaims(context.Background(), testutil.RandomMultihash())
		require.ErrorIs(t, err, types.ErrKeyNotFound)
	})
}

type mockBlockIndexStore struct {
	data bytemap.ByteMap[multihash.Multihash, []BlockIndexRecord]
}

func (bs *mockBlockIndexStore) Query(ctx context.Context, digest multihash.Multihash) ([]BlockIndexRecord, error) {
	records := bs.data.Get(digest)
	if len(records) == 0 {
		return nil, types.ErrKeyNotFound
	}
	return records, nil
}

func newMockBlockIndexStore() *mockBlockIndexStore {
	return &mockBlockIndexStore{
		data: bytemap.NewByteMap[multihash.Multihash, []BlockIndexRecord](1),
	}
}
