package client

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/storacha/go-libstoracha/capabilities/assert"
	"github.com/storacha/go-libstoracha/capabilities/claim"
	"github.com/storacha/go-libstoracha/digestutil"
	"github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/message"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	fdm "github.com/storacha/go-ucanto/core/result/failure/datamodel"
	unit "github.com/storacha/go-ucanto/core/result/ok"
	udm "github.com/storacha/go-ucanto/core/result/ok/datamodel"
	"github.com/storacha/go-ucanto/principal"
	hcmsg "github.com/storacha/go-ucanto/transport/headercar/message"
	ucan_http "github.com/storacha/go-ucanto/transport/http"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
)

const claimsPath = "/claims"

var ErrNoReceiptFound = errors.New("missing receipt link")

type ErrFailedResponse struct {
	StatusCode int
	Body       string
}

func errFromResponse(res *http.Response) ErrFailedResponse {
	err := ErrFailedResponse{StatusCode: res.StatusCode}

	message, merr := io.ReadAll(res.Body)
	if merr != nil {
		err.Body = merr.Error()
	} else {
		err.Body = string(message)
	}
	return err
}

func (e ErrFailedResponse) Error() string {
	return fmt.Sprintf("http request failed, status: %d %s, message: %s", e.StatusCode, http.StatusText(e.StatusCode), e.Body)
}

type Client struct {
	servicePrincipal ucan.Principal
	serviceURL       url.URL
	connection       client.Connection
	httpClient       *http.Client
}

func (c *Client) execute(ctx context.Context, inv invocation.Invocation) error {
	resp, err := client.Execute(ctx, []invocation.Invocation{inv}, c.connection)
	if err != nil {
		return fmt.Errorf("sending invocation: %w", err)
	}
	rcptlnk, ok := resp.Get(inv.Link())
	if !ok {
		return ErrNoReceiptFound
	}

	reader, err := receipt.NewReceiptReaderFromTypes[unit.Unit, fdm.FailureModel](udm.UnitType(), fdm.FailureType())
	if err != nil {
		return fmt.Errorf("generating receipt reader: %w", err)
	}

	rcpt, err := reader.Read(rcptlnk, resp.Blocks())
	if err != nil {
		return fmt.Errorf("reading receipt: %w", err)
	}

	_, err = result.Unwrap(result.MapError(rcpt.Out(), failure.FromFailureModel))
	return err
}

func (c *Client) PublishIndexClaim(ctx context.Context, issuer principal.Signer, caveats assert.IndexCaveats, options ...delegation.Option) error {
	inv, err := assert.Index.Invoke(issuer, c.servicePrincipal, c.servicePrincipal.DID().String(), caveats, options...)
	if err != nil {
		return fmt.Errorf("generating invocation: %w", err)
	}
	return c.execute(ctx, inv)
}

func (c *Client) PublishEqualsClaim(ctx context.Context, issuer principal.Signer, caveats assert.EqualsCaveats, options ...delegation.Option) error {
	inv, err := assert.Equals.Invoke(issuer, c.servicePrincipal, c.servicePrincipal.DID().String(), caveats, options...)
	if err != nil {
		return fmt.Errorf("generating invocation: %w", err)
	}
	return c.execute(ctx, inv)
}

func (c *Client) CacheClaim(ctx context.Context, issuer principal.Signer, cacheClaim delegation.Delegation, provider claim.Provider, options ...delegation.Option) error {
	inv, err := claim.Cache.Invoke(issuer, c.servicePrincipal, c.servicePrincipal.DID().String(), claim.CacheCaveats{
		Claim:    cacheClaim.Link(),
		Provider: provider,
	}, options...)
	if err != nil {
		return fmt.Errorf("generating invocation: %w", err)
	}

	for blk, err := range cacheClaim.Blocks() {
		if err != nil {
			return fmt.Errorf("reading claim blocks: %w", err)
		}
		if err := inv.Attach(blk); err != nil {
			return fmt.Errorf("attaching claim block: %w", err)
		}
	}

	return c.execute(ctx, inv)
}

func (c *Client) QueryClaims(ctx context.Context, query types.Query) (types.QueryResult, error) {
	url := c.serviceURL.JoinPath(claimsPath)
	q := url.Query()
	q.Add("type", query.Type.String())
	for _, mh := range query.Hashes {
		q.Add("multihash", digestutil.Format(mh))
	}
	for _, space := range query.Match.Subject {
		q.Add("spaces", space.String())
	}
	url.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	// Request gzip compression
	req.Header.Set("Accept-Encoding", "gzip")
	// If there are query delegations, then add them to an X-Agent-Message header.
	if len(query.Delegations) > 0 {
		invs := make([]invocation.Invocation, 0, len(query.Delegations))
		for _, d := range query.Delegations {
			invs = append(invs, d)
		}
		msg, err := message.Build(invs, nil)
		if err != nil {
			return nil, fmt.Errorf("building agent message: %w", err)
		}
		headerValue, err := hcmsg.EncodeHeader(msg)
		if err != nil {
			return nil, fmt.Errorf("encoding %s header: %w", hcmsg.HeaderName, err)
		}
		req.Header.Set(hcmsg.HeaderName, headerValue)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending query to server: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errFromResponse(res)
	}

	// Handle gzip decompression if needed
	var reader io.ReadCloser = res.Body
	if res.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, fmt.Errorf("creating gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	return queryresult.Extract(reader)
}

type Option func(*Client)

// WithHTTPClient configures the HTTP client to use for making query requests
// and invocations.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func New(servicePrincipal ucan.Principal, serviceURL url.URL, options ...Option) (*Client, error) {
	c := Client{
		servicePrincipal: servicePrincipal,
		serviceURL:       serviceURL,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range options {
		opt(&c)
	}
	channel := ucan_http.NewChannel(serviceURL.JoinPath(claimsPath), ucan_http.WithClient(c.httpClient))
	conn, err := client.NewConnection(servicePrincipal, channel)
	if err != nil {
		return nil, fmt.Errorf("creating connection: %w", err)
	}
	c.connection = conn
	return &c, nil
}
