package legacy

import (
	"context"
	"errors"
	"io"
	"net/url"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestLegacyService(t *testing.T) {
	id := testutil.Service
	bucketURL, err := url.Parse("https://test.bucket.example.com")
	require.NoError(t, err)

	fixtures := []struct {
		name   string
		digest multihash.Multihash
		record BlockIndexRecord
		// the expected location URL in the materlized claim
		expectedURL url.URL
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
			expectedURL: *bucketURL.JoinPath("/bagbaierah63es3fder47bixl2tz5aix6nn44i55zy52ilxs462jx7oah25iq/bagbaierah63es3fder47bixl2tz5aix6nn44i55zy52ilxs462jx7oah25iq.car"),
		},
		{
			name:   "b32 CAR CID key",
			digest: testutil.Must(digestutil.Parse("zQmNVL7AESETquhed2Sv7VRq8ujqiL8NiPVmTBMCoKioZXh"))(t),
			record: BlockIndexRecord{
				CarPath: "us-west-2/carpark-prod-0/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq.car",
				Offset:  9196818,
				Length:  262144,
			},
			expectedURL: *bucketURL.JoinPath("/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq/bagbaieras4pzdxrc6rxlqfu73a4g4zmbtn54e77gwxyq4lvwqouwkknndquq.car"),
		},
		{
			name:   "b32 root CID key",
			digest: testutil.Must(digestutil.Parse("zQmPc8FCfDtjgC5xB2EXArnuYs2d53vT5kbH7HJejkYwCz4"))(t),
			record: BlockIndexRecord{
				CarPath: "us-west-2/dotstorage-prod-1/complete/bafybeihya44jmfali7ret42wvhasnkacg6s5pfuxt4ydszdyp5ib4knzjm.car",
				Offset:  8029928,
				Length:  58,
			},
			expectedURL: url.URL{},
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
			expectedURL: *testutil.Must(url.Parse("https://carpark-prod-0.r2.w3s.link/zQmRYBmBVN28FpKprXj8FiRxE8KLSkQ96gNsBu8LtnK7sEe/zQmRYBmBVN28FpKprXj8FiRxE8KLSkQ96gNsBu8LtnK7sEe.blob"))(t),
		},
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			mockStore := newMockBlockIndexStore()
			mockStore.data.Set(f.digest, []BlockIndexRecord{f.record})
			mockService := mockIndexingService{}
			service, err := NewService(id, &mockService, mockStore, bucketURL.String())
			require.NoError(t, err)

			query := types.Query{Hashes: []multihash.Multihash{f.digest}}
			results, err := service.Query(context.Background(), query)
			require.NoError(t, err)
			require.Empty(t, results.Indexes())

			if f.noClaim {
				require.Empty(t, results.Claims())
				return
			}

			require.Equal(t, 1, len(results.Claims()))

			br, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(results.Blocks()))
			require.NoError(t, err)

			claim, err := delegation.NewDelegationView(results.Claims()[0], br)
			require.NoError(t, err)
			require.Equal(t, id.DID().String(), claim.Issuer().DID().String())
			require.Equal(t, assert.LocationAbility, claim.Capabilities()[0].Can())
			require.NotNil(t, claim.Expiration())

			nb, err := assert.LocationCaveatsReader.Read(claim.Capabilities()[0].Nb())
			require.NoError(t, err)
			require.Equal(t, f.digest, nb.Content.Hash())
			require.Equal(t, 1, len(nb.Location))
			require.Equal(t, f.expectedURL.String(), nb.Location[0].String())
			require.Equal(t, f.record.Offset, nb.Range.Offset)
			require.Equal(t, f.record.Length, *nb.Range.Length)
		})
	}

	t.Run("returns claims from underlying indexing service", func(t *testing.T) {
		mockStore := newMockBlockIndexStore()
		digest := testutil.RandomMultihash()
		nb := assert.LocationCaveats{Content: assert.FromHash(digest)}
		claim, err := assert.Location.Delegate(id, id, id.DID().String(), nb)
		require.NoError(t, err)

		claims := map[cid.Cid]delegation.Delegation{link.ToCID(claim.Link()): claim}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](0)
		result, err := queryresult.Build(claims, indexes)
		require.NoError(t, err)

		mockService := mockIndexingService{result, nil}
		service, err := NewService(id, &mockService, mockStore, bucketURL.String())
		require.NoError(t, err)

		query := types.Query{Hashes: []multihash.Multihash{digest}}
		results, err := service.Query(context.Background(), query)
		require.NoError(t, err)
		require.Empty(t, results.Indexes())
		require.Equal(t, 1, len(results.Claims()))
		require.Equal(t, claim.Link(), results.Claims()[0])
	})

	t.Run("returns indexes from underlying indexing service", func(t *testing.T) {
		mockStore := newMockBlockIndexStore()
		root := testutil.RandomCID()
		digest := link.ToCID(root).Hash()

		index := blobindex.NewShardedDagIndexView(root, 0)
		indexBytes, err := io.ReadAll(testutil.Must(index.Archive())(t))
		require.NoError(t, err)

		indexCID, err := cid.Prefix{
			Version:  1,
			Codec:    uint64(multicodec.Car),
			MhType:   multihash.SHA2_256,
			MhLength: -1,
		}.Sum(indexBytes)
		require.NoError(t, err)

		claims := map[cid.Cid]delegation.Delegation{}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
		indexes.Set(types.EncodedContextID(digest), index)
		result, err := queryresult.Build(claims, indexes)
		require.NoError(t, err)

		mockService := mockIndexingService{result, nil}
		service, err := NewService(id, &mockService, mockStore, bucketURL.String())
		require.NoError(t, err)

		query := types.Query{Hashes: []multihash.Multihash{digest}}
		results, err := service.Query(context.Background(), query)
		require.NoError(t, err)
		require.Empty(t, results.Claims())
		require.Equal(t, 1, len(results.Indexes()))
		require.Equal(t, indexCID.String(), results.Indexes()[0].String())
	})

	t.Run("calls through to underlying indexing service", func(t *testing.T) {
		digest := testutil.RandomMultihash()
		mockStore := newMockBlockIndexStore()
		mockService := mockIndexingService{nil, errNotImplemented}
		service, err := NewService(id, &mockService, mockStore, bucketURL.String())
		require.NoError(t, err)

		nb := assert.LocationCaveats{Content: assert.FromHash(digest)}
		claim, err := assert.Location.Delegate(id, id, id.DID().String(), nb)
		require.NoError(t, err)

		err = service.Cache(context.Background(), peer.AddrInfo{}, claim)
		require.True(t, errors.Is(err, errNotImplemented))

		_, err = service.Get(context.Background(), testutil.RandomCID())
		require.True(t, errors.Is(err, errNotImplemented))

		err = service.Publish(context.Background(), claim)
		require.True(t, errors.Is(err, errNotImplemented))

		query := types.Query{Hashes: []multihash.Multihash{digest}}
		_, err = service.Query(context.Background(), query)
		require.True(t, errors.Is(err, errNotImplemented))
	})
}

var errNotImplemented = errors.New("not implemented")

type mockIndexingService struct {
	queryResult types.QueryResult
	queryError  error
}

func (is *mockIndexingService) Cache(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error {
	return errNotImplemented
}

func (is *mockIndexingService) Get(ctx context.Context, claim datamodel.Link) (delegation.Delegation, error) {
	return nil, errNotImplemented
}

func (is *mockIndexingService) Publish(ctx context.Context, claim delegation.Delegation) error {
	return errNotImplemented
}

func (is *mockIndexingService) Query(ctx context.Context, q types.Query) (types.QueryResult, error) {
	if is.queryError != nil {
		return nil, is.queryError
	}
	if is.queryResult != nil {
		return is.queryResult, nil
	}
	claims := map[cid.Cid]delegation.Delegation{}
	indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](0)
	return queryresult.Build(claims, indexes)
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
