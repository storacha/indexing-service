package service

import (
	"context"
	"net/url"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
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

func TestUrlForResource(t *testing.T) {
	const addrBase = "/dns/storacha.network/https/http-path/"
	testCases := []struct {
		name        string
		addrs       []multiaddr.Multiaddr
		placeholder string
		id          string
		expectedUrl string
		expectErr   bool
	}{
		{
			name: "happy path",
			addrs: []multiaddr.Multiaddr{
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/claims/{claim}")))(t),
			},
			placeholder: "{claim}",
			id:          "123",
			expectedUrl: "https://storacha.network/claims/123",
			expectErr:   false,
		},
		{
			name: "multiple addresses, uses the first one that contains the placeholder",
			addrs: []multiaddr.Multiaddr{
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/blobs/{blob}")))(t),
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/claims1/{claim}")))(t),
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/claims2/{claim}")))(t),
			},
			placeholder: "{claim}",
			id:          "123",
			expectedUrl: "https://storacha.network/claims1/123",
			expectErr:   false,
		},
		{
			name:        "no addresses in peer addr info",
			addrs:       []multiaddr.Multiaddr{},
			placeholder: "{claim}",
			expectedUrl: "",
			expectErr:   true,
		},
		{
			name: "no address contains the placeholder",
			addrs: []multiaddr.Multiaddr{
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/claims/{foo}")))(t),
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/claims/{bar}")))(t),
				testutil.Must(multiaddr.NewMultiaddr(addrBase + url.PathEscape("/claims/{baz}")))(t),
			},
			placeholder: "{claim}",
			expectedUrl: "",
			expectErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := peer.AddrInfo{
				Addrs: tc.addrs,
			}
			u, err := urlForResource(provider, tc.placeholder, tc.id)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedUrl, u.String())
			}
		})
	}
}
