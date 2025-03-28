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
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/stretchr/testify/require"
)

func TestBlobIndexLookup__Find(t *testing.T) {
	cid := testutil.RandomCID().(cidlink.Link).Cid
	provider := testutil.RandomProviderResult()
	_, index := testutil.RandomShardedDagIndexView(32)
	indexBytes := testutil.Must(io.ReadAll(testutil.Must(index.Archive())(t)))(t)
	indexEncodedLength := uint64(len(indexBytes))
	// sample error
	testCases := []struct {
		name          string
		handler       http.HandlerFunc
		rngHeader     *metadata.Range
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
				randomBytes := testutil.RandomBytes(10)
				allBytes := append(randomBytes, indexBytes...)
				http.ServeContent(w, r, "index", time.Now(), bytes.NewReader(allBytes))
			},
			rngHeader:     &metadata.Range{Offset: 10},
			expectedIndex: index,
		},
		{
			name: "partial fetch from offset + length",
			handler: func(w http.ResponseWriter, r *http.Request) {

				randomStartBytes := testutil.RandomBytes(10)
				randomEndBytes := testutil.RandomBytes(20)

				allBytes := append(append(randomStartBytes, indexBytes...), randomEndBytes...)

				http.ServeContent(w, r, "index", time.Now(), bytes.NewReader(allBytes))
			},
			rngHeader:     &metadata.Range{Offset: 10, Length: &indexEncodedLength},
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
			index, err := cl.Find(context.Background(), cid.Bytes(), provider, testutil.Must(url.Parse(testServer.URL))(t), tc.rngHeader)
			if tc.expectedErr != nil {
				require.ErrorContains(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualIndex(t, tc.expectedIndex, index)
		})
	}
}
