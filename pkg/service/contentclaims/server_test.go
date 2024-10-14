package contentclaims

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/core/result"
	"github.com/storacha/indexing-service/pkg/capability/assert"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
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
	server, err := NewServer(testutil.Service, nil)
	require.NoError(t, err)

	conn, err := client.NewConnection(testutil.Service, server)
	require.NoError(t, err)

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
		testutil.Must(assert.Location.Invoke(
			testutil.Service,
			testutil.Service,
			testutil.Service.DID().String(),
			assert.LocationCaveats{
				Content:  assert.FromHash(testutil.RandomMultihash()),
				Location: []url.URL{},
			},
		))(t),
	}

	for _, inv := range invs {
		t.Run(inv.Capabilities()[0].Can(), func(t *testing.T) {
			resp, err := client.Execute([]invocation.Invocation{inv}, conn)
			require.NoError(t, err)

			rcptlnk, ok := resp.Get(inv.Link())
			require.True(t, ok, "missing receipt for invocation: %s", inv.Link())

			reader, err := receipt.NewReceiptReader[assert.Unit, datamodel.Node](rcptsch)
			require.NoError(t, err)

			rcpt, err := reader.Read(rcptlnk, resp.Blocks())
			require.NoError(t, err)

			result.MatchResultR0(rcpt.Out(), func(ok assert.Unit) {
				fmt.Printf("%+v\n", ok)
			}, func(x datamodel.Node) {
				require.Fail(t, "unexpected failure")
			})
		})
	}
}
