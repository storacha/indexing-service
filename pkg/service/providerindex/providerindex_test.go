package providerindex

import (
	"context"
	"errors"
	"testing"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multicodec"
	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/extmocks"
	"github.com/storacha/indexing-service/pkg/types"
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

		ctx := context.Background()

		mockStore.EXPECT().Members(ctx, someHash).Return([]model.ProviderResult{expectedResult}, nil)

		results, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{0})

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

		ctx := context.Background()

		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Add(ctx, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(ctx, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{metadata.LocationCommitmentID})

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

		ctx := context.Background()

		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(ctx, someHash, []multicodec.Code{metadata.LocationCommitmentID}).Return([]model.ProviderResult{expectedResult}, nil)
		mockStore.EXPECT().Add(ctx, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(ctx, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{metadata.LocationCommitmentID})

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

		ctx := context.Background()

		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(ipniFinderResponse, nil)
		mockLegacyClaims.EXPECT().Find(ctx, someHash, []multicodec.Code{metadata.LocationCommitmentID}).Return([]model.ProviderResult{expectedResult}, nil)
		mockStore.EXPECT().Add(ctx, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(ctx, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{metadata.LocationCommitmentID})

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

		ctx := context.Background()

		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(ctx, someHash, []multicodec.Code{0}).Return([]model.ProviderResult{}, nil)

		results, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{0})

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

		ctx := context.Background()
		mockStore.EXPECT().Members(ctx, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error fetching from IPNI returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		ctx := context.Background()
		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error in legacy claims service returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		ctx := context.Background()
		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(ctx, someHash, []multicodec.Code{0}).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{0})

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

		ctx := context.Background()
		mockStore.EXPECT().Members(ctx, someHash).Return(nil, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(ctx, someHash).Return(ipniFinderResponse, nil)
		mockStore.EXPECT().Add(ctx, someHash, expectedResult).Return(0, errors.New("some error"))

		_, err := providerIndex.getProviderResults(ctx, someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.Error(t, err)
	})
}
