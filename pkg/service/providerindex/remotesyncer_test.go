package providerindex

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/ingest/schema"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/bytemap"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func dagCBORLink(t *testing.T, n ipld.Node) ipld.Link {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	err := dagcbor.Encode(n, buf)
	require.NoError(t, err)
	codec := uint64(multicodec.DagCbor)
	digest := testutil.Must(multihash.Sum(buf.Bytes(), multihash.SHA2_256, -1))(t)
	return cidlink.Link{Cid: cid.NewCidV1(codec, digest)}
}

func TestHandleRemoteSync(t *testing.T) {
	ads := map[string]schema.Advertisement{}
	ents := map[string]schema.EntryChunk{}
	ipniStore := mockIpniStore{ads, ents}
	providerStore := mockProviderStore{
		data: bytemap.NewByteMap[multihash.Multihash, *valueExpirable[[]model.ProviderResult]](-1),
	}

	var hd ipld.Link
	for range 5 {
		ad := schema.Advertisement{}
		if hd != nil {
			ad.PreviousID = hd
		}

		var ec schema.EntryChunk
		for range rand.IntN(100) {
			digest := testutil.RandomMultihash()
			ec.Entries = append(ec.Entries, digest)
			_, err := providerStore.Add(context.Background(), digest, model.ProviderResult{})
			require.NoError(t, err)
		}
		n := testutil.Must(ec.ToNode())(t)
		eclnk := dagCBORLink(t, n)
		ents[eclnk.String()] = ec
		ad.Entries = eclnk

		n = testutil.Must(ad.ToNode())(t)
		adlink := dagCBORLink(t, n)
		ads[adlink.String()] = ad
		hd = adlink
	}

	prev := ads[hd.String()].PreviousID

	syncer := NewRemoteSyncer(&providerStore, &ipniStore)
	syncer.HandleRemoteSync(context.Background(), prev, nil)
	requireExpirable(t, &providerStore, &ipniStore, hd, prev, false)
	requireExpirable(t, &providerStore, &ipniStore, prev, nil, true)

	syncer.HandleRemoteSync(context.Background(), hd, prev)
	requireExpirable(t, &providerStore, &ipniStore, hd, nil, true)
}

func requireExpirable(t *testing.T, providerStore *mockProviderStore, ipniStore *mockIpniStore, head ipld.Link, tail ipld.Link, expirable bool) {
	cur := head
	for cur != tail {
		ad, err := ipniStore.Advert(context.Background(), cur)
		require.NoError(t, err)

		for digest, err := range ipniStore.Entries(context.Background(), ad.Entries) {
			require.NoError(t, err)
			exp, err := providerStore.GetExpiration(context.Background(), digest)
			require.NoError(t, err)
			require.Equal(t, expirable, !exp.IsZero())
		}
		cur = ad.PreviousID
	}
}

type mockIpniStore struct {
	ads  map[string]schema.Advertisement
	ents map[string]schema.EntryChunk
}

func (i *mockIpniStore) Advert(ctx context.Context, id datamodel.Link) (schema.Advertisement, error) {
	ad, ok := i.ads[id.String()]
	if !ok {
		return schema.Advertisement{}, fmt.Errorf("not found: %s", id.String())
	}
	return ad, nil
}

func (i *mockIpniStore) Entries(ctx context.Context, root datamodel.Link) iter.Seq2[multihash.Multihash, error] {
	return func(yield func(multihash.Multihash, error) bool) {
		ent := i.ents[root.String()]
		for _, digest := range ent.Entries {
			if !yield(digest, nil) {
				return
			}
		}
	}
}

var _ Store = (*mockIpniStore)(nil)

type valueExpirable[T any] struct {
	val T
	exp time.Time
}

type mockProviderStore struct {
	data bytemap.ByteMap[multihash.Multihash, *valueExpirable[[]model.ProviderResult]]
}

func (m *mockProviderStore) Get(ctx context.Context, key multihash.Multihash) ([]model.ProviderResult, error) {
	val := m.data.Get(key)
	if val == nil {
		return nil, types.ErrKeyNotFound
	}
	if !val.exp.IsZero() && time.Now().After(val.exp) {
		return nil, types.ErrKeyNotFound
	}
	return val.val, nil
}

func (m *mockProviderStore) Add(ctx context.Context, digest multihash.Multihash, newProviders ...model.ProviderResult) (uint64, error) {
	var providers []model.ProviderResult
	var expires time.Time
	existing := m.data.Get(digest)
	if existing != nil {
		providers = append(providers, existing.val...)
		expires = existing.exp
	}
	written := uint64(0)
	for _, provider := range newProviders {
		if !slices.ContainsFunc(providers, func(p model.ProviderResult) bool { return p.Equal(provider) }) {
			providers = append(providers, provider)
			written++
		}
	}
	v := valueExpirable[[]model.ProviderResult]{providers, expires}
	m.data.Set(digest, &v)
	return written, nil
}

func (m *mockProviderStore) GetExpiration(ctx context.Context, key multihash.Multihash) (time.Time, error) {
	val := m.data.Get(key)
	if val == nil {
		return time.Time{}, types.ErrKeyNotFound
	}
	return val.exp, nil
}

func (m *mockProviderStore) SetExpirable(ctx context.Context, key multihash.Multihash, expires bool) error {
	val := m.data.Get(key)
	if val == nil {
		return types.ErrKeyNotFound
	}
	if expires {
		val.exp = time.Now().Add(time.Second * 30)
	} else {
		val.exp = time.Time{}
	}
	return nil
}

var _ types.ProviderStore = (*mockProviderStore)(nil)
