package blobindexlookup_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/capabilities/space/content"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/server/retrieval"
	ucan_http "github.com/storacha/go-ucanto/transport/http"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestBlobIndexLookup__Find(t *testing.T) {
	cid := testutil.RandomCID(t).(cidlink.Link).Cid
	provider := testutil.RandomProviderResult(t)
	_, index := testutil.RandomShardedDagIndexView(t, 32)
	indexBytes := testutil.Must(io.ReadAll(testutil.Must(index.Archive())(t)))(t)
	indexEncodedLength := uint64(len(indexBytes))

	// sample error
	testCases := []struct {
		name          string
		handler       http.HandlerFunc
		rngHeader     *metadata.Range
		auth          *types.RetrievalAuth
		expectedErr   error
		expectedIndex blobindex.ShardedDagIndexView
	}{
		{
			name: "success fetch",
			handler: func(w http.ResponseWriter, r *http.Request) {
				testutil.Must(w.Write(indexBytes))(t)
			},
			expectedIndex: index,
		},
		{
			name:        "failure",
			handler:     http.NotFound,
			expectedErr: errors.New("failure response fetching index. status: 404 Not Found, message: 404 page not found\n, url:"),
		},
		{
			name: "partial fetch from offset",
			handler: func(w http.ResponseWriter, r *http.Request) {
				randomBytes := testutil.RandomBytes(t, 10)
				allBytes := append(randomBytes, indexBytes...)
				http.ServeContent(w, r, "index", time.Now(), bytes.NewReader(allBytes))
			},
			rngHeader:     &metadata.Range{Offset: 10},
			expectedIndex: index,
		},
		{
			name: "partial fetch from offset + length",
			handler: func(w http.ResponseWriter, r *http.Request) {

				randomStartBytes := testutil.RandomBytes(t, 10)
				randomEndBytes := testutil.RandomBytes(t, 20)

				allBytes := append(append(randomStartBytes, indexBytes...), randomEndBytes...)

				http.ServeContent(w, r, "index", time.Now(), bytes.NewReader(allBytes))
			},
			rngHeader:     &metadata.Range{Offset: 10, Length: &indexEncodedLength},
			expectedIndex: index,
		},
		{
			name: "authorized retrieval",
			handler: func(w http.ResponseWriter, r *http.Request) {
				srv, err := retrieval.NewServer(
					testutil.Service,
					retrieval.WithServiceMethod(
						content.RetrieveAbility,
						retrieval.Provide(
							content.Retrieve,
							func(ctx context.Context, capability ucan.Capability[content.RetrieveCaveats], invocation invocation.Invocation, context server.InvocationContext, request retrieval.Request) (result.Result[content.RetrieveOk, failure.IPLDBuilderFailure], fx.Effects, retrieval.Response, error) {
								res := result.Ok[content.RetrieveOk, failure.IPLDBuilderFailure](content.RetrieveOk{})
								resp := retrieval.NewResponse(http.StatusOK, nil, io.NopCloser(bytes.NewReader(indexBytes)))
								return res, nil, resp, nil
							},
						),
					),
				)
				require.NoError(t, err)
				res, err := srv.Request(r.Context(), ucan_http.NewRequest(r.Body, r.Header))
				require.NoError(t, err)
				for key, values := range res.Headers() {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				if res.Status() > 0 {
					w.WriteHeader(res.Status())
				}
				_, err = io.Copy(w, res.Body())
				require.NoError(t, err)
			},
			rngHeader: &metadata.Range{Offset: 10, Length: &indexEncodedLength},
			auth: &types.RetrievalAuth{
				Issuer:   testutil.Service,
				Audience: testutil.Service,
				Capability: ucan.NewCapability[ucan.CaveatBuilder](
					content.RetrieveAbility,
					testutil.Service.DID().String(),
					content.RetrieveCaveats{
						Blob: content.BlobDigest{
							Digest: cid.Hash(),
						},
						Range: content.Range{
							Start: 10,
							End:   indexEncodedLength - 1,
						},
					},
				),
			},
			expectedIndex: index,
		},
	}
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testServer := httptest.NewServer(tc.handler)
			defer func() { testServer.Close() }()
			// Create BlobIndexLookup instance
			cl := blobindexlookup.NewBlobIndexLookup(testServer.Client())
			req := types.NewRetrievalRequest(testutil.Must(url.Parse(testServer.URL))(t), tc.rngHeader, tc.auth)
			index, err := cl.Find(context.Background(), cid.Bytes(), provider, req)
			if tc.expectedErr != nil {
				require.ErrorContains(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualIndex(t, tc.expectedIndex, index)
		})
	}
}
