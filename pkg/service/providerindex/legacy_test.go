package providerindex

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/storacha/go-metadata"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/mocks"
	"github.com/storacha/indexing-service/pkg/types"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/maurl"
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
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "https://storacha.network/claims/{claim}")

		ctx := context.Background()
		contentHash := testutil.RandomMultihash()
		locationDelegation := testutil.RandomLocationDelegation()
		locationDelegationCid := testutil.RandomCID().(cidlink.Link).Cid
		indexDelegation := testutil.RandomIndexDelegation()
		indexDelegationCid := testutil.RandomCID().(cidlink.Link).Cid

		mockMapper.EXPECT().GetClaims(ctx, contentHash).Return([]cid.Cid{locationDelegationCid, indexDelegationCid}, nil)
		mockStore.EXPECT().Get(ctx, cidlink.Link{Cid: locationDelegationCid}).Return(locationDelegation, nil)
		mockStore.EXPECT().Get(ctx, cidlink.Link{Cid: indexDelegationCid}).Return(indexDelegation, nil)

		results, err := legacyClaims.Find(ctx, contentHash)

		require.NoError(t, err)
		require.Len(t, results, 2)

	})

	t.Run("returns ErrKeyNotFound when the content hash is not found in the mapper", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "https://storacha.network/claims/{claim}")

		mockMapper.EXPECT().GetClaims(mock.Anything, mock.Anything).Return(nil, types.ErrKeyNotFound)

		_, err := legacyClaims.Find(context.Background(), testutil.RandomMultihash())

		require.Equal(t, types.ErrKeyNotFound, err)
	})

	t.Run("returns ErrKeyNotFound when claims are not found in the store", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "https://storacha.network/claims/{claim}")

		testCID := testutil.RandomCID().(cidlink.Link).Cid

		mockMapper.EXPECT().GetClaims(mock.Anything, mock.Anything).Return([]cid.Cid{testCID}, nil)
		mockStore.EXPECT().Get(mock.Anything, mock.Anything).Return(nil, types.ErrKeyNotFound)

		_, err := legacyClaims.Find(context.Background(), testutil.RandomMultihash())

		require.Equal(t, types.ErrKeyNotFound, err)
	})
}

