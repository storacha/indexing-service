package providerresults_test

import (
	"testing"

	"github.com/ipni/go-libipni/find/model"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/providerresults"
	"github.com/stretchr/testify/require"
)

func TestProviderResults__Equals(t *testing.T) {
	testProvider := testutil.RandomProviderResult()
	testProvider2 := testutil.RandomProviderResult()

	// Create slightly modified versions of the testProvider
	contextIDChanged := testProvider
	contextIDChanged.ContextID = testutil.RandomBytes(10)

	metadataChanged := testProvider
	metadataChanged.Metadata = testutil.RandomBytes(10)

	nullProvider := testProvider
	nullProvider.Provider = nil

	providerPeerIDChanged := testProvider
	providerPeerIDChanged.Provider = &peer.AddrInfo{
		ID:    testutil.RandomPeer(),
		Addrs: testProvider.Provider.Addrs,
	}

	providerAddrsChanged := testProvider
	providerAddrsChanged.Provider = &peer.AddrInfo{
		ID:    testProvider.Provider.ID,
		Addrs: []multiaddr.Multiaddr{testutil.RandomMultiaddr(), testutil.RandomMultiaddr()},
	}

	testCases := []struct {
		name     string
		provider model.ProviderResult
		assert   require.BoolAssertionFunc
	}{
		{"same provider", testProvider, require.True},
		{"full alternate", testProvider2, require.False},
		{"context ID changed", contextIDChanged, require.False},
		{"metadata changed", metadataChanged, require.False},
		{"provider changed to null", nullProvider, require.False},
		{"provider peer ID changed", providerPeerIDChanged, require.False},
		{"provider addrs changed", providerAddrsChanged, require.False},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.assert(t, providerresults.Equals(testProvider, testCase.provider))
		})
	}
}

func TestSerialization(t *testing.T) {
	randomResult := testutil.RandomProviderResult()

	testCases := []struct {
		name       string
		testResult model.ProviderResult
	}{
		{
			name:       "random result",
			testResult: randomResult,
		},
		{
			name: "empty peer ID",
			testResult: model.ProviderResult{
				ContextID: randomResult.ContextID,
				Metadata:  randomResult.Metadata,
				Provider: &peer.AddrInfo{
					ID:    "",
					Addrs: randomResult.Provider.Addrs,
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			marshalled := testutil.Must(providerresults.MarshalCBOR(tc.testResult))(t)
			unmarshalled := testutil.Must(providerresults.UnmarshalCBOR(marshalled))(t)
			require.Equal(t, tc.testResult, unmarshalled)
		})
	}
}
