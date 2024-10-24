package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
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
