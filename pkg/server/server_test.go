package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetRootHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svr := httptest.NewServer(GetRootHandler(testutil.Service))
		defer svr.Close()

		res, err := http.Get(svr.URL)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)

		bytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		strings.Contains(string(bytes), testutil.Service.DID().String())
	})

	t.Run("did:web", func(t *testing.T) {
		didweb, err := did.Parse("did:web:example.org")
		require.NoError(t, err)

		signer, err := signer.Wrap(testutil.Service, didweb)
		require.NoError(t, err)

		svr := httptest.NewServer(GetRootHandler(signer))
		defer svr.Close()

		res, err := http.Get(svr.URL)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)

		bytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		strings.Contains(string(bytes), testutil.Service.DID().String())
		strings.Contains(string(bytes), didweb.String())
	})
}

func TestGetClaimHandler(t *testing.T) {
	store := contentclaims.NewStoreFromDatastore(datastore.NewMapDatastore())
	claim := testutil.RandomIndexDelegation()
	err := store.Put(context.Background(), claim.Link(), claim)
	require.NoError(t, err)

	svr := httptest.NewServer(GetClaimHandler(store))
	defer svr.Close()

	t.Run("success", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s/claim/%s", svr.URL, claim.Link()))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)

		bytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		d, err := delegation.Extract(bytes)
		require.NoError(t, err)

		require.Equal(t, claim.Link(), d.Link())
	})

	t.Run("not found", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s/claim/%s", svr.URL, testutil.RandomCID()))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("invalid CID", func(t *testing.T) {
		res, err := http.Get(fmt.Sprintf("%s/claim/invalid", svr.URL))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})
}

