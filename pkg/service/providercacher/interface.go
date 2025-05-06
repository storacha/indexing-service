package providercacher

import (
	"iter"

	"github.com/ipni/go-libipni/find/model"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/types"
)

// CacheProviderMessage instructs caching the provider result for each digest.
type CacheProviderMessage struct {
	Provider model.ProviderResult
	Digests  iter.Seq[mh.Multihash]
}

// ProviderCachingQueue asynchronously caches provider information for the
// passed digests in a [types.ProviderStore].
type ProviderCachingQueue types.Queue[CacheProviderMessage]
