package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/result/ok"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/providerindex"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		contentLink := testutil.RandomCID()
		contentHash := contentLink.(cidlink.Link).Hash()

		// content will have a location claim, an index claim and an equals claim
		locationDelegationCid, locationDelegation, locationProviderResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		indexDelegationCid, indexDelegation, indexResult, indexCid, index := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)
		equalsDelegationCid, equalsDelegation, equalsResult, equivalentCid := buildTestEqualsClaim(t, contentLink.(cidlink.Link), providerAddr)

		contentResults := []model.ProviderResult{locationProviderResult, indexResult, equalsResult}

		// expect a call to find records for content
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         contentHash,
			TargetClaims: []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return(contentResults, nil)

		// the results for content should make the IndexingService ask for all claims
		locationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", locationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, locationDelegationCid, locationClaimUrl).Return(locationDelegation, nil)
		indexClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", indexDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, indexDelegationCid, indexClaimUrl).Return(indexDelegation, nil)
		equalsClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", equalsDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, equalsDelegationCid, equalsClaimUrl).Return(equalsDelegation, nil)

		// then attempt to find records for the index referenced in the index claim
		indexLocationDelegationCid, indexLocationDelegation, indexLocationProviderResult := buildTestLocationClaim(t, indexCid, providerAddr)

		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexCid.Hash(),
			TargetClaims: []multicodec.Code{metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{indexLocationProviderResult}, nil)

		// fetch the index's location claim
		indexLocationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", indexLocationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, indexLocationDelegationCid, indexLocationClaimUrl).Return(indexLocationDelegation, nil)

		// and finally call the blob index lookup service to fetch the actual index
		indexBlobUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/blobs/%s", digestutil.Format(indexCid.Hash()))))(t)
		mockBlobIndexLookup.EXPECT().Find(testutil.AnyContext, types.EncodedContextID(indexLocationProviderResult.ContextID), indexResult, indexBlobUrl, (*metadata.Range)(nil)).Return(index, nil)

		// similarly, the equals claim should make the service ask for the location claim of the equivalent content
		equalsLocationDelegationCid, equalsLocationDelegation, equalsLocationProviderResult := buildTestLocationClaim(t, equivalentCid, providerAddr)

		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         equivalentCid.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{equalsLocationProviderResult}, nil)

		// and fetch the equivalent content's location claim
		equalsLocationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", equalsLocationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, equalsLocationDelegationCid, equalsLocationClaimUrl).Return(equalsLocationDelegation, nil)

		service := NewIndexingService(mockBlobIndexLookup, mockClaimsService, peer.AddrInfo{ID: testutil.RandomPeer()}, mockProviderIndex)

		result, err := service.Query(context.Background(), types.Query{Hashes: []mh.Multihash{contentHash}})

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

	t.Run("targets the right claims depending on query type", func(t *testing.T) {
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)

		service := NewIndexingService(mockBlobIndexLookup, mockClaimsService, peer.AddrInfo{ID: testutil.RandomPeer()}, mockProviderIndex)

		contentHash := testutil.RandomMultihash()

		t.Run("standard query: location, index and equals", func(t *testing.T) {
			query := types.Query{
				Type:   types.QueryTypeStandard,
				Hashes: []mh.Multihash{contentHash},
			}

			expectedQueryKey := providerindex.QueryKey{
				Hash:         contentHash,
				TargetClaims: []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
			}

			mockProviderIndex.EXPECT().Find(testutil.AnyContext, expectedQueryKey).Return([]model.ProviderResult{}, nil)

			_, err := service.Query(context.Background(), query)
			require.NoError(t, err)
		})

		t.Run("location query: location only", func(t *testing.T) {
			query := types.Query{
				Type:   types.QueryTypeLocation,
				Hashes: []mh.Multihash{contentHash},
			}

			expectedQueryKey := providerindex.QueryKey{
				Hash:         contentHash,
				TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
			}

			mockProviderIndex.EXPECT().Find(testutil.AnyContext, expectedQueryKey).Return([]model.ProviderResult{}, nil)

			_, err := service.Query(context.Background(), query)
			require.NoError(t, err)
		})

		t.Run("index_or_location query: location and index", func(t *testing.T) {
			query := types.Query{
				Type:   types.QueryTypeIndexOrLocation,
				Hashes: []mh.Multihash{contentHash},
			}

			expectedQueryKey := providerindex.QueryKey{
				Hash:         contentHash,
				TargetClaims: []multicodec.Code{metadata.IndexClaimID, metadata.LocationCommitmentID},
			}

			mockProviderIndex.EXPECT().Find(testutil.AnyContext, expectedQueryKey).Return([]model.ProviderResult{}, nil)

			_, err := service.Query(context.Background(), query)
			require.NoError(t, err)
		})
	})

	t.Run("returns error when ProviderIndex service errors", func(t *testing.T) {
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)

		contentLink := testutil.RandomCID()
		contentHash := contentLink.(cidlink.Link).Hash()

		// expect a call to find records for content
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         contentHash,
			TargetClaims: []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{}, errors.New("provider index error"))

		service := NewIndexingService(mockBlobIndexLookup, mockClaimsService, peer.AddrInfo{ID: testutil.RandomPeer()}, mockProviderIndex)

		_, err := service.Query(context.Background(), types.Query{Hashes: []mh.Multihash{contentHash}})

		require.Error(t, err)
	})

	t.Run("returns error when ContentClaims service errors", func(t *testing.T) {
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		contentLink := testutil.RandomCID()
		contentHash := contentLink.(cidlink.Link).Hash()

		// content will have a location claim
		locationDelegationCid, _, locationProviderResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)

		contentResults := []model.ProviderResult{locationProviderResult}

		// expect a call to find records for content
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         contentHash,
			TargetClaims: []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return(contentResults, nil)

		// the results for content should make the IndexingService ask for the location claim, but that will fail
		locationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", locationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, locationDelegationCid, locationClaimUrl).Return(nil, errors.New("content claims service error"))
		service := NewIndexingService(mockBlobIndexLookup, mockClaimsService, peer.AddrInfo{ID: testutil.RandomPeer()}, mockProviderIndex)

		_, err := service.Query(context.Background(), types.Query{Hashes: []mh.Multihash{contentHash}})

		require.Error(t, err)
	})

	t.Run("returns error when BlobIndexLookup service errors", func(t *testing.T) {
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		contentLink := testutil.RandomCID()
		contentHash := contentLink.(cidlink.Link).Hash()

		// content will have a location claim and an index claim
		locationDelegationCid, locationDelegation, locationProviderResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		indexDelegationCid, indexDelegation, indexResult, indexCid, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		contentResults := []model.ProviderResult{locationProviderResult, indexResult}

		// expect a call to find records for content
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         contentHash,
			TargetClaims: []multicodec.Code{metadata.EqualsClaimID, metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return(contentResults, nil)

		// the results for content should make the IndexingService ask for both claims
		locationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", locationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, locationDelegationCid, locationClaimUrl).Return(locationDelegation, nil)
		indexClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", indexDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, indexDelegationCid, indexClaimUrl).Return(indexDelegation, nil)

		// then attempt to find records for the index referenced in the index claim
		indexLocationDelegationCid, indexLocationDelegation, indexLocationProviderResult := buildTestLocationClaim(t, indexCid, providerAddr)

		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexCid.Hash(),
			TargetClaims: []multicodec.Code{metadata.IndexClaimID, metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{indexLocationProviderResult}, nil)

		// fetch the index's location claim
		indexLocationClaimUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/claims/%s", indexLocationDelegationCid.String())))(t)
		mockClaimsService.EXPECT().Find(testutil.AnyContext, indexLocationDelegationCid, indexLocationClaimUrl).Return(indexLocationDelegation, nil)

		// and finally call the blob index lookup service to fetch the actual index, which will fail
		indexBlobUrl := testutil.Must(url.Parse(fmt.Sprintf("https://storacha.network/blobs/%s", digestutil.Format(indexCid.Hash()))))(t)
		mockBlobIndexLookup.EXPECT().Find(testutil.AnyContext, types.EncodedContextID(indexLocationProviderResult.ContextID), indexResult, indexBlobUrl, (*metadata.Range)(nil)).Return(nil, errors.New("blob index lookup error"))

		service := NewIndexingService(mockBlobIndexLookup, mockClaimsService, peer.AddrInfo{ID: testutil.RandomPeer()}, mockProviderIndex)

		_, err := service.Query(context.Background(), types.Query{Hashes: []mh.Multihash{contentHash}})

		require.Error(t, err)
	})
}