func TestGetClaimsHandler(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		mockService := types.NewMockService(t)

		randomHash := testutil.RandomMultihash()
		query := types.Query{
			Type:   types.QueryTypeStandard,
			Hashes: []multihash.Multihash{randomHash},
			Match: types.Match{
				Subject: []did.DID{},
			},
		}

		locationClaim := testutil.RandomLocationDelegation()
		indexClaim := testutil.RandomIndexDelegation()
		equalsClaim := testutil.RandomEqualsDelegation()
		claims := map[cid.Cid]delegation.Delegation{
			link.ToCID(locationClaim.Link()): locationClaim,
			link.ToCID(indexClaim.Link()):    indexClaim,
			link.ToCID(equalsClaim.Link()):   equalsClaim,
		}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](1)
		indexHash, index := testutil.RandomShardedDagIndexView(32)
		indexContextID := testutil.Must(types.ContextID{
			Hash: indexHash,
		}.ToEncoded())(t)
		indexes.Set(indexContextID, index)
		queryResult := testutil.Must(queryresult.Build(claims, indexes))(t)
		mockService.EXPECT().Query(mock.Anything, query).Return(queryResult, nil)

		svr := httptest.NewServer(GetClaimsHandler(mockService))
		defer svr.Close()

		res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s", svr.URL, digestutil.Format(randomHash)))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)

		result, err := queryresult.Extract(res.Body)
		require.NoError(t, err)

		require.ElementsMatch(t, queryResult.Claims(), result.Claims())
		require.ElementsMatch(t, queryResult.Indexes(), result.Indexes())
	})

	t.Run("empty results are ok", func(t *testing.T) {
		mockService := types.NewMockService(t)

		randomHash := testutil.RandomMultihash()
		query := types.Query{
			Type:   types.QueryTypeStandard,
			Hashes: []multihash.Multihash{randomHash},
			Match: types.Match{
				Subject: []did.DID{},
			},
		}

		claims := map[cid.Cid]delegation.Delegation{}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1)
		queryResult := testutil.Must(queryresult.Build(claims, indexes))(t)
		mockService.EXPECT().Query(mock.Anything, query).Return(queryResult, nil)

		svr := httptest.NewServer(GetClaimsHandler(mockService))
		defer svr.Close()

		res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s", svr.URL, digestutil.Format(randomHash)))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)

		result, err := queryresult.Extract(res.Body)
		require.NoError(t, err)

		require.Empty(t, result.Claims())
		require.Empty(t, result.Indexes())
	})

	t.Run("invalid hash", func(t *testing.T) {
		mockService := types.NewMockService(t)

		svr := httptest.NewServer(GetClaimsHandler(mockService))
		defer svr.Close()

		res, err := http.Get(fmt.Sprintf("%s/claims?multihash=invalid", svr.URL))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("honors spaces parameter", func(t *testing.T) {
		mockService := types.NewMockService(t)

		randomHash := testutil.RandomMultihash()
		randomSubject := testutil.RandomPrincipal().DID()
		query := types.Query{
			Type:   types.QueryTypeStandard,
			Hashes: []multihash.Multihash{randomHash},
			Match: types.Match{
				Subject: []did.DID{randomSubject},
			},
		}

		claims := map[cid.Cid]delegation.Delegation{}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1)
		queryResult := testutil.Must(queryresult.Build(claims, indexes))(t)
		mockService.EXPECT().Query(mock.Anything, query).Return(queryResult, nil)

		svr := httptest.NewServer(GetClaimsHandler(mockService))
		defer svr.Close()

		res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s&spaces=%s", svr.URL, digestutil.Format(randomHash), randomSubject.String()))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("invalid space", func(t *testing.T) {
		mockService := types.NewMockService(t)

		svr := httptest.NewServer(GetClaimsHandler(mockService))
		defer svr.Close()

		res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s&spaces=invalid", svr.URL, digestutil.Format(testutil.RandomMultihash())))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("validates query type properly", func(t *testing.T) {
		mockService := types.NewMockService(t)

		svr := httptest.NewServer(GetClaimsHandler(mockService))
		defer svr.Close()

		randomHash := testutil.RandomMultihash()

		claims := map[cid.Cid]delegation.Delegation{}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1)
		queryResult := testutil.Must(queryresult.Build(claims, indexes))(t)

		t.Run("standard", func(t *testing.T) {
			query := types.Query{
				Type:   types.QueryTypeStandard,
				Hashes: []multihash.Multihash{randomHash},
				Match: types.Match{
					Subject: []did.DID{},
				},
			}

			mockService.EXPECT().Query(mock.Anything, query).Return(queryResult, nil)

			res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s&type=standard", svr.URL, digestutil.Format(randomHash)))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
		})

		t.Run("location", func(t *testing.T) {
			query := types.Query{
				Type:   types.QueryTypeLocation,
				Hashes: []multihash.Multihash{randomHash},
				Match: types.Match{
					Subject: []did.DID{},
				},
			}

			mockService.EXPECT().Query(mock.Anything, query).Return(queryResult, nil)

			res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s&type=location", svr.URL, digestutil.Format(randomHash)))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
		})

		t.Run("index_or_location", func(t *testing.T) {
			query := types.Query{
				Type:   types.QueryTypeIndexOrLocation,
				Hashes: []multihash.Multihash{randomHash},
				Match: types.Match{
					Subject: []did.DID{},
				},
			}

			mockService.EXPECT().Query(mock.Anything, query).Return(queryResult, nil)

			res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s&type=index_or_location", svr.URL, digestutil.Format(randomHash)))
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, res.StatusCode)
		})

		t.Run("invalid", func(t *testing.T) {
			res, err := http.Get(fmt.Sprintf("%s/claims?multihash=%s&type=invalid", svr.URL, digestutil.Format(randomHash)))
			require.NoError(t, err)
			require.Equal(t, http.StatusBadRequest, res.StatusCode)
		})
	})
}

func TestGetDIDDocumentHandler(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svr := httptest.NewServer(GetDIDDocument(testutil.Service))
		defer svr.Close()

		res, err := http.Get(svr.URL)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		bytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		var doc Document
		err = json.Unmarshal(bytes, &doc)
		require.NoError(t, err)

		require.Equal(t, doc.ID, testutil.Service.DID().String())
	})

	t.Run("did:web", func(t *testing.T) {
		didweb, err := did.Parse("did:web:example.org")
		require.NoError(t, err)

		signer, err := signer.Wrap(testutil.Service, didweb)
		require.NoError(t, err)

		svr := httptest.NewServer(GetDIDDocument(signer))
		defer svr.Close()

		res, err := http.Get(svr.URL)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusOK, res.StatusCode)

		bytes, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		var doc Document
		err = json.Unmarshal(bytes, &doc)
		require.NoError(t, err)

		require.Equal(t, doc.ID, didweb.DID().String())
		require.True(t, strings.HasSuffix(testutil.Service.DID().String(), doc.VerificationMethod[0].PublicKeyMultibase))
	})
}
