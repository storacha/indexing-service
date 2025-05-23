package aws_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestBucketFallbackMapper(t *testing.T) {
	ctx := context.Background()
	responses := bytemap.NewByteMap[multihash.Multihash, resp](-1)
	hasResponses := bytemap.NewByteMap[multihash.Multihash, hasResp](-1)
	signer := testutil.RandomSigner()

	// set up test server
	mux := http.NewServeMux()
	mux.Handle("/{multihash1}/{multihash2}", mockServer{responses})

	server := httptest.NewServer(mux)
	defer server.Close()
	serverURL := testutil.Must(url.Parse(server.URL))(t)

	errorOnReadAllocationsHash := testutil.RandomMultihash()
	hasResponses.Set(errorOnReadAllocationsHash, hasResp{false, errors.New("something went wrong")})

	nonHasHash := testutil.RandomMultihash()
	hasResponses.Set(nonHasHash, hasResp{false, nil})

	hasNonSuccessHash := testutil.RandomMultihash()
	responses.Set(hasNonSuccessHash, resp{0, http.StatusInternalServerError})
	hasResponses.Set(hasNonSuccessHash, hasResp{true, nil})

	hasSuccessHash := testutil.RandomMultihash()
	hasSuccessContentLength := uint64(500)
	responses.Set(hasSuccessHash, resp{int64(hasSuccessContentLength), http.StatusOK})
	hasResponses.Set(hasSuccessHash, hasResp{true, nil})
	hasSuccessClaim := testutil.Must(cassert.Location.Delegate(
		signer,
		signer,
		signer.DID().String(),
		cassert.LocationCaveats{
			Content: ctypes.FromHash(hasSuccessHash),
			Location: []url.URL{
				*serverURL.JoinPath(digestutil.Format(hasSuccessHash), fmt.Sprintf("%s.blob", digestutil.Format(hasSuccessHash))),
			},
			Range: &cassert.Range{
				Offset: 0,
				Length: &hasSuccessContentLength,
			},
		},
		delegation.WithNoExpiration(),
	))(t)

	data := testutil.Must(io.ReadAll(hasSuccessClaim.Archive()))(t)

	hasSuccessCids := []cid.Cid{testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   multihash.IDENTITY,
		MhLength: int(hasSuccessContentLength),
	}.Sum(data))(t)}

	testCases := []struct {
		name          string
		hash          multihash.Multihash
		expectedCids  []cid.Cid
		expectedErr   error
		expectedClaim delegation.Delegation
	}{
		{
			name:        "error checking allocations store",
			hash:        errorOnReadAllocationsHash,
			expectedErr: types.ErrKeyNotFound,
		},
		{
			name:        "allocations store does not have hash",
			hash:        nonHasHash,
			expectedErr: types.ErrKeyNotFound,
		},
		{
			name:        "non 200 status code from fallback bucket",
			hash:        hasNonSuccessHash,
			expectedErr: types.ErrKeyNotFound,
		},
		{
			name:          "200 status code on head generates claim",
			hash:          hasSuccessHash,
			expectedCids:  hasSuccessCids,
			expectedClaim: hasSuccessClaim,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			bucketFallbackMapper := aws.NewBucketFallbackMapper(signer, http.DefaultClient, serverURL, mockAllocationsStore{hasResponses}, func() []delegation.Option {
				return []delegation.Option{delegation.WithNoExpiration()}
			})
			cids, err := bucketFallbackMapper.GetClaims(ctx, testCase.hash)
			if testCase.expectedErr != nil {
				require.ErrorIs(t, err, testCase.expectedErr)
				require.Len(t, cids, 0)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.expectedCids, cids)
				if testCase.expectedClaim != nil {
					require.Len(t, cids, 1)
					require.Equal(t, cids[0].Prefix().MhType, uint64(multihash.IDENTITY))
					decoded := testutil.Must(multihash.Decode(cids[0].Hash()))(t)
					claim := testutil.Must(delegation.Extract(decoded.Digest))(t)
					testutil.RequireEqualDelegation(t, testCase.expectedClaim, claim)
				}
			}
		})
	}
}

type resp struct {
	contentLength int64
	status        int
}

type mockServer struct {
	hashes bytemap.ByteMap[multihash.Multihash, resp]
}

// ServeHTTP implements http.Handler.
func (m mockServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "HEAD" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mhString := r.PathValue("multihash1")
	mhString2 := r.PathValue("multihash2")
	mhTrimmed, hadSuffix := strings.CutSuffix(mhString2, ".blob")
	if mhString != mhTrimmed || !hadSuffix || mhString == "" {
		http.Error(w, "invalid multihash", http.StatusBadRequest)
		return
	}
	mh, err := digestutil.Parse(mhString)
	if err != nil {
		http.Error(w, fmt.Sprintf("parsing multihash: %s", err.Error()), http.StatusBadRequest)
	}
	if !m.hashes.Has(mh) {
		http.Error(w, "not found", http.StatusNotFound)
	}
	resp := m.hashes.Get(mh)
	w.Header().Add("Content-Length", strconv.FormatInt(resp.contentLength, 10))
	w.WriteHeader(resp.status)
}

type hasResp struct {
	has bool
	err error
}

type mockAllocationsStore struct {
	hashes bytemap.ByteMap[multihash.Multihash, hasResp]
}

func (m mockAllocationsStore) Has(ctx context.Context, mh multihash.Multihash) (bool, error) {
	if !m.hashes.Has(mh) {
		return false, types.ErrKeyNotFound
	}
	hasResp := m.hashes.Get(mh)
	return hasResp.has, hasResp.err
}
