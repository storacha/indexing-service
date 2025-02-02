package contentclaims

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/printer"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	cassert "github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-capabilities/pkg/claim"
	ctypes "github.com/storacha/go-capabilities/pkg/types"
	"github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	unit "github.com/storacha/go-ucanto/core/result/ok"
	"github.com/storacha/go-ucanto/did"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/principalresolver"
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
	server, err := NewUCANServer(testutil.Service, &mockIndexer{})
	require.NoError(t, err)

	conn, err := client.NewConnection(testutil.Service, server)
	require.NoError(t, err)

	locationCommitment := testutil.Must(cassert.Location.Delegate(testutil.Alice,
		testutil.Alice,
		testutil.Alice.DID().String(),
		cassert.LocationCaveats{
			Content:  ctypes.FromHash(testutil.RandomMultihash()),
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
			t.Logf("iterating claim blocks: %s", err)
			t.FailNow()
		}
		require.NoError(t, cacheInvocation.Attach(b))
	}
	invs := []invocation.Invocation{
		testutil.Must(cassert.Equals.Invoke(
			testutil.Service,
			testutil.Service,
			testutil.Service.DID().String(),
			cassert.EqualsCaveats{
				Content: ctypes.FromHash(testutil.RandomMultihash()),
				Equals:  testutil.RandomCID(),
			},
		))(t),
		testutil.Must(cassert.Index.Invoke(
			testutil.Service,
			testutil.Service,
			testutil.Service.DID().String(),
			cassert.IndexCaveats{
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

func TestPrincipalResolver(t *testing.T) {
	// simulate the upload service (a did:web) issuing an invocation to the
	// indexing service
	uploadID, err := ed25519.Generate()
	require.NoError(t, err)

	uploadIDWeb, err := signer.Wrap(uploadID, testutil.Must(did.Parse("did:web:upload.storacha.network"))(t))
	require.NoError(t, err)

	presolv, err := principalresolver.New(map[string]string{
		uploadIDWeb.DID().String(): uploadIDWeb.Unwrap().DID().String(),
	})
	require.NoError(t, err)

	server, err := NewUCANServer(testutil.Service, &mockIndexer{}, server.WithPrincipalResolver(presolv.ResolveDIDKey))
	require.NoError(t, err)

	proof := delegation.FromDelegation(
		testutil.Must(
			delegation.Delegate(
				testutil.Service,
				uploadIDWeb,
				[]ucan.Capability[ucan.NoCaveats]{
					ucan.NewCapability(cassert.EqualsAbility, testutil.Service.DID().String(), ucan.NoCaveats{}),
				},
			),
		)(t),
	)

	inv := testutil.Must(cassert.Equals.Invoke(
		uploadIDWeb,
		testutil.Service,
		testutil.Service.DID().String(),
		cassert.EqualsCaveats{
			Content: ctypes.FromHash(testutil.RandomMultihash()),
			Equals:  testutil.RandomCID(),
		},
		delegation.WithProof(proof),
	))(t)

	conn, err := client.NewConnection(testutil.Service, server)
	require.NoError(t, err)

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
		fmt.Println(printer.Sprint(x))
		require.Fail(t, "unexpected failure")
	})
}

type mockIndexer struct {
}

func (m *mockIndexer) Get(ctx context.Context, claim ipld.Link) (delegation.Delegation, error) {
	return nil, nil
}

// Cache implements types.Service.
func (m *mockIndexer) Cache(ctx context.Context, provider peer.AddrInfo, claim delegation.Delegation) error {
	return nil
}

// Publish implements types.Service.
func (m *mockIndexer) Publish(ctx context.Context, claim delegation.Delegation) error {
	return nil
}

// Query implements types.Service.
func (m *mockIndexer) Query(ctx context.Context, q types.Query) (types.QueryResult, error) {
	return nil, nil
}

var _ types.Service = (*mockIndexer)(nil)
