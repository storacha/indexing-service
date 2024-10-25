package contentclaims

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-capabilities/pkg/claim"
	"github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	unit "github.com/storacha/go-ucanto/core/result/ok"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

var rcptsch = []byte(`
	type Result union {
		| Unit "ok"
		| Any "error"
	} representation keyed

	type Unit struct {}
`)

func TestServer(t *testing.T) {
	server, err := NewServer(testutil.Service, &mockIndexer{})
	require.NoError(t, err)

	conn, err := client.NewConnection(testutil.Service, server)
	require.NoError(t, err)

	locationCommitment := testutil.Must(assert.Location.Delegate(testutil.Alice,
		testutil.Alice,
		testutil.Alice.DID().String(),
		assert.LocationCaveats{
			Content:  assert.FromHash(testutil.RandomMultihash()),
			Location: []url.URL{*testutil.Must(url.Parse("https://www.yahoo.com"))(t)},
			Space:    testutil.Bob.DID(),
		}))(t)

	cacheInvocation := testutil.Must(claim.Cache.Invoke(testutil.Service,
		testutil.Service,
		testutil.Service.DID().String(), claim.CacheCaveats{
			Claim: locationCommitment.Link(),
			Provider: claim.Provider{
				Addresses: []multiaddr.Multiaddr{testutil.RandomMultiaddr()},
			},
		}))(t)
	for b, err := range locationCommitment.Blocks() {
		if err != nil {
			t.Log(fmt.Sprintf("iterating claim blocks: %s", err))
			t.FailNow()
		}
		require.NoError(t, cacheInvocation.Attach(b))
	}
	invs := []invocation.Invocation{
		testutil.Must(assert.Equals.Invoke(
			testutil.Service,
			testutil.Service,
			testutil.Service.DID().String(),
			assert.EqualsCaveats{
				Content: assert.FromHash(testutil.RandomMultihash()),
				Equals:  testutil.RandomCID(),
			},
		))(t),
		testutil.Must(assert.Index.Invoke(
			testutil.Service,
			testutil.Service,
			testutil.Service.DID().String(),
			assert.IndexCaveats{
				Content: testutil.RandomCID(),
				Index:   testutil.RandomCID(),
			},
		))(t),
		cacheInvocation,
	}

	for _, inv := range invs {
		t.Run(inv.Capabilities()[0].Can(), func(t *testing.T) {
			resp, err := client.Execute([]invocation.Invocation{inv}, conn)
			require.NoError(t, err)

			rcptlnk, ok := resp.Get(inv.Link())
			require.True(t, ok, "missing receipt for invocation: %s", inv.Link())

			reader, err := receipt.NewReceiptReader[unit.Unit, datamodel.Node](rcptsch)
			require.NoError(t, err)

			rcpt, err := reader.Read(rcptlnk, resp.Blocks())
			require.NoError(t, err)

			result.MatchResultR0(rcpt.Out(), func(ok unit.Unit) {
				fmt.Printf("%+v\n", ok)
			}, func(x datamodel.Node) {
				require.Fail(t, "unexpected failure")
			})
		})
	}
}

type mockIndexer struct {
}

// CacheClaim implements types.Service.
func (m *mockIndexer) CacheClaim(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error {
	return nil
}

// PublishClaim implements types.Service.
func (m *mockIndexer) PublishClaim(ctx context.Context, claim delegation.Delegation) error {
	return nil
}

// Query implements types.Service.
func (m *mockIndexer) Query(ctx context.Context, q types.Query) (types.QueryResult, error) {
	return nil, nil
}

var _ types.Service = (*mockIndexer)(nil)