func TestSynthetizeProviderResult(t *testing.T) {
	t.Run("location claim", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "https://storacha.network/claims/{claim}")

		contentLink := testutil.RandomCID()
		contentCid := contentLink.(cidlink.Link).Cid
		contentHash := contentLink.(cidlink.Link).Hash()
		spaceDID := testutil.RandomPrincipal().DID()

		locationClaim := assert.Location.New(testutil.Service.DID().String(), assert.LocationCaveats{
			Content:  testutil.Must(assert.Digest(adm.DigestModel{Digest: contentHash}))(t),
			Location: []url.URL{*testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/blobs/%s", digestutil.Format(contentHash))))(t)},
			Space:    spaceDID,
		})
		locationDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[assert.LocationCaveats]{locationClaim}))(t)
		locationDelegationCid := testutil.Must(cid.Prefix{
			Version:  1,
			Codec:    cid.Raw,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(testutil.Must(io.ReadAll(delegation.Archive(locationDelegation)))(t)))(t)

		result, err := legacyClaims.synthetizeProviderResult(locationDelegationCid, locationDelegation)

		require.NoError(t, err)

		locContextID := testutil.Must(types.ContextID{
			Hash:  contentHash,
			Space: &spaceDID,
		}.ToEncoded())(t)
		require.Equal(t, []byte(locContextID), result.ContextID)

		md := metadata.MetadataContext.New()
		require.NoError(t, md.UnmarshalBinary(result.Metadata))
		locMeta := md.Get(metadata.LocationCommitmentID).(*metadata.LocationCommitmentMetadata)
		require.Equal(t, contentCid, *locMeta.Shard)
		require.Nil(t, locMeta.Range)
		require.Equal(t, int64(*locationDelegation.Expiration()), locMeta.Expiration)
		require.Equal(t, locationDelegationCid, locMeta.Claim)

		blobUrl := testutil.Must(url.Parse("https://storacha.network/blobs/{blob}"))(t)
		blobProviderAddr := testutil.Must(maurl.FromURL(blobUrl))(t)
		require.Equal(t, blobProviderAddr, result.Provider.Addrs[0])
	})

	t.Run("index claim", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "https://storacha.network/claims/{claim}")

		contentLink := testutil.RandomCID()
		indexLink := testutil.RandomCID()
		indexCid := indexLink.(cidlink.Link).Cid

		indexClaim := assert.Index.New(testutil.Service.DID().String(), assert.IndexCaveats{
			Content: contentLink,
			Index:   indexLink,
		})
		indexDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[assert.IndexCaveats]{indexClaim}))(t)
		indexDelegationCid := testutil.Must(cid.Prefix{
			Version:  1,
			Codec:    cid.Raw,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(testutil.Must(io.ReadAll(delegation.Archive(indexDelegation)))(t)))(t)

		result, err := legacyClaims.synthetizeProviderResult(indexDelegationCid, indexDelegation)

		require.NoError(t, err)

		indexContextID := []byte(indexLink.Binary())
		require.Equal(t, indexContextID, result.ContextID)

		md := metadata.MetadataContext.New()
		require.NoError(t, md.UnmarshalBinary(result.Metadata))
		indexMeta := md.Get(metadata.IndexClaimID).(*metadata.IndexClaimMetadata)
		require.Equal(t, indexCid, indexMeta.Index)
		require.Equal(t, int64(*indexDelegation.Expiration()), indexMeta.Expiration)
		require.Equal(t, indexDelegationCid, indexMeta.Claim)

		claimsUrl := testutil.Must(url.Parse("https://storacha.network/claims/{claim}"))(t)
		claimsProviderAddr := testutil.Must(maurl.FromURL(claimsUrl))(t)
		require.Equal(t, claimsProviderAddr, result.Provider.Addrs[0])
	})

	t.Run("equals claim", func(t *testing.T) {
		mockMapper := mocks.NewMockContentToClaimsMapper(t)
		mockStore := mocks.NewMockContentClaimsStore(t)
		legacyClaims := NewLegacyClaimsStore(mockMapper, mockStore, "https://storacha.network/claims/{claim}")

		contentLink := testutil.RandomCID()
		contentHash := contentLink.(cidlink.Link).Hash()
		equalsLink := testutil.RandomCID()
		equalsCid := equalsLink.(cidlink.Link).Cid

		equalsClaim := assert.Equals.New(testutil.Service.DID().String(), assert.EqualsCaveats{
			Content: testutil.Must(assert.Digest(adm.DigestModel{Digest: contentHash}))(t),
			Equals:  equalsLink,
		})

		equalsDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[assert.EqualsCaveats]{equalsClaim}))(t)
		equalsDelegationCid := testutil.Must(cid.Prefix{
			Version:  1,
			Codec:    cid.Raw,
			MhType:   mh.SHA2_256,
			MhLength: -1,
		}.Sum(testutil.Must(io.ReadAll(delegation.Archive(equalsDelegation)))(t)))(t)

		result, err := legacyClaims.synthetizeProviderResult(equalsDelegationCid, equalsDelegation)

		require.NoError(t, err)

		equalsContextID := []byte(equalsLink.Binary())
		require.Equal(t, equalsContextID, result.ContextID)

		md := metadata.MetadataContext.New()
		require.NoError(t, md.UnmarshalBinary(result.Metadata))
		equalsMeta := md.Get(metadata.EqualsClaimID).(*metadata.EqualsClaimMetadata)
		require.Equal(t, equalsCid, equalsMeta.Equals)
		require.Equal(t, int64(*equalsDelegation.Expiration()), equalsMeta.Expiration)
		require.Equal(t, equalsDelegationCid, equalsMeta.Claim)

		claimsUrl := testutil.Must(url.Parse("https://storacha.network/claims/{claim}"))(t)
		claimsProviderAddr := testutil.Must(maurl.FromURL(claimsUrl))(t)
		require.Equal(t, claimsProviderAddr, result.Provider.Addrs[0])
	})
}
