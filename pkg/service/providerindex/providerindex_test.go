package providerindex

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/ipnipublisher/publisher"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/extmocks"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetProviderResults(t *testing.T) {
	t.Run("results found in the cache", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return([]model.ProviderResult{expectedResult}, nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results found in the cache, do not match current query type", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		existingResult := testutil.RandomIndexClaimProviderResult()

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

		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return([]model.ProviderResult{existingResult}, nil)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return(nil, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, found in no providers cache, empty results returned", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)

		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("results not cached, found in IPNI, results cached afterwards", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

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

		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return(nil, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("results not cached, no results from IPNI, found in legacy claims service, results cached afterwards", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
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
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

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
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{metadata.LocationCommitmentID}).Return([]model.ProviderResult{expectedResult}, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("returns an empty slice when results are not found anywhere, no providers record gets cached", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{0}).Return([]model.ProviderResult{}, nil)
		mockNoProviderStore.EXPECT().Set(testutil.AnyContext, someHash, struct{}{}, true).Return(nil)
		mockNoProviderStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("error in cache returns error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error fetching from IPNI returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{0}).Return(nil, nil)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("error in legacy claims service returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(&model.FindResponse{}, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, []multicodec.Code{0}).Return(nil, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, []multicodec.Code{0})

		require.Error(t, err)
	})

	t.Run("errors in both legacy claims service and IPNI returns wrapped error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		// Simulate a cache miss.
		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)

		// Both queries error.
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(nil, errors.New("ipni error"))
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return(nil, errors.New("legacy error"))

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)
		require.Nil(t, results)
		require.Error(t, err)
		// Verify that the final error message contains both errors.
		require.Contains(t, err.Error(), "fetching from IPNI failed: ipni error")
		require.Contains(t, err.Error(), "fetching from legacy services failed: legacy error")
	})

	t.Run("error caching results returns an error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

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

		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniFinderResponse, nil)
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return(nil, nil)
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(0, errors.New("some error"))

		_, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)

		require.Error(t, err)
	})

	t.Run("IPNI returns valid result and legacy returns error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		ipniResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}
		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		// Simulate a cache miss.
		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		// IPNI returns a valid result.
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniResponse, nil)
		// Legacy returns an error.
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return(nil, errors.New("legacy error"))
		// Expect caching of the IPNI result.
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)
		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("IPNI returns valid result and legacy returns empty result", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		ipniResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}
		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		// Simulate a cache miss.
		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		// IPNI returns a valid result.
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniResponse, nil)
		// Legacy returns an error.
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return([]model.ProviderResult{}, nil)
		// Expect caching of the IPNI result.
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)
		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("legacy returns valid result and IPNI returns error", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		// IPNI returns an error.
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(nil, errors.New("ipni error"))
		// Legacy returns a valid result.
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return([]model.ProviderResult{expectedResult}, nil)
		// Expect caching of the legacy result.
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)
		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	t.Run("legacy returns valid result and IPNI returns empty result", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		// IPNI returns an error.
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(&model.FindResponse{}, nil)
		// Legacy returns a valid result.
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).Return([]model.ProviderResult{expectedResult}, nil)
		// Expect caching of the legacy result.
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)
		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})

	// NB(forrest): we may remove this test case when legacy service is fully removed and IPNI is the sole finder.
	t.Run("IPNI returns result before legacy is complete and legacy is canceled", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

		someHash := testutil.RandomMultihash()
		expectedResult := testutil.RandomLocationCommitmentProviderResult()
		ipniResponse := &model.FindResponse{
			MultihashResults: []model.MultihashResult{
				{
					Multihash:       someHash,
					ProviderResults: []model.ProviderResult{expectedResult},
				},
			},
		}
		targetClaim := []multicodec.Code{metadata.LocationCommitmentID}

		// Simulate a cache miss.
		mockStore.EXPECT().Members(testutil.AnyContext, someHash).Return(nil, types.ErrKeyNotFound)
		mockNoProviderStore.EXPECT().Get(testutil.AnyContext, someHash).Return(struct{}{}, types.ErrKeyNotFound)
		// IPNI returns immediately with a valid result.
		mockIpniFinder.EXPECT().Find(testutil.AnyContext, someHash).Return(ipniResponse, nil)
		// Legacy query simulates a delay and then returns, but will be canceled.
		mockLegacyClaims.EXPECT().Find(testutil.AnyContext, someHash, targetClaim).
			RunAndReturn(func(ctx context.Context, mh multihash.Multihash, targetClaims []multicodec.Code) ([]model.ProviderResult, error) {
				// Wait for cancellation.
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(100 * time.Millisecond):
					// This branch should not be reached if cancellation works.
					t.Fatal("legacy services returned a result when it should have been canceled by success of IPNI query")
				}
				return nil, nil
			})
		// Expect caching of the IPNI result.
		mockStore.EXPECT().Add(testutil.AnyContext, someHash, expectedResult).Return(1, nil)
		mockStore.EXPECT().SetExpirable(testutil.AnyContext, someHash, true).Return(nil)

		results, err := providerIndex.getProviderResults(context.Background(), someHash, targetClaim)
		require.NoError(t, err)
		require.Equal(t, []model.ProviderResult{expectedResult}, results)
	})
}

func TestPublish(t *testing.T) {
	t.Run("allow skip publish existing advert", func(t *testing.T) {
		mockStore := types.NewMockProviderStore(t)
		mockNoProviderStore := types.NewMockNoProviderStore(t)
		mockIpniFinder := extmocks.NewMockIpniFinder(t)
		mockIpniPublisher := extmocks.NewMockIpniPublisher(t)
		mockLegacyClaims := NewMockLegacyClaimsFinder(t)

		providerIndex := New(mockStore, mockNoProviderStore, mockIpniFinder, mockIpniPublisher, mockLegacyClaims)

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
