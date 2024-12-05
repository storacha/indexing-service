package providerindex

import (
	"context"
	"errors"
	"testing"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/mocks"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestGetProviderResults(t *testing.T) {
	t.Run("results found in the cache", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomProviderResult()

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return([]model.ProviderResult{expectedResult}, nil)

		results, err := providerIndex.getProviderResults(ctx, someHash)

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, found in legacy claims service", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomProviderResult()

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockLegacyClaims.EXPECT().Find(ctx, someHash).Return([]model.ProviderResult{expectedResult}, nil)
		mockStore.EXPECT().Set(ctx, someHash, []model.ProviderResult{expectedResult}, true).Return(nil)

		results, err := providerIndex.getProviderResults(ctx, someHash)

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, not found in legacy claims service, found in IPNI", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomProviderResult()
		ipniFinderResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockLegacyClaims.EXPECT().Find(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Set(ctx, someHash, []model.ProviderResult{expectedResult}, true).Return(nil)

		results, err := providerIndex.getProviderResults(ctx, someHash)

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("error in cache returns error", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash)

		require.Error(t, err)
	})

	t.Run("error in legacy claims service causes fallback to IPNI", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomProviderResult()
		ipniFinderResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockLegacyClaims.EXPECT().Find(ctx, someHash).Return(nil, errors.New("some error"))
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Set(ctx, someHash, []model.ProviderResult{expectedResult}, true).Return(nil)

		results, err := providerIndex.getProviderResults(ctx, someHash)

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("error fetching from IPNI returns an error", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockLegacyClaims.EXPECT().Find(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash)

		require.Error(t, err)
	})

	t.Run("error caching results returns an error", func(t *testing.T) {
		mockStore := mocks.NewMockProviderStore(t)
		mockIpniFinder := mocks.NewMockFinder(t)
		mockIpniPublisher := mocks.NewMockPublisher(t)
		mockLegacyClaims := mocks.NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomProviderResult()
		ipniFinderResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}

		ctx := context.Background()
		mockStore.EXPECT().Get(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockLegacyClaims.EXPECT().Find(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Set(ctx, someHash, []model.ProviderResult{expectedResult}, true).Return(errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash)

		require.Error(t, err)
	})
}
