package client

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/maurl"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	"github.com/storacha/go-capabilities/pkg/claim"
	"github.com/storacha/go-ucanto/client"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt/fx"
	"github.com/storacha/go-ucanto/core/result/ok"
	"github.com/storacha/go-ucanto/principal"
	ucanserver "github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	indexingID := testutil.Service
	indexingURL := randomLocalURL(t)

	storageID := testutil.RandomSigner()
	storageProof := delegation.FromDelegation(
		testutil.Must(
			delegation.Delegate(
				indexingID,
				storageID,
				[]ucan.Capability[ucan.NoCaveats]{
					ucan.NewCapability(
						claim.CacheAbility,
						indexingID.DID().String(),
						ucan.NoCaveats{},
					),
				},
			),
		)(t),
	)
	storageURL := randomLocalURL(t)

	alice := testutil.Alice
	aliceProof := delegation.FromDelegation(
		testutil.Must(
			delegation.Delegate(
				indexingID,
				alice,
				[]ucan.Capability[ucan.NoCaveats]{
					ucan.NewCapability(
						assert.EqualsAbility,
						indexingID.DID().String(),
						ucan.NoCaveats{},
					),
					ucan.NewCapability(
						assert.IndexAbility,
						indexingID.DID().String(),
						ucan.NoCaveats{},
					),
				},
			),
		)(t),
	)
	space := testutil.RandomPrincipal()

	root, digest, bytes := testutil.RandomCAR(128)
	rootDigest := link.ToCID(root).Hash()
	index, err := blobindex.FromShardArchives(root, [][]byte{bytes})
	require.NoError(t, err)

	locationClaim, err := assert.Location.Delegate(
		storageID,
		space,
		storageID.DID().String(),
		assert.LocationCaveats{
			Space:    space.DID(),
			Content:  assert.FromHash(digest),
			Location: []url.URL{*storageURL.JoinPath("/blob/%s", digestutil.Format(digest))},
		},
	)
	require.NoError(t, err)

	provider := claim.Provider{
		Addresses: []multiaddr.Multiaddr{
			testutil.Must(maurl.FromURL(storageURL.JoinPath("/claim/{claim}")))(t),
			testutil.Must(maurl.FromURL(storageURL.JoinPath("/blob/{blob}")))(t),
		},
	}

	indexBytes, err := io.ReadAll(testutil.Must(index.Archive())(t))
	require.NoError(t, err)
	indexDigest, err := multihash.Sum(indexBytes, multihash.SHA2_256, -1)
	require.NoError(t, err)
	indexLink := cidlink.Link{Cid: cid.NewCidV1(uint64(multicodec.Car), indexDigest)}

	indexLocationClaim, err := assert.Location.Delegate(
		storageID,
		space,
		storageID.DID().String(),
		assert.LocationCaveats{
			Space:    space.DID(),
			Content:  assert.FromHash(indexDigest),
			Location: []url.URL{*storageURL.JoinPath("/blob/%s", digestutil.Format(indexDigest))},
		},
	)
	require.NoError(t, err)

	t.Run("cache claim", func(t *testing.T) {
		indexingUCANInvocations := []invocation.Invocation{}
		indexingUCANServer := mockUCANService(t, indexingID, func(inv invocation.Invocation) {
			indexingUCANInvocations = append(indexingUCANInvocations, inv)
		})
		indexingQueryResults := bytemap.NewByteMap[multihash.Multihash, types.QueryResult](-1)
		indexingQueryServer := mockQueryServer(indexingQueryResults)
		t.Cleanup(indexingQueryServer.Close)

		c, err := New(indexingID, indexingURL)
		c.connection = testutil.Must(client.NewConnection(indexingID, indexingUCANServer))(t)
		require.NoError(t, err)

		err = c.CacheClaim(context.Background(), storageID, locationClaim, provider, delegation.WithProof(storageProof))
		require.NoError(t, err)

		cacheClaimInvocation := indexingUCANInvocations[len(indexingUCANInvocations)-1]
		require.NotNil(t, cacheClaimInvocation)
		require.Equal(t, claim.CacheAbility, cacheClaimInvocation.Capabilities()[0].Can())
	})

	t.Run("publish index claim", func(t *testing.T) {
		indexingUCANInvocations := []invocation.Invocation{}
		indexingUCANServer := mockUCANService(t, indexingID, func(inv invocation.Invocation) {
			indexingUCANInvocations = append(indexingUCANInvocations, inv)
		})

		c, err := New(indexingID, indexingURL)
		c.connection = testutil.Must(client.NewConnection(indexingID, indexingUCANServer))(t)
		require.NoError(t, err)

		// alice publishes index claim
		err = c.PublishIndexClaim(
			context.Background(),
			alice,
			assert.IndexCaveats{
				Content: root,
				Index:   indexLink,
			},
			delegation.WithProof(aliceProof),
		)
		require.NoError(t, err)

		assertIndexInvocation := indexingUCANInvocations[len(indexingUCANInvocations)-1]
		require.NotNil(t, assertIndexInvocation)
		require.Equal(t, assert.IndexAbility, assertIndexInvocation.Capabilities()[0].Can())
	})

	t.Run("query", func(t *testing.T) {
		indexingQueryResults := bytemap.NewByteMap[multihash.Multihash, types.QueryResult](-1)
		indexingQueryServer := mockQueryServer(indexingQueryResults)
		t.Cleanup(indexingQueryServer.Close)

		c, err := New(indexingID, *testutil.Must(url.Parse(indexingQueryServer.URL))(t))
		require.NoError(t, err)

		claims := map[cid.Cid]delegation.Delegation{
			link.ToCID(locationClaim.Link()):      locationClaim,
			link.ToCID(indexLocationClaim.Link()): indexLocationClaim,
		}
		indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1)
		spaceDID := space.DID()
		contextID := types.ContextID{Space: &spaceDID, Hash: rootDigest}
		indexes.Set(testutil.Must(contextID.ToEncoded())(t), index)

		indexingQueryResults.Set(rootDigest, testutil.Must(queryresult.Build(claims, indexes))(t))

		res, err := c.QueryClaims(context.Background(), types.Query{
			Hashes: []multihash.Multihash{rootDigest},
		})
		require.NoError(t, err)

		require.NotEmpty(t, res.Claims())
		require.True(t, slices.ContainsFunc(res.Claims(), func(c datamodel.Link) bool {
			return c.String() == locationClaim.Link().String()
		}))
		require.True(t, slices.ContainsFunc(res.Claims(), func(c datamodel.Link) bool {
			return c.String() == indexLocationClaim.Link().String()
		}))
		require.Equal(t, 1, len(res.Indexes()))
		require.Equal(t, indexLink.String(), res.Indexes()[0].String())
	})
}

