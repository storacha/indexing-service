package service

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	adm "github.com/storacha/go-capabilities/pkg/assert/datamodel"
	"github.com/storacha/go-metadata"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/result/ok"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil/mocks"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockBlobIndexLookup := mocks.NewMockBlobIndexLookup(t)
		mockClaimsService := mocks.NewMockContentClaimsService(t)
		mockProviderIndex := mocks.NewMockProviderIndex(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		contentLink := testutil.RandomCID()
		contentHash := contentLink.(cidlink.Link).Hash()

		ctx := context.Background()

		// content will have a location claim, an index claim and an equals claim
		locationDelegationCid, locationDelegation, locationProviderResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		indexDelegationCid, indexDelegation, indexResult, indexCid, index := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)
		equalsDelegationCid, equalsDelegation, equalsResult, equivalentCid := buildTestEqualsClaim(t, contentLink.(cidlink.Link), providerAddr)

		contentResults := []model.ProviderResult{locationProviderResult, indexResult, equalsResult}

		// expect a call to find records for content
		mockProviderIndex.EXPECT().Find(ctx, providerindex.QueryKey{
			Hash:         contentHash,
			TargetClaims: []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return(contentResults, nil)

		// the results for content should make the IndexingService ask for all claims
		locationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", locationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(ctx, locationDelegationCid, locationClaimUrl).Return(locationDelegation, nil)
		indexClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", indexDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(ctx, indexDelegationCid, indexClaimUrl).Return(indexDelegation, nil)
		equalsClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", equalsDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(ctx, equalsDelegationCid, equalsClaimUrl).Return(equalsDelegation, nil)

		// then attempt to find records for the index referenced in the index claim
		indexLocationDelegationCid, indexLocationDelegation, indexLocationProviderResult := buildTestLocationClaim(t, indexCid, providerAddr)

		mockProviderIndex.EXPECT().Find(ctx, providerindex.QueryKey{
			Hash:         indexCid.Hash(),
			TargetClaims: []multicodec.Code{metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{indexLocationProviderResult}, nil)

		// fetch the index's location claim
		indexLocationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", indexLocationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(ctx, indexLocationDelegationCid, indexLocationClaimUrl).Return(indexLocationDelegation, nil)

		// and finally call the blob index lookup service to fetch the actual index
		indexBlobUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/blobs/%s", digestutil.Format(indexCid.Hash()))))(t)
		mockBlobIndexLookup.EXPECT().Find(ctx, types.EncodedContextID(indexLocationProviderResult.ContextID), indexResult, indexBlobUrl, (*metadata.Range)(nil)).Return(index, nil)

		// similarly, the equals claim should make the service ask for the location claim of the equivalent content
		equalsLocationDelegationCid, equalsLocationDelegation, equalsLocationProviderResult := buildTestLocationClaim(t, equivalentCid, providerAddr)

		mockProviderIndex.EXPECT().Find(ctx, providerindex.QueryKey{
			Hash:         equivalentCid.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{equalsLocationProviderResult}, nil)

		// and fetch the equivalent content's location claim
		equalsLocationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", equalsLocationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(ctx, equalsLocationDelegationCid, equalsLocationClaimUrl).Return(equalsLocationDelegation, nil)

		service := NewIndexingService(mockBlobIndexLookup, mockClaimsService, peer.AddrInfo{ID: testutil.RandomPeer()}, mockProviderIndex)

		result, err := service.Query(ctx, types.Query{Hashes: []mh.Multihash{contentHash}})

		require.NoError(t, err)

		expectedClaims := map[cid.Cid]delegation.Delegation{
			locationDelegationCid.Cid:       locationDelegation,
			indexDelegationCid.Cid:          indexDelegation,
			equalsDelegationCid.Cid:         equalsDelegation,
			indexLocationDelegationCid.Cid:  indexLocationDelegation,
			equalsLocationDelegationCid.Cid: equalsLocationDelegation,
		}
		expectedIndexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
		expectedIndexes.Set(types.EncodedContextID(indexLocationProviderResult.ContextID), index)
		expectedResult := testutil.Must(queryresult.Build(expectedClaims, expectedIndexes))(t)

		require.ElementsMatch(t, expectedResult.Claims(), result.Claims())
		require.ElementsMatch(t, expectedResult.Indexes(), result.Indexes())
	})
}

func buildTestLocationClaim(t *testing.T, contentLink cidlink.Link, providerAddr *peer.AddrInfo) (cidlink.Link, delegation.Delegation, model.ProviderResult) {
	locationClaim := assert.Location.New(testutil.Service.DID().String(), assert.LocationCaveats{
		Content:  testutil.Must(assert.Digest(adm.DigestModel{Digest: contentLink.Hash()}))(t),
		Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
	})

	locationDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[assert.LocationCaveats]{locationClaim}))(t)
	locationDelegationCid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(locationDelegation)))(t)))(t)

	locationMetadata := metadata.LocationCommitmentMetadata{
		Shard: &contentLink.Cid,
		Claim: locationDelegationCid,
	}

	locationProviderResult := model.ProviderResult{
		ContextID: testutil.Must(types.ContextID{Hash: contentLink.Hash()}.ToEncoded())(t),
		Metadata:  testutil.Must(locationMetadata.MarshalBinary())(t),
		Provider:  providerAddr,
	}

	return cidlink.Link{Cid: locationDelegationCid}, locationDelegation, locationProviderResult
}

