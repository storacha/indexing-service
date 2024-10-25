package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	gohttp "net/http"
	"net/url"

	"github.com/multiformats/go-multibase"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-capabilities/pkg/claim"
	"github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/go-ucanto/core/result/failure"
	fdm "github.com/storacha/go-ucanto/core/result/failure/datamodel"
	unit "github.com/storacha/go-ucanto/core/result/ok"
	udm "github.com/storacha/go-ucanto/core/result/ok/datamodel"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/transport/http"
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
	serviceURL       string
}

func (c *Client) connect() (client.Connection, error) {
	url, err := url.Parse(c.serviceURL)
	if err != nil {
		return nil, fmt.Errorf("parsing service URL: %w", err)
	}
	return client.NewConnection(c.servicePrincipal, http.NewHTTPChannel(url.JoinPath(claimsPath)))
}

func (c *Client) execute(inv invocation.Invocation) error {
	connection, err := c.connect()
	if err != nil {
		return fmt.Errorf("establishing client connection: %w", err)
	}

	resp, err := client.Execute([]invocation.Invocation{inv}, connection)
	if err != nil {
		return fmt.Errorf("sending invocation: %w", err)
	}
	rcptlnk, ok := resp.Get(inv.Link())
	if !ok {
		return ErrNoReceiptFound
	}

	reader, err := receipt.NewReceiptReaderFromTypes[unit.Unit, fdm.FailureModel](udm.UnitType(), fdm.FailureType())
	if err != nil {
		return fmt.Errorf("generating receipt reader: %w")
	}

	rcpt, err := reader.Read(rcptlnk, resp.Blocks())
	if err != nil {
		return fmt.Errorf("reading receipt: %w")
	}

	_, err = result.Unwrap(result.MapError(rcpt.Out(), failure.FromFailureModel))
	return err
}

func (c *Client) PublishIndexClaim(ctx context.Context, issuer principal.Signer, caveats assert.IndexCaveats) error {
	inv, err := assert.Index.Invoke(issuer, c.servicePrincipal, c.servicePrincipal.DID().String(), caveats)
	if err != nil {
		return fmt.Errorf("generating invocation")
	}
	return c.execute(inv)
}

func (c *Client) PublishEqualsClaim(ctx context.Context, issuer principal.Signer, caveats assert.EqualsCaveats) error {
	inv, err := assert.Equals.Invoke(issuer, c.servicePrincipal, c.servicePrincipal.DID().String(), caveats)
	if err != nil {
		return fmt.Errorf("generating invocation")
	}
	return c.execute(inv)
}

func (c *Client) CacheClaim(ctx context.Context, issuer principal.Signer, provider claim.Provider, caveats assert.LocationCaveats) error {
	lc, err := assert.Location.Invoke(issuer, issuer.DID(), caveats.Space.String(), caveats)
	if err != nil {
		return fmt.Errorf("building location commitment: %w", err)
	}
	inv, err := claim.Cache.Invoke(issuer, c.servicePrincipal, c.servicePrincipal.DID().String(), claim.CacheCaveats{
		Claim:    lc.Link(),
		Provider: provider,
	})
	if err != nil {
		return fmt.Errorf("generating invocation: %w", err)
	}
	for blk, err := range lc.Blocks() {
		if err != nil {
			return fmt.Errorf("reading blocks from location commitment: %w", err)
		}
		if err := inv.Attach(blk); err != nil {
			return fmt.Errorf("attaching location commitment block: %w", err)
		}
	}

	return c.execute(inv)
}

func (c *Client) QueryClaims(ctx context.Context, query types.Query) (types.QueryResult, error) {
	url, err := url.Parse(c.serviceURL)
	if err != nil {
		return nil, fmt.Errorf("parsing service URL: %w", err)
	}
	url = url.JoinPath(claimsPath)
	q := url.Query()
	for _, mh := range query.Hashes {
		mhString, err := multibase.Encode(multibase.Base64pad, mh)
		if err != nil {
			return nil, fmt.Errorf("encoding query multihash")
		}
		q.Add("multihash", mhString)
	}
	for _, space := range query.Match.Subject {
		q.Add("spaces", space.String())
	}
	url.RawQuery = q.Encode()
	res, err := gohttp.DefaultClient.Get(url.String())
	if err != nil {
		return nil, fmt.Errorf("sending query to server: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errFromResponse(res)
	}
	return queryresult.Extract(res.Body)
}
