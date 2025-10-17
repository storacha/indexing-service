package blobindexlookup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/go-libstoracha/blobindex"
	rclient "github.com/storacha/go-ucanto/client/retrieval"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	fdm "github.com/storacha/go-ucanto/core/result/failure/datamodel"
	"github.com/storacha/indexing-service/pkg/types"
)

type simpleLookup struct {
	httpClient *http.Client
}

var _ BlobIndexLookup = (*simpleLookup)(nil)

func NewBlobIndexLookup(httpClient *http.Client) BlobIndexLookup {
	return &simpleLookup{httpClient}
}

// Find fetches the blob index from the given fetchURL
func (s *simpleLookup) Find(ctx context.Context, _ types.EncodedContextID, result model.ProviderResult, request types.RetrievalRequest) (blobindex.ShardedDagIndexView, error) {
	// If retrieval authroization details were provided, make a UCAN authorized
	// retrieval request.
	if request.Auth != nil {
		body, err := doAuthorizedRetrieval(ctx, s.httpClient, request)
		if err != nil {
			return nil, fmt.Errorf("executing authorized retrieval: %w", err)
		}
		defer body.Close()
		return blobindex.Extract(body)
	}

	// attempt to fetch the index from provided url
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("constructing request: %w", err)
	}
	rng := request.Range
	if rng != nil {
		rangeHeader := fmt.Sprintf("bytes=%d-", rng.Offset)
		if rng.Length != nil {
			rangeHeader += strconv.FormatUint(rng.Offset+*rng.Length-1, 10)
		}
		httpReq.Header.Set("Range", rangeHeader)
	}
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failure response fetching index. status: %s, message: %s, url: %s", resp.Status, string(body), request.URL.String())
	}
	defer resp.Body.Close()
	return blobindex.Extract(resp.Body)
}

func doAuthorizedRetrieval(ctx context.Context, httpClient *http.Client, request types.RetrievalRequest) (io.ReadCloser, error) {
	headers := http.Header{}
	if request.Range != nil {
		if request.Range.Length != nil {
			headers.Set("Range", fmt.Sprintf("bytes=%d-%d", request.Range.Offset, request.Range.Offset+*request.Range.Length-1))
		} else {
			headers.Set("Range", fmt.Sprintf("bytes=%d-", request.Range.Offset))
		}
	}

	conn, err := rclient.NewConnection(
		request.Auth.Audience,
		request.URL,
		rclient.WithClient(httpClient),
		rclient.WithHeaders(headers),
	)
	if err != nil {
		return nil, err
	}

	iss, aud, cap := request.Auth.Issuer, request.Auth.Audience, request.Auth.Capability
	inv, err := invocation.Invoke(iss, aud, cap, delegation.WithProof(request.Auth.Proofs...))
	if err != nil {
		return nil, err
	}

	xres, hres, err := rclient.Execute(ctx, inv, conn)
	if err != nil {
		return nil, fmt.Errorf("executing retrieval invocation: %w", err)
	}

	rcptLink, ok := xres.Get(inv.Link())
	if !ok {
		return nil, errors.New("execution response did not contain receipt for invocation")
	}

	bs, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(xres.Blocks()))
	if err != nil {
		return nil, fmt.Errorf("adding blocks to reader: %w", err)
	}

	rcpt, err := receipt.NewAnyReceipt(rcptLink, bs)
	if err != nil {
		return nil, fmt.Errorf("creating receipt: %w", err)
	}

	_, x := result.Unwrap(rcpt.Out())
	if x != nil {
		fail := fdm.Bind(x)
		name := "Unnamed"
		if fail.Name != nil {
			name = *fail.Name
		}
		return nil, fmt.Errorf("execution resulted in failure: %s: %s", name, fail.Message)
	}

	return hres.Body(), nil
}
