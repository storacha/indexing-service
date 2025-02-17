package providerindex

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/multiformats/go-multicodec"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/types"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/maurl"
	cassert "github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFind(t *testing.T) {
	allTargetClaims := []multicodec.Code{metadata.LocationCommitmentID, metadata.IndexClaimID, metadata.EqualsClaimID}
	t.Run("happy path, unsupported claims are filtered out", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentHash := testutil.RandomMultihash()

		partitionClaim := cassert.Partition.New(testutil.Service.DID().String(), cassert.PartitionCaveats{
			Content: ctypes.FromHash(contentHash),
			Blocks:  nil,
			Parts:   nil,
		})
		partitionDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[cassert.PartitionCaveats]{partitionClaim}))(t)
		partitionDelegationCid := link.ToCID(testutil.RandomCID())
		locationDelegation := testutil.RandomLocationDelegation()
		locationDelegationCid := link.ToCID(testutil.RandomCID())
		indexDelegation := testutil.RandomIndexDelegation()
		indexDelegationCid := link.ToCID(testutil.RandomCID())

		mockMapper.EXPECT().GetClaims(testutil.AnyContext, contentHash).Return([]cid.Cid{partitionDelegationCid, locationDelegationCid, indexDelegationCid}, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: partitionDelegationCid}, &url.URL{}).Return(partitionDelegation, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: locationDelegationCid}, &url.URL{}).Return(locationDelegation, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: indexDelegationCid}, &url.URL{}).Return(indexDelegation, nil)

		results, err := legacyClaims.Find(context.Background(), contentHash, allTargetClaims)

		require.NoError(t, err)
		require.Len(t, results, 2)
	})

	t.Run("second mapper is not checked if first mapper returns the claims we are interested in, other claims are filtered", func(t *testing.T) {
		mockMapper1 := NewMockContentToClaimsMapper(t)
		mockMapper2 := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper1, mockMapper2}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentHash := testutil.RandomMultihash()

		locationDelegation := testutil.RandomLocationDelegation()
		locationDelegationCid := link.ToCID(testutil.RandomCID())
		indexDelegation := testutil.RandomIndexDelegation()
		indexDelegationCid := link.ToCID(testutil.RandomCID())
		equalsDelegation := testutil.RandomEqualsDelegation()
		equalsDelegationCid := link.ToCID(testutil.RandomCID())

		mockMapper1.EXPECT().GetClaims(testutil.AnyContext, contentHash).Return([]cid.Cid{locationDelegationCid, indexDelegationCid, equalsDelegationCid}, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: locationDelegationCid}, &url.URL{}).Return(locationDelegation, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: indexDelegationCid}, &url.URL{}).Return(indexDelegation, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: equalsDelegationCid}, &url.URL{}).Return(equalsDelegation, nil)

		results, err := legacyClaims.Find(context.Background(), contentHash, []multicodec.Code{metadata.LocationCommitmentID})

		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("second mapper is checked when first mapper returns claims, but not the type we are interested in", func(t *testing.T) {
		mockMapper1 := NewMockContentToClaimsMapper(t)
		mockMapper2 := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper1, mockMapper2}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentHash := testutil.RandomMultihash()

		locationDelegation := testutil.RandomLocationDelegation()
		locationDelegationCid := link.ToCID(testutil.RandomCID())
		indexDelegation := testutil.RandomIndexDelegation()
		indexDelegationCid := link.ToCID(testutil.RandomCID())
		equalsDelegation := testutil.RandomEqualsDelegation()
		equalsDelegationCid := link.ToCID(testutil.RandomCID())

		// mapper1 returns an equals claim, but we are looking for location and index
		mockMapper1.EXPECT().GetClaims(testutil.AnyContext, contentHash).Return([]cid.Cid{equalsDelegationCid}, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: equalsDelegationCid}, &url.URL{}).Return(equalsDelegation, nil)

		// GetClaims is called on mapper2
		mockMapper2.EXPECT().GetClaims(testutil.AnyContext, contentHash).Return([]cid.Cid{locationDelegationCid, indexDelegationCid}, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: locationDelegationCid}, &url.URL{}).Return(locationDelegation, nil)
		mockStore.EXPECT().Find(testutil.AnyContext, cidlink.Link{Cid: indexDelegationCid}, &url.URL{}).Return(indexDelegation, nil)

		results, err := legacyClaims.Find(context.Background(), contentHash, []multicodec.Code{metadata.LocationCommitmentID, metadata.IndexClaimID})

		require.NoError(t, err)
		require.Len(t, results, 2)
	})

	t.Run("returns no error, but empty results, when the content hash is not found in the mapper", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		mockMapper.EXPECT().GetClaims(mock.Anything, mock.Anything).Return(nil, types.ErrKeyNotFound)

		results, err := legacyClaims.Find(context.Background(), testutil.RandomMultihash(), allTargetClaims)

		require.NoError(t, err)
		require.Empty(t, results)
	})

	t.Run("returns no error, but empty results, when claims are not found in the store", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		testCID := link.ToCID(testutil.RandomCID())

		mockMapper.EXPECT().GetClaims(mock.Anything, mock.Anything).Return([]cid.Cid{testCID}, nil)
		mockStore.EXPECT().Find(mock.Anything, mock.Anything, mock.Anything).Return(nil, types.ErrKeyNotFound)

		results, err := legacyClaims.Find(context.Background(), testutil.RandomMultihash(), allTargetClaims)

		require.NoError(t, err)
		require.Empty(t, results)
	})
}

