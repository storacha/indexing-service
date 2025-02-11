package providerindex

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/extmocks"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/storacha/ipni-publisher/pkg/publisher"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetProviderResults(t *testing.T) {
	t.Run("results found in the cache", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomProviderResult()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return([]model.ProviderResult{expectedResult}, nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, found in IPNI, results cached afterwards", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		ipniFinderResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, no results from IPNI, found in legacy claims service, results cached afterwards", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{metadata.LocationCommitmentID}).Return([]model.ProviderResult{expectedResult}, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, IPNI returns uninteresting results, search in legacy claims", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		bitswapResult := testutil.RandomBitswapProviderResult()
		ipniFinderResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{bitswapResult},
				},
			},
		}
		expectedResult := testutil.RandomLocationCommitmentProviderResult()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{metadata.LocationCommitmentID}).Return([]model.ProviderResult{expectedResult}, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("returns an empty slice when results are not found anywhere, nothing gets cached", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{0}).Return([]model.ProviderResult{}, nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("error in cache returns error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error fetching from IPNI returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error in legacy claims service returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{0}).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error caching results returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		ipniFinderResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(0, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.Error(t, err)
	})
}

func TestPublish(t *testing.T) {
	t.Run("allow skip publish existing advert", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		result := testutil.RandomLocationCommitmentProviderResult()
		provider := *result.Provider
		contextID := string(result.ContextID)
		digest := testutil.RandomMultihash()
		anyDigestSeq := mock.MatchedBy(func(digests iter.Seq[multihash.Multihash]) bool {
			return true
		})
		meta := metadata.MetadataContext.New()
		err := meta.UnmarshalBinary(result.Metadata)
		require.NoError(t, err)

		mockStore.EXPECT().Add(testutil.AnyContext, digest, result).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, digest, false).Return(nil)
		mockIpniPublisher.EXPECT().Publish(testutil.AnyContext, provider, contextID, anyDigestSeq, meta).Return(testutil.RandomCID(), publisher.ErrAlreadyAdvertised)

		err = providerIndex.Publish(context.Background(), provider, contextID, slices.Values([]multihash.Multihash{digest}), meta)
		require.NoError(t, err)
	})
}
