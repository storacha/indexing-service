package client

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/alanshaw/storetheindex/config"
	"github.com/alanshaw/storetheindex/ingest"
	"github.com/alanshaw/storetheindex/registry"
	httpfind "github.com/alanshaw/storetheindex/server/find"
	httpingest "github.com/alanshaw/storetheindex/server/ingest"
	"github.com/ipfs/go-datastore"
	"github.com/ipni/go-indexer-core/store/memory"
	"github.com/ipni/go-libipni/maurl"
	"github.com/ipni/index-provider/engine"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/indexing-service/pkg/construct"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/server"
	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	ipniFindURL := randomLocalURL(t)
	ipniAnnounceURL := randomLocalURL(t)

	shutdownIPNI := startIPNIService(t, ipniFindURL, ipniAnnounceURL)
	t.Cleanup(shutdownIPNI)

	serviceID := testutil.Service
	serviceURL := randomLocalURL(t)

	shutdownService := startIndexingService(t, testutil.Service, serviceURL, ipniFindURL, ipniAnnounceURL)
	t.Cleanup(shutdownService)

	c, err := New(serviceID, *serviceURL)
	require.NoError(t, err)

}

func startIPNIService(
	t *testing.T,
	announceURL url.URL,
	findURL url.URL,
) func() {
	indexerCore := engine.New(memory.New())

	reg, err := registry.New(
		context.Background(),
		config.NewDiscovery(),
		datastore.NewMapDatastore(),
	)
	require.NoError(t, err)

	p2pHost, err := libp2p.New()
	require.NoError(t, err)

	ingConfig := config.NewIngest()
	ingConfig.PubSubTopic = "/storacha/indexer/ingest/testnet"
	ing, err := ingest.NewIngester(
		ingConfig,
		p2pHost,
		indexerCore,
		reg,
		datastore.NewMapDatastore(),
		datastore.NewMapDatastore(),
	)
	require.NoError(t, err)

	announceAddr := fmt.Sprintf("%s:%s", announceURL.Hostname(), announceURL.Port())
	ingSvr, err := httpingest.New(announceAddr, indexerCore, ing, reg)
	require.NoError(t, err)

	go func() {
		err = ingSvr.Start()
	}()

	findAddr := fmt.Sprintf("%s:%s", findURL.Hostname(), findURL.Port())
	findSvr, err := httpfind.New(findAddr, indexerCore, reg)

	go func() {
		err = findSvr.Start()
	}()

	time.Sleep(time.Millisecond * 100)
	require.NoError(t, err)
}

func startIndexingService(
	t *testing.T,
	id principal.Signer,
	publicURL url.URL,
	indexerURL url.URL,
	directAnnounceURL url.URL,
) func() {
	privKey, err := crypto.UnmarshalEd25519PrivateKey(id.Raw())
	require.NoError(t, err)

	publisherListenURL := randomLocalURL(t)
	announceAddr, err := maurl.FromURL(&publisherListenURL)
	require.NoError(t, err)

	cfg := construct.ServiceConfig{
		PrivateKey:                  privKey,
		PublicURL:                   []string{publicURL.String()},
		IndexerURL:                  indexerURL.String(),
		PublisherDirectAnnounceURLs: []string{directAnnounceURL.String()},
		PublisherListenAddr:         fmt.Sprintf("%s:%s", publisherListenURL.Hostname(), publisherListenURL.Port()),
		PublisherAnnounceAddrs:      []string{announceAddr.String()},
	}
	indexer, err := construct.Construct(
		cfg,
		construct.WithStartIPNIServer(true),
		construct.WithDatastore(datastore.NewMapDatastore()),
		construct.WithProvidersClient(redis.NewMapStore()),
		construct.WithClaimsClient(redis.NewMapStore()),
		construct.WithIndexesClient(redis.NewMapStore()),
	)
	require.NoError(t, err)

	err = indexer.Startup(context.Background())
	require.NoError(t, err)

	go func() {
		addr := fmt.Sprintf("%s:%s", publicURL.Hostname(), publicURL.Port())
		server.ListenAndServe(addr, indexer, server.WithIdentity(id))
	}()

	return func() {
		indexer.Shutdown(context.Background())
	}
}

func randomLocalURL(t *testing.T) url.URL {
	port := testutil.GetFreePort(t)
	pubURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	require.NoError(t, err)
	return *pubURL
}
