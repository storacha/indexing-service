package service

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/result/ok"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPublishClaim(t *testing.T) {
	t.Run("does not publish unknown claims", func(t *testing.T) {
		claim, err := delegation.Delegate(
			testutil.Alice,
			testutil.Bob,
			[]ucan.Capability[ok.Unit]{
				ucan.NewCapability("unknown/claim", testutil.Mallory.DID().String(), ok.Unit{}),
			},
		)
		require.NoError(t, err)
		err = Publish(context.Background(), nil, nil, nil, peer.AddrInfo{}, claim)
		require.ErrorIs(t, err, ErrUnrecognizedClaim)
	})
}

func TestCacheClaim(t *testing.T) {
	t.Run("does not cache unknown claims", func(t *testing.T) {
		claim, err := delegation.Delegate(
			testutil.Alice,
			testutil.Bob,
			[]ucan.Capability[ok.Unit]{
				ucan.NewCapability("unknown/claim", testutil.Mallory.DID().String(), ok.Unit{}),
			},
		)
		require.NoError(t, err)
		err = Cache(context.Background(), nil, nil, nil, peer.AddrInfo{}, claim)
		require.ErrorIs(t, err, ErrUnrecognizedClaim)
	})
}