func TestSynthetizeProviderResult(t *testing.T) {
	allTargetClaims := []multicodec.Code{metadata.LocationCommitmentID, metadata.IndexClaimID, metadata.EqualsClaimID}

	t.Run("location claim", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentLink := testutil.RandomCID()
		contentCid := link.ToCID(contentLink)
		contentHash := contentCid.Hash()
		spaceDID := testutil.RandomPrincipal().DID()

		locationClaim := cassert.Location.New(testutil.Service.DID().String(), cassert.LocationCaveats{
			Content:  ctypes.FromHash(contentHash),
			Location: []url.URL{*testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/blobs/%s", digestutil.Format(contentHash))))(t)},
			Space:    spaceDID,
		})
		locationDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[cassert.LocationCaveats]{locationClaim}))(t)

		result, err := legacyClaims.synthetizeProviderResult(link.ToCID(locationDelegation.Link()), locationDelegation, allTargetClaims)

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
		require.Equal(t, link.ToCID(locationDelegation.Link()), locMeta.Claim)

		blobUrl := testutil.Must(url.Parse("https://storacha.network/blobs/{blob}"))(t)
		blobProviderAddr := testutil.Must(maurl.FromURL(blobUrl))(t)
		require.Equal(t, blobProviderAddr, result.Provider.Addrs[0])
	})

	t.Run("filters out location claims", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		locationClaimCid := link.ToCID(testutil.RandomCID())
		locationClaim := testutil.RandomLocationDelegation()
		targetClaims := []multicodec.Code{metadata.IndexClaimID, metadata.EqualsClaimID}
		_, err := legacyClaims.synthetizeProviderResult(locationClaimCid, locationClaim, targetClaims)

		require.ErrorIs(t, err, ErrIgnoreFiltered)
	})

	t.Run("index claim", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentLink := testutil.RandomCID()
		indexLink := testutil.RandomCID()
		indexCid := link.ToCID(indexLink)

		indexClaim := cassert.Index.New(testutil.Service.DID().String(), cassert.IndexCaveats{
			Content: contentLink,
			Index:   indexLink,
		})
		indexDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[cassert.IndexCaveats]{indexClaim}))(t)

		result, err := legacyClaims.synthetizeProviderResult(link.ToCID(indexDelegation.Link()), indexDelegation, allTargetClaims)

		require.NoError(t, err)

		indexContextID := []byte(indexLink.Binary())
		require.Equal(t, indexContextID, result.ContextID)

		md := metadata.MetadataContext.New()
		require.NoError(t, md.UnmarshalBinary(result.Metadata))
		indexMeta := md.Get(metadata.IndexClaimID).(*metadata.IndexClaimMetadata)
		require.Equal(t, indexCid, indexMeta.Index)
		require.Equal(t, int64(*indexDelegation.Expiration()), indexMeta.Expiration)
		require.Equal(t, link.ToCID(indexDelegation.Link()), indexMeta.Claim)

		claimsUrl := testutil.Must(url.Parse("https://storacha.network/claims/{claim}"))(t)
		claimsProviderAddr := testutil.Must(maurl.FromURL(claimsUrl))(t)
		require.Equal(t, claimsProviderAddr, result.Provider.Addrs[0])
	})

	t.Run("filters out index claims", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		indexClaimCid := link.ToCID(testutil.RandomCID())
		indexClaim := testutil.RandomIndexDelegation()
		targetClaims := []multicodec.Code{metadata.LocationCommitmentID, metadata.EqualsClaimID}
		_, err := legacyClaims.synthetizeProviderResult(indexClaimCid, indexClaim, targetClaims)

		require.ErrorIs(t, err, ErrIgnoreFiltered)
	})

	t.Run("equals claim", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentHash := link.ToCID(testutil.RandomCID()).Hash()
		equalsLink := testutil.RandomCID()
		equalsCid := link.ToCID(equalsLink)

		equalsClaim := cassert.Equals.New(testutil.Service.DID().String(), cassert.EqualsCaveats{
			Content: ctypes.FromHash(contentHash),
			Equals:  equalsLink,
		})

		equalsDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[cassert.EqualsCaveats]{equalsClaim}))(t)

		result, err := legacyClaims.synthetizeProviderResult(link.ToCID(equalsDelegation.Link()), equalsDelegation, allTargetClaims)

		require.NoError(t, err)

		equalsContextID := []byte(equalsLink.Binary())
		require.Equal(t, equalsContextID, result.ContextID)

		md := metadata.MetadataContext.New()
		require.NoError(t, md.UnmarshalBinary(result.Metadata))
		equalsMeta := md.Get(metadata.EqualsClaimID).(*metadata.EqualsClaimMetadata)
		require.Equal(t, equalsCid, equalsMeta.Equals)
		require.Equal(t, int64(*equalsDelegation.Expiration()), equalsMeta.Expiration)
		require.Equal(t, link.ToCID(equalsDelegation.Link()), equalsMeta.Claim)

		claimsUrl := testutil.Must(url.Parse("https://storacha.network/claims/{claim}"))(t)
		claimsProviderAddr := testutil.Must(maurl.FromURL(claimsUrl))(t)
		require.Equal(t, claimsProviderAddr, result.Provider.Addrs[0])
	})

	t.Run("filters out equals claims", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		equalsClaimCid := link.ToCID(testutil.RandomCID())
		equalsClaim := testutil.RandomEqualsDelegation()
		targetClaims := []multicodec.Code{metadata.LocationCommitmentID, metadata.IndexClaimID}
		_, err := legacyClaims.synthetizeProviderResult(equalsClaimCid, equalsClaim, targetClaims)

		require.ErrorIs(t, err, ErrIgnoreFiltered)
	})

	t.Run("unsupported claim", func(t *testing.T) {
		mockMapper := NewMockContentToClaimsMapper(t)
		mockStore := contentclaims.NewMockContentClaimsFinder(t)
		legacyClaims := testutil.Must(NewLegacyClaimsStore([]ContentToClaimsMapper{mockMapper}, mockStore, "https://storacha.network/claims/{claim}"))(t)

		contentHash := link.ToCID(testutil.RandomCID()).Hash()

		partitionClaim := cassert.Partition.New(testutil.Service.DID().String(), cassert.PartitionCaveats{
			Content: ctypes.FromHash(contentHash),
			Blocks:  nil,
			Parts:   nil,
		})

		partitionDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[cassert.PartitionCaveats]{partitionClaim}))(t)

		_, err := legacyClaims.synthetizeProviderResult(link.ToCID(partitionDelegation.Link()), partitionDelegation, allTargetClaims)

		require.Error(t, err)
	})
}