func buildTestLocationClaim(t *testing.T, contentLink cidlink.Link, providerAddr *peer.AddrInfo) (cidlink.Link, delegation.Delegation, model.ProviderResult) {
	locationClaim := cassert.Location.New(testutil.Service.DID().String(), cassert.LocationCaveats{
		Content:  ctypes.FromHash(contentLink.Hash()),
		Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
	})

	locationDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[cassert.LocationCaveats]{locationClaim}))(t)
	locationDelegationCid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(locationDelegation)))(t)))(t)

	locationMetadata := metadata.LocationCommitmentMetadata{
		Shard:      &contentLink.Cid,
		Claim:      locationDelegationCid,
		Expiration: time.Now().Add(time.Hour).Unix(),
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
	indexClaim := cassert.Index.New(testutil.Service.DID().String(), cassert.IndexCaveats{
		Content: contentLink,
		Index:   indexLink,
	})

	indexDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[cassert.IndexCaveats]{indexClaim}))(t)
	indexDelegationCid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(indexDelegation)))(t)))(t)

	indexMetadata := metadata.IndexClaimMetadata{
		Index:      indexLink.Cid,
		Claim:      indexDelegationCid,
		Expiration: time.Now().Add(time.Hour).Unix(),
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
	equalsClaim := cassert.Equals.New(testutil.Service.DID().String(), cassert.EqualsCaveats{
		Content: ctypes.FromHash(contentLink.Hash()),
		Equals:  equivalentCid,
	})

	equalsDelegation := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[cassert.EqualsCaveats]{equalsClaim}))(t)
	equalsDelegationCid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(equalsDelegation)))(t)))(t)

	equalsMetadata := metadata.EqualsClaimMetadata{
		Equals:     equivalentCid.(cidlink.Link).Cid,
		Claim:      equalsDelegationCid,
		Expiration: time.Now().Add(time.Hour).Unix(),
	}

	equalsProviderResults := model.ProviderResult{
		ContextID: testutil.Must(types.ContextID{Hash: contentLink.Hash()}.ToEncoded())(t),
		Metadata:  testutil.Must(equalsMetadata.MarshalBinary())(t),
		Provider:  providerAddr,
	}

	return cidlink.Link{Cid: equalsDelegationCid}, equalsDelegation, equalsProviderResults, equivalentCid.(cidlink.Link)
}

