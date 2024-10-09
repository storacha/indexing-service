package publisher

import (
	"context"
	"testing"

	"github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPublish(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(nil)
	require.NoError(t, err)

	pid, err := peer.IDFromPrivateKey(priv)
	require.NoError(t, err)

	provInfo := peer.AddrInfo{ID: pid}

	publisher, err := New(priv)
	require.NoError(t, err)

	ctx := context.Background()
	err = publisher.Start(ctx)
	require.NoError(t, err)

	defer func() {
		err := publisher.Shutdown()
		require.NoError(t, err)
	}()

	var digests []multihash.Multihash
	for range 10 {
		digests = append(digests, testutil.RandomMultihash())
	}

	adlnk, err := publisher.Publish(ctx, &provInfo, "test0", digests, metadata.Default.New())
	require.NoError(t, err)

	ad, err := publisher.Store().Advert(ctx, adlnk)
	require.NoError(t, err)

	var ents []multihash.Multihash
	for e, err := range publisher.Store().Entries(ctx, ad.Entries) {
		require.NoError(t, err)
		ents = append(ents, e)
	}

	require.Equal(t, digests, ents)
}
