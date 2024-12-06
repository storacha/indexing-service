package providerindex

import (
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/mocks"
	"github.com/storacha/indexing-service/pkg/types"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	adm "github.com/storacha/go-capabilities/pkg/assert/datamodel"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFind(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "")

		contentCID := testutil.RandomCID()
		contentHash := contentCID.(cidlink.Link).Hash()

		locationClaim := assert.Location.New(testutil.Service.DID().String(), assert.LocationCaveats{
			Content:  testutil.Must(assert.Digest(adm.DigestModel{Digest: contentHash}))(t),
			Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
		})
		locationDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[assert.LocationCaveats]{locationClaim}))(t)
		locationDelegationCid := testutil.Must(cid.Prefix{
			Version:  1,
			Codec:    cid.Raw,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(testutil.Must(io.ReadAll(delegation.Archive(locationDelegation)))(t)))(t)

		indexClaim := assert.Index.New(testutil.Service.DID().String(), assert.IndexCaveats{
			Content: contentCID,
			Index:   testutil.RandomCID(),
		})
		indexDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[assert.IndexCaveats]{indexClaim}))(t)
		indexDelegationCid := testutil.Must(cid.Prefix{
			Version:  1,
			Codec:    cid.Raw,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(testutil.Must(io.ReadAll(delegation.Archive(indexDelegation)))(t)))(t)

		ctx := context.Background()

		mockMapper.EXPECT().GetClaims(ctx, contentHash).Return([]cid.Cid{locationDelegationCid, indexDelegationCid}, nil)
		mockStore.EXPECT().Get(ctx, cidlink.Link{Cid: locationDelegationCid}).Return(locationDelegation, nil)
		mockStore.EXPECT().Get(ctx, cidlink.Link{Cid: indexDelegationCid}).Return(indexDelegation, nil)

		results, err := legacyClaims.Find(ctx, contentHash)

		require.NoError(t, err)
		require.Len(t, results, 2)

		// TODO: assert returned ProviderResults look as expected when synthetizing them is implemented
		expectedLocationResult := model.ProviderResult{}
		expectedIndexResult := model.ProviderResult{}
		require.Equal(t, []model.ProviderResult{expectedLocationResult, expectedIndexResult}, results)
	})

	t.Run("returns ErrKeyNotFound when the content hash is not found in the mapper", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "")

		mockMapper.EXPECT().GetClaims(mock.Anything, mock.Anything).Return(nil, types.ErrKeyNotFound)

		_, err := legacyClaims.Find(context.Background(), testutil.RandomMultihash())

		require.Equal(t, types.ErrKeyNotFound, err)
	})

	t.Run("returns ErrKeyNotFound when claims are not found in the store", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "")

		testCID := testutil.RandomCID().(cidlink.Link).Cid

		mockMapper.EXPECT().GetClaims(mock.Anything, mock.Anything).Return([]cid.Cid{testCID}, nil)
		mockStore.EXPECT().Get(mock.Anything, mock.Anything).Return(nil, types.ErrKeyNotFound)

		_, err := legacyClaims.Find(context.Background(), testutil.RandomMultihash())

		require.Equal(t, types.ErrKeyNotFound, err)
	})
}