func TestPublishIndexClaim(t *testing.T) {
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

	t.Run("successful publishing the index claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}
		// content will have a location claim, an index claim
		locationDelegationCid, locationDelegation, locationResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		_, indexDelegation, _, indexLink, shardIndex := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// expect a call to cache the index claim using claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// expect a call to find records for index location commitment
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{locationResult}, nil)

		// expect the claim service to be called for each result from providerIndex.Find
		mockClaimsService.EXPECT().Find(
			testutil.AnyContext, locationDelegationCid, mock.AnythingOfType("*url.URL"),
		).Return(locationDelegation, nil)

		// expect the blob index lookup service to be called once to fetch the shard index
		mockBlobIndexLookup.EXPECT().Find(
			testutil.AnyContext, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(shardIndex, nil)

		// expect the index claim to be published
		digests := bytemap.NewByteMap[mh.Multihash, struct{}](-1)
		for _, slices := range shardIndex.Shards().Iterator() {
			for d := range slices.Iterator() {
				digests.Set(d, struct{}{})
			}
		}
		mockProviderIndex.EXPECT().Publish(testutil.AnyContext, *providerAddr, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)
		require.NoError(t, err)
	})

	t.Run("error when index claim has no capabilities", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a claim with no capabilities
		claim, err := delegation.Delegate(
			testutil.Alice,
			testutil.Bob,
			[]ucan.Capability[ucan.NoCaveats]{},
		)
		require.NoError(t, err)

		// Attempt to publish the claim
		err = Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, claim)

		// Expect an error indicating missing capabilities
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing capabilities in claim")
	})

	t.Run("error when reading index claim caveats fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a faulty index claim that will cause the Read method to fail
		c := ucan.NewCapability("assert/index", testutil.Alice.DID().String(), ucan.NoCaveats{})
		faultyIndexClaim, err := delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[ucan.NoCaveats]{c})
		require.NoError(t, err)

		// Attempt to publish the claim
		err = Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, faultyIndexClaim)

		// Expect an error indicating a problem with reading the index claim caveats
		require.Error(t, err)
		require.Contains(t, err.Error(), "reading index claim data: missing required fields: content,index")
	})

	t.Run("error when caching the claim in claims.Publish fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a valid index claim
		_, indexDelegation, _, _, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate an error when caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(fmt.Errorf("failed to cache claim"))

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)

		// Expect an error indicating a problem with caching the claim
		require.Error(t, err)
		require.Contains(t, err.Error(), "caching index claim with claim lookup: failed to cache claim")
	})

	t.Run("error when no location commitments found", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a valid index claim
		_, indexDelegation, _, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate an empty result set from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{}, nil) // no location commitments found

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)

		// Expect an error indicating no location commitments found
		require.Error(t, err)
		require.Contains(t, err.Error(), "no location commitments found for index")
	})

	t.Run("error when finding location commitments fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a valid index claim
		_, indexDelegation, _, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate an error when finding location commitments
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{}, fmt.Errorf("failed to find location commitments"))

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)

		// Expect an error indicating a problem with finding location commitments
		require.Error(t, err)
		require.Contains(t, err.Error(), "finding location commitment: failed to find location commitments")
	})

	t.Run("error when fetching the blob index fails to decode location commitment metadata", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a valid index claim
		_, indexDelegation, _, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{{}}, nil)

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)

		// Expect an error indicating a problem with fetching the blob index
		require.Error(t, err)
		require.Contains(t, err.Error(), "fetching blob index: decoding location commitment metadata")
	})

	t.Run("error when fetching the blob index fails with invalid metadata type", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a valid index claim
		_, indexDelegation, indexResult, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{indexResult}, nil) // this is the wrong claim type

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)

		// Expect an error indicating a problem with the metadata type
		require.Error(t, err)
		require.Contains(t, err.Error(), "fetching blob index: metadata is not expected type")
	})

	t.Run("error when fetching the blob index fails to build the retrieval URL", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				// Only the claim URL is provided, it is missing the Blob URL which is used to build the retrieval URL
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// content will have a location claim, an index claim
		_, _, locationResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		_, indexDelegation, _, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{locationResult}, nil)

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)

		// Expect an error indicating a problem with building the retrieval URL
		require.Error(t, err)
		require.Contains(t, err.Error(), "fetching blob index: building retrieval URL")
	})

	t.Run("error when fetching the blob index fails to build the claim URL", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				// Only the blob URL is provided, it is missing the claim URL which is used to build the claim URL
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		// content will have a location claim, an index claim
		_, _, locationResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		_, indexDelegation, _, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{locationResult}, nil)

		// Simulate a successful result from blobIndexLookup.Find
		mockBlobIndexLookup.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "fetching blob index: verifying claim: building claim URL: no {claim} endpoint found")
	})

	t.Run("error when fetching the blob index fails to find the location commitment claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		// content will have a location claim, an index claim
		_, _, locationResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		_, indexDelegation, _, indexLink, _ := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{locationResult}, nil)

		// Simulate a successful result from blobIndexLookup.Find
		mockBlobIndexLookup.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

		// Simulate a failure from claims.Find
		mockClaimsService.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("failed to find claim"))

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "fetching blob index: verifying claim: failed to find claim")
	})

	t.Run("error when publishing the claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		// content will have a location claim, an index claim
		_, locationDelegation, locationResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		_, indexDelegation, _, indexLink, shardIndex := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{locationResult}, nil)

		// Simulate a successful result from blobIndexLookup.Find
		mockBlobIndexLookup.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(shardIndex, nil)

		// Simulate a failure from claims.Find
		mockClaimsService.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything).Return(locationDelegation, nil)

		// Simulate a failure from provIndex.Publish
		mockProviderIndex.EXPECT().Publish(testutil.AnyContext, *providerAddr, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("failed to publish claim"))

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "publishing index claim: failed to publish claim")
	})

	t.Run("error when publishing the claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
			},
		}

		// content will have a location claim, an index claim
		_, locationDelegation, locationResult := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		_, indexDelegation, _, indexLink, shardIndex := buildTestIndexClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate caching the claim in claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, indexDelegation).Return(nil)

		// Simulate a successful result from provIndex.Find
		mockProviderIndex.EXPECT().Find(testutil.AnyContext, providerindex.QueryKey{
			Hash:         indexLink.Hash(),
			TargetClaims: []multicodec.Code{metadata.LocationCommitmentID},
		}).Return([]model.ProviderResult{locationResult}, nil)

		// Simulate a successful result from blobIndexLookup.Find
		mockBlobIndexLookup.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(shardIndex, nil)

		// Simulate a failure from claims.Find
		mockClaimsService.EXPECT().Find(testutil.AnyContext, mock.Anything, mock.Anything).Return(locationDelegation, nil)

		// Simulate a failure from provIndex.Publish
		mockProviderIndex.EXPECT().Publish(testutil.AnyContext, *providerAddr, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("failed to publish claim"))

		// Attempt to publish the claim
		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, indexDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "publishing index claim: failed to publish claim")
	})

}