func mockUCANService(t *testing.T, id principal.Signer, notifyInvocation func(inv invocation.Invocation)) ucanserver.ServerView {
	s, err := ucanserver.NewServer(
		id,
		ucanserver.WithServiceMethod(
			assert.EqualsAbility,
			ucanserver.Provide(
				assert.Equals,
				func(cap ucan.Capability[assert.EqualsCaveats], inv invocation.Invocation, ctx ucanserver.InvocationContext) (ok.Unit, fx.Effects, error) {
					notifyInvocation(inv)
					return ok.Unit{}, nil, nil
				},
			),
		),
		ucanserver.WithServiceMethod(
			assert.IndexAbility,
			ucanserver.Provide(
				assert.Index,
				func(cap ucan.Capability[assert.IndexCaveats], inv invocation.Invocation, ctx ucanserver.InvocationContext) (ok.Unit, fx.Effects, error) {
					notifyInvocation(inv)
					return ok.Unit{}, nil, nil
				},
			),
		),
		ucanserver.WithServiceMethod(
			claim.CacheAbility,
			ucanserver.Provide(
				claim.Cache,
				func(cap ucan.Capability[claim.CacheCaveats], inv invocation.Invocation, ctx ucanserver.InvocationContext) (ok.Unit, fx.Effects, error) {
					notifyInvocation(inv)
					return ok.Unit{}, nil, nil
				},
			),
		),
	)
	require.NoError(t, err)
	return s
}

func mockQueryServer(results bytemap.ByteMap[multihash.Multihash, types.QueryResult]) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mhStrings := r.URL.Query()["multihash"]
		if len(mhStrings) != 1 {
			http.Error(w, "mock query service supports only single hash", http.StatusNotImplemented)
			return
		}

		digest, err := digestutil.Parse(mhStrings[0])
		if err != nil {
			http.Error(w, "invalid digest", http.StatusBadRequest)
			return
		}

		qr := results.Get(digest)
		if qr == nil {
			qr, _ = queryresult.Build(
				make(map[cid.Cid]delegation.Delegation),
				bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1),
			)
		}

		body := car.Encode([]datamodel.Link{qr.Root().Link()}, qr.Blocks())
		w.WriteHeader(http.StatusOK)
		io.Copy(w, body)
	}))
}

func randomLocalURL(t *testing.T) url.URL {
	port := 3000 + rand.IntN(1000)
	pubURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	require.NoError(t, err)
	return *pubURL
}
