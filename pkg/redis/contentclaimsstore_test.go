package redis_test

import (
	"context"
	"io"
	"net/url"
	"testing"

	cid "github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-capabilities/pkg/assert"
	adm "github.com/storacha/go-capabilities/pkg/assert/datamodel"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/stretchr/testify/require"
)

func TestContentClaimsStore(t *testing.T) {
	mockRedis := NewMockRedis()
	contentClaimsStore := redis.NewContentClaimsStore(mockRedis)
	claim1 := assert.Location.New(testutil.Service.DID().String(), assert.LocationCaveats{
		Content:  testutil.Must(assert.Digest(adm.DigestModel{Digest: testutil.RandomMultihash()}))(t),
		Location: []url.URL{*testutil.Must(url.Parse("https://storacha.network"))(t)},
	})
	delegation1 := testutil.Must(delegation.Delegate(testutil.Service, testutil.Alice, []ucan.Capability[assert.LocationCaveats]{claim1}))(t)
	claim2 := assert.Index.New(testutil.Service.DID().String(), assert.IndexCaveats{
		Content: testutil.RandomCID(),
		Index:   testutil.RandomCID(),
	})
	delegation1Cid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(delegation1)))(t)))(t)
	delegation2 := testutil.Must(delegation.Delegate(testutil.Service, testutil.Service, []ucan.Capability[assert.IndexCaveats]{claim2}))(t)
	delegation2Cid := testutil.Must(cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(testutil.Must(io.ReadAll(delegation.Archive(delegation2)))(t)))(t)
	ctx := context.Background()
	require.NoError(t, contentClaimsStore.Set(ctx, delegation1Cid, delegation1, false))
	require.NoError(t, contentClaimsStore.Set(ctx, delegation2Cid, delegation2, true))

	returnedDelegation1 := testutil.Must(contentClaimsStore.Get(ctx, delegation1Cid))(t)
	returnedDelegation2 := testutil.Must(contentClaimsStore.Get(ctx, delegation2Cid))(t)
	testutil.RequireEqualDelegation(t, delegation1, returnedDelegation1)
	testutil.RequireEqualDelegation(t, delegation2, returnedDelegation2)
}