func TestPublishEqualsClaim(t *testing.T) {
	t.Run("successful publishing the equals claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// content will have an equals claim
		_, equalsDelegation, _, _ := buildTestEqualsClaim(t, contentLink.(cidlink.Link), providerAddr)

		// expect a call to cache the equals claim using claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, equalsDelegation).Return(nil)

		// Simulate a successful result from provIndex.Publish
		mockProviderIndex.EXPECT().Publish(testutil.AnyContext, *providerAddr, mock.Anything, mock.Anything, mock.Anything).Return(nil)

		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, equalsDelegation)
		require.NoError(t, err)
	})

	t.Run("error when reading index claim caveats fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a faulty equals claim that will cause the Read method to fail
		c := ucan.NewCapability("assert/equals", testutil.Alice.DID().String(), ucan.NoCaveats{})
		faultyEqualsClaim, err := delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[ucan.NoCaveats]{c})
		require.NoError(t, err)

		// Attempt to publish the claim
		err = Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, faultyEqualsClaim)

		// Expect an error indicating a problem with reading the claim caveats
		require.Error(t, err)
		require.Contains(t, err.Error(), "reading equals claim data: missing required fields: content,equals")
	})

	t.Run("error when publishing claim in claims service fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// content will have an equals claim
		_, equalsDelegation, _, _ := buildTestEqualsClaim(t, contentLink.(cidlink.Link), providerAddr)

		// Simulate a failure from claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, equalsDelegation).Return(fmt.Errorf("failed to publish claim"))

		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, equalsDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "caching equals claim with claim service: failed to publish claim")
	})

	t.Run("error when publishing claim in claims service fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()

		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fblobs%2F%7Bblob%7D"))(t),
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// content will have an equals claim
		_, equalsDelegation, _, _ := buildTestEqualsClaim(t, contentLink.(cidlink.Link), providerAddr)

		// expect a call to cache the equals claim using claims.Publish
		mockClaimsService.EXPECT().Publish(testutil.AnyContext, equalsDelegation).Return(nil)

		// Simulate a failure from provIndex.Publish
		mockProviderIndex.EXPECT().Publish(testutil.AnyContext, *providerAddr, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("failed to publish claim"))

		err := Publish(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, equalsDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "publishing equals claim: failed to publish claim")
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

	t.Run("successful caching for assert/location claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		contentLink := testutil.RandomCID()
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		_, locationDelegation, _ := buildTestLocationClaim(t, contentLink.(cidlink.Link), providerAddr)
		mockClaimsService.EXPECT().Cache(testutil.AnyContext, locationDelegation).Return(nil)

		anyContextID := mock.AnythingOfType("string")
		anyMultihash := mock.AnythingOfType("iter.Seq[github.com/multiformats/go-multihash.Multihash]")
		anyMetadata := mock.AnythingOfType("metadata.Metadata")
		mockProviderIndex.EXPECT().Cache(testutil.AnyContext, *providerAddr, anyContextID, anyMultihash, anyMetadata).Return(nil)

		err := Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, locationDelegation)
		require.NoError(t, err)
	})

	t.Run("returns error when claim has no capabilities", func(t *testing.T) {
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a claim with no capabilities
		claim, err := delegation.Delegate(
			testutil.Alice,
			testutil.Bob,
			[]ucan.Capability[ok.Unit]{}, // No capabilities
		)
		require.NoError(t, err)

		err = Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, claim)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("missing capabilities in claim: %s", claim.Link()))
	})

	t.Run("returns error when reading location claim caveats fails", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		// Create a faulty location claim that will cause the Read method to fail
		c := ucan.NewCapability("assert/location", testutil.Alice.DID().String(), ucan.NoCaveats{})
		faultyLocationClaim, err := delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[ucan.NoCaveats]{c})
		require.NoError(t, err)

		// Attempt to cache the claim, which will fail
		err = Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, faultyLocationClaim)
		require.Error(t, err)
		require.Contains(t, err.Error(), "reading index claim data")
	})

	t.Run("handle the expiration correctly and cache the claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		locationClaim := cassert.Location.New(testutil.Service.DID().String(), cassert.LocationCaveats{
			Content:  ctypes.FromHash([]byte{1, 2, 3}),
			Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
		})
		locationDelegation := testutil.Must(delegation.Delegate(
			testutil.Service,
			testutil.Alice,
			[]ucan.Capability[cassert.LocationCaveats]{locationClaim},
			// set the expiration to 1 hour in the future
			delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix())),
		))(t)

		anyContextID := mock.AnythingOfType("string")
		anyMultihash := mock.AnythingOfType("iter.Seq[github.com/multiformats/go-multihash.Multihash]")
		anyMetadata := mock.AnythingOfType("metadata.Metadata")
		mockProviderIndex.EXPECT().Cache(testutil.AnyContext, *providerAddr, anyContextID, anyMultihash, anyMetadata).Return(nil)
		mockClaimsService.EXPECT().Cache(testutil.AnyContext, locationDelegation).Return(nil)

		// Cache the claim with expiration
		err := Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, locationDelegation)
		require.NoError(t, err)
	})

	t.Run("handle a delegation with a range in the caveats and cache the claim", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		locationClaim := cassert.Location.New(testutil.Service.DID().String(), cassert.LocationCaveats{
			Content:  ctypes.FromHash([]byte{1, 2, 3}),
			Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
			// set the range
			Range: &cassert.Range{Offset: 0, Length: &[]uint64{3}[0]},
		})
		locationDelegation := testutil.Must(delegation.Delegate(
			testutil.Service,
			testutil.Alice,
			[]ucan.Capability[cassert.LocationCaveats]{locationClaim},
			delegation.WithExpiration(int(time.Now().Add(time.Hour).Unix())),
		))(t)

		anyContextID := mock.AnythingOfType("string")
		anyMultihash := mock.AnythingOfType("iter.Seq[github.com/multiformats/go-multihash.Multihash]")
		anyMetadata := mock.AnythingOfType("metadata.Metadata")
		mockProviderIndex.EXPECT().Cache(testutil.AnyContext, *providerAddr, anyContextID, anyMultihash, anyMetadata).Return(nil)
		mockClaimsService.EXPECT().Cache(testutil.AnyContext, locationDelegation).Return(nil)

		// Cache the claim with expiration
		err := Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, locationDelegation)
		require.NoError(t, err)
	})

	t.Run("handle the error from the claims.Cache function call", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		locationClaim := cassert.Location.New(testutil.Service.DID().String(), cassert.LocationCaveats{
			Content:  ctypes.FromHash([]byte{1, 2, 3}),
			Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
		})
		locationDelegation := testutil.Must(delegation.Delegate(
			testutil.Service,
			testutil.Alice,
			[]ucan.Capability[cassert.LocationCaveats]{locationClaim},
		))(t)

		// mock the error from the claims.Cache function call
		mockClaimsService.EXPECT().Cache(testutil.AnyContext, locationDelegation).Return(
			errors.New("something went wrong while caching claim in claims.Cache"),
		)

		// Attempt to cache the claim
		err := Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, locationDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "something went wrong while caching claim in claims.Cache")
	})

	t.Run("handle the error from the providerIndex.Cache function call", func(t *testing.T) {
		mockClaimsService := contentclaims.NewMockContentClaimsService(t)
		mockProviderIndex := providerindex.NewMockProviderIndex(t)
		mockBlobIndexLookup := blobindexlookup.NewMockBlobIndexLookup(t)
		providerAddr := &peer.AddrInfo{
			Addrs: []ma.Multiaddr{
				testutil.Must(ma.NewMultiaddr("/dns/storacha.network/tls/http/http-path/%2Fclaims%2F%7Bclaim%7D"))(t),
			},
		}

		locationClaim := cassert.Location.New(testutil.Service.DID().String(), cassert.LocationCaveats{
			Content:  ctypes.FromHash([]byte{1, 2, 3}),
			Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
		})
		locationDelegation := testutil.Must(delegation.Delegate(
			testutil.Service,
			testutil.Alice,
			[]ucan.Capability[cassert.LocationCaveats]{locationClaim},
		))(t)

		mockClaimsService.EXPECT().Cache(testutil.AnyContext, locationDelegation).Return(nil)
		// mock the error from the providerIndex.Cache function call
		anyContextID := mock.AnythingOfType("string")
		anyMultihash := mock.AnythingOfType("iter.Seq[github.com/multiformats/go-multihash.Multihash]")
		anyMetadata := mock.AnythingOfType("metadata.Metadata")
		mockProviderIndex.EXPECT().Cache(testutil.AnyContext, *providerAddr, anyContextID, anyMultihash, anyMetadata).Return(
			errors.New("something went wrong while caching claim in providerIndex.Cache"),
		)

		// Attempt to cache the claim
		err := Cache(context.Background(), mockBlobIndexLookup, mockClaimsService, mockProviderIndex, *providerAddr, locationDelegation)
		require.Error(t, err)
		require.Contains(t, err.Error(), "something went wrong while caching claim in providerIndex.Cache")
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
