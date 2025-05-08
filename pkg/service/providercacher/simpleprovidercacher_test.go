package providercacher_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/ipni/go-libipni/find/model"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestSimpleProviderCacher_CacheProviderForIndexRecords(t *testing.T) {

	// Create a test context
	ctx := context.Background()

	// Create test providers
	testProvider := testutil.RandomProviderResult()
	testProvider2 := testutil.RandomProviderResult()

	// Create random CIDs
	testCid1 := testutil.RandomCID()
	sliceMhs := testutil.RandomMultihashes(6)
	sliceMhs = append(sliceMhs, link.ToCID(testCid1).Hash())

	evensFilled := func() map[string][]model.ProviderResult {
		starter := make(map[string][]model.ProviderResult)
		for i, sliceMh := range sliceMhs {
			if i%2 == 0 {
				starter[sliceMh.String()] = []model.ProviderResult{testProvider}
			}
		}
		return starter
	}

	// Define test cases
	testCases := []struct {
		name          string
		provider      model.ProviderResult
		digests       []multihash.Multihash
		getErr        error
		setErr        error
		initialStore  map[string][]model.ProviderResult
		expectedCount uint64
		expectedErr   error
		testStore     func(t *testing.T, store map[string][]model.ProviderResult)
	}{
		{
			name:          "Cache new provider",
			provider:      testProvider,
			digests:       sliceMhs,
			expectedCount: 7,
			expectedErr:   nil,
			testStore: func(t *testing.T, store map[string][]model.ProviderResult) {
				require.Len(t, store, 7)
				for _, sliceMh := range sliceMhs {
					require.Equal(t, store[sliceMh.String()], []model.ProviderResult{testProvider})
				}
			},
		},
		{
			name:          "Cache provider already present",
			provider:      testProvider,
			digests:       sliceMhs,
			initialStore:  evensFilled(),
			expectedCount: 3,
			expectedErr:   nil,
			testStore: func(t *testing.T, store map[string][]model.ProviderResult) {
				require.Len(t, store, 7)
				for _, sliceMh := range sliceMhs {
					require.Equal(t, store[sliceMh.String()], []model.ProviderResult{testProvider})
				}
			},
		},
		{
			name:          "Cache another provider on top",
			provider:      testProvider2,
			digests:       sliceMhs,
			initialStore:  evensFilled(),
			expectedCount: 7,
			expectedErr:   nil,
			testStore: func(t *testing.T, store map[string][]model.ProviderResult) {
				require.Len(t, store, 7)
				for i, sliceMh := range sliceMhs {
					expected := []model.ProviderResult{testProvider2}
					if i%2 == 0 {
						expected = []model.ProviderResult{testProvider, testProvider2}
					}
					require.Equal(t, store[sliceMh.String()], expected)
				}
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Initialize mock store
			initialStore := tc.initialStore
			if initialStore == nil {
				initialStore = make(map[string][]model.ProviderResult)
			}
			mockStore := &MockProviderStore{
				setErr: tc.setErr,
				getErr: tc.getErr,
				store:  initialStore,
			}

			// Create SimpleProviderCacher instance
			cacher := providercacher.NewSimpleProviderCacher(mockStore)

			err := cacher.Queue(ctx, providercacher.CacheProviderMessage{
				Provider: tc.provider,
				Digests:  slices.Values(tc.digests),
			})
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr.Error())
			}
			require.Equal(t, tc.expectedCount, mockStore.written)
			if tc.testStore != nil {
				tc.testStore(t, mockStore.store)
			}
		})
	}
}

// Simulate a 10k NFT - a directory with 10k image/metadata files
func TestSimpleProviderCacher_10kNFT(t *testing.T) {
	// Create a test context
	ctx := context.Background()

	prov := testutil.RandomProviderResult()
	root := testutil.RandomCID()

	// make 10_000 unique hashes
	sliceDigests := make([]multihash.Multihash, 0, 10_000)
	for range 10_000 {
		for {
			digest := testutil.RandomMultihash()
			if slices.ContainsFunc(sliceDigests, func(s multihash.Multihash) bool { return s.String() == digest.String() }) {
				continue
			}
			sliceDigests = append(sliceDigests, digest)
			break
		}
	}

	mockStore := &MockProviderStore{store: map[string][]model.ProviderResult{}}
	cacher := providercacher.NewSimpleProviderCacher(mockStore)
	err := cacher.Queue(ctx, providercacher.CacheProviderMessage{
		Provider: prov,
		Digests: func(yield func(digest multihash.Multihash) bool) {
			if !yield(link.ToCID(root).Hash()) {
				return
			}
			for _, d := range sliceDigests {
				if !yield(d) {
					return
				}
			}
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1+len(sliceDigests), len(mockStore.store))
}

// MockProviderStore is a mock implementation of the ProviderStore interface
type MockProviderStore struct {
	setErr, getErr error
	store          map[string][]model.ProviderResult
	mutex          sync.RWMutex
	written        uint64
}

var _ types.ProviderStore = &MockProviderStore{}

func (m *MockProviderStore) Members(ctx context.Context, hash multihash.Multihash) ([]model.ProviderResult, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	results, exists := m.store[hash.String()]
	if !exists {
		return nil, types.ErrKeyNotFound
	}
	return results, nil
}

func (m *MockProviderStore) Add(ctx context.Context, hash multihash.Multihash, providers ...model.ProviderResult) (uint64, error) {
	written := uint64(0)
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for _, provider := range providers {
		providers := m.store[hash.String()]
		if !slices.ContainsFunc(providers, func(p model.ProviderResult) bool { return p.Equal(provider) }) {
			m.store[hash.String()] = append(providers, provider)
			written++
		}
	}
	m.written += written
	return written, nil
}

// SetExpirable implements types.ProviderStore.
func (m *MockProviderStore) SetExpirable(ctx context.Context, key multihash.Multihash, expires bool) error {
	return nil
}