func buildTestIndexClaim(t *testing.T, contentLink cidlink.Link, providerAddr *peer.AddrInfo) (cidlink.Link, delegation.Delegation, model.ProviderResult, cidlink.Link, blobindex.ShardedDagIndexView) {
	indexHash, index := testutil.RandomShardedDagIndexView(32)
	indexLink := cidlink.Link{Cid: cid.NewCidV1(uint64(multicodec.Car), indexHash)}
	indexClaim := assert.Index.New(testutil.Service.DID().String(), assert.IndexCaveats{
		Content: contentLink,
		Index:   indexLink,
	})

	indexDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[assert.IndexCaveats]{indexClaim}))(t)
	indexDelegationCid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(indexDelegation)))(t)))(t)

	indexMetadata := metadata.IndexClaimMetadata{
		Index: indexLink.Cid,
		Claim: indexDelegationCid,
	}

	indexResult := model.ProviderResult{
		ContextID: testutil.Must(types.ContextID{Hash: contentLink.Hash()}.ToEncoded())(t),
		Metadata:  testutil.Must(indexMetadata.MarshalBinary())(t),
		Provider:  providerAddr,
	}

	return cidlink.Link{Cid: indexDelegationCid}, indexDelegation, indexResult, indexLink, index
}

func buildTestEqualsClaim(t *testing.T, contentLink cidlink.Link, providerAddr *peer.AddrInfo) (cidlink.Link, delegation.Delegation, model.ProviderResult, cidlink.Link) {
	equivalentCid := testutil.RandomCID()
	equalsClaim := assert.Equals.New(testutil.Service.DID().String(), assert.EqualsCaveats{
		Content: testutil.Must(assert.Digest(adm.DigestModel{Digest: contentLink.Hash()}))(t),
		Equals:  equivalentCid,
	})

	equalsDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[assert.EqualsCaveats]{equalsClaim}))(t)
	equalsDelegationCid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(equalsDelegation)))(t)))(t)

	equalsMetadata := metadata.EqualsClaimMetadata{
		Equals: equivalentCid.(cidlink.Link).Cid,
		Claim:  equalsDelegationCid,
	}

	equalsProviderResults := model.ProviderResult{
		ContextID: testutil.Must(types.ContextID{Hash: contentLink.Hash()}.ToEncoded())(t),
		Metadata:  testutil.Must(equalsMetadata.MarshalBinary())(t),
		Provider:  providerAddr,
	}

	return cidlink.Link{Cid: equalsDelegationCid}, equalsDelegation, equalsProviderResults, equivalentCid.(cidlink.Link)
}

func TestPublishClaim(t *testing.T) {
	t.Run("does not publish unknown claims", func(t *testing.T) {
		claim, err := delegation.Delegate(
			testutil.Alice,
			testutil.Bob,
			[]ucan.Capability[ok.Unit]{
				ucan.NewCapability("unknown/claim", testutil.Mallory.DID().String(), ok.Unit{}),
			},
		)
		require.NoError(t, err)
		err = Publish(context.Background(), nil, nil, nil, peer.AddrInfo{}, claim)
		require.ErrorIs(t, err, ErrUnrecognizedClaim)
	})
}

func TestCacheClaim(t *testing.T) {
	t.Run("does not cache unknown claims", func(t *testing.T) {
		claim, err := delegation.Delegate(
			testutil.Alice,
			testutil.Bob,
			[]ucan.Capability[ok.Unit]{
				ucan.NewCapability("unknown/claim", testutil.Mallory.DID().String(), ok.Unit{}),
			},
		)
		require.NoError(t, err)
		err = Cache(context.Background(), nil, nil, nil, peer.AddrInfo{}, claim)
		require.ErrorIs(t, err, ErrUnrecognizedClaim)
	})
}

func TestUrlForResource(t *testing.T) {
	const addrBase = "/dns/storacha.network/https/http-path/"
	testCases := []struct {
		name        string
		addrs       []ma.Multiaddr
		placeholder string
		id          string
		expectedUrl string
		expectErr   bool
	}{
		{
			name: "happy path",
			addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/claims/{claim}")))(t),
			},
			placeholder: "{claim}",
			id:          "123",
			expectedUrl: "https://storacha.network/claims/123",
			expectErr:   false,
		},
		{
			name: "multiple addresses, uses the first one that contains the placeholder",
			addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/blobs/{blob}")))(t),
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/claims1/{claim}")))(t),
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/claims2/{claim}")))(t),
			},
			placeholder: "{claim}",
			id:          "123",
			expectedUrl: "https://storacha.network/claims1/123",
			expectErr:   false,
		},
		{
			name:        "no addresses in peer addr info",
			addrs:       []ma.Multiaddr{},
			placeholder: "{claim}",
			expectedUrl: "",
			expectErr:   true,
		},
		{
			name: "no address contains the placeholder",
			addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/claims/{foo}")))(t),
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/claims/{bar}")))(t),
				testutil.Must(ma.NewMultiaddr(addrBase + url.PathEscape("/claims/{baz}")))(t),
			},
			placeholder: "{claim}",
			expectedUrl: "",
			expectErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := peer.AddrInfo{
				Addrs: tc.addrs,
			}
			u, err := urlForResource(provider, tc.placeholder, tc.id)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedUrl, u.String())
			}
		})
	}
}
