package blobindexlookup_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/metadata"
	"github.com/storacha/indexing-service/pkg/service/blobindexlookup"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestWithCache__Find(t *testing.T) {

	// Create a test CID
	cachedContextID := testutil.RandomBytes(16)
	notCachedContextID := testutil.RandomBytes(16)
	// Create a cached index
	_, cachedIndex := testutil.RandomShardedDagIndexView(32)
	_, notCachedIndex := testutil.RandomShardedDagIndexView(32)

	// Create provider
	provider := testutil.RandomProviderResult()

	// sample error
	anError := errors.New("something went wrong")
	// Define test cases
	testCases := []struct {
		name           string
		contextID      types.EncodedContextID
		setErr         error
		getErr         error
		expectedErr    error
		baseLookup     *mockBlobIndexLookup
		providerCacher *mockCachingQueue
		expectedIndex  blobindex.ShardedDagIndexView
		finalState     map[string]blobindex.ShardedDagIndexView
	}{
		{
			name:          "Index cached",
			contextID:     cachedContextID,
			expectedIndex: cachedIndex,
			finalState: map[string]blobindex.ShardedDagIndexView{
				string(cachedContextID): cachedIndex,
			},
		},
		{
			name:          "Index not cached, successful fetch",
			contextID:     notCachedContextID,
			expectedIndex: notCachedIndex,
			finalState: map[string]blobindex.ShardedDagIndexView{
				string(cachedContextID):    cachedIndex,
				string(notCachedContextID): notCachedIndex,
			},
		},
		{
			name:          "Lookup error",
			contextID:     cachedContextID,
			expectedIndex: nil,
			getErr:        anError,
			expectedErr:   fmt.Errorf("reading from index cache: %w", anError),
			finalState: map[string]blobindex.ShardedDagIndexView{
				string(cachedContextID): cachedIndex,
			},
		},
		{
			name:          "Save cache error",
			contextID:     notCachedContextID,
			expectedIndex: nil,
			setErr:        anError,
			expectedErr:   fmt.Errorf("caching fetched index: %w", anError),
			finalState: map[string]blobindex.ShardedDagIndexView{
				string(cachedContextID): cachedIndex,
			},
		},
		{
			name:          "underlying lookup error",
			contextID:     notCachedContextID,
			expectedIndex: nil,
			baseLookup:    &mockBlobIndexLookup{nil, anError},
			expectedErr:   fmt.Errorf("fetching underlying index: %w", anError),
			finalState: map[string]blobindex.ShardedDagIndexView{
				string(cachedContextID): cachedIndex,
			},
		},
		{
			name:           "provider cacher error",
			contextID:      notCachedContextID,
			expectedIndex:  nil,
			providerCacher: &mockCachingQueue{anError},
			expectedErr:    fmt.Errorf("queueing provider caching for index failed: %w", anError),
			finalState: map[string]blobindex.ShardedDagIndexView{
				string(cachedContextID):    cachedIndex,
				string(notCachedContextID): notCachedIndex,
			},
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockStore := &MockShardedDagIndexStore{
				setErr: tc.setErr,
				getErr: tc.getErr,
				indexes: map[string]blobindex.ShardedDagIndexView{
					string(cachedContextID): cachedIndex,
				},
			}
			lookup := tc.baseLookup
			if lookup == nil {
				lookup = &mockBlobIndexLookup{notCachedIndex, nil}
			}
			providerCacher := tc.providerCacher
			if providerCacher == nil {
				providerCacher = &mockCachingQueue{nil}
			}
			// Create ClaimLookup instance
			cl := blobindexlookup.WithCache(lookup, mockStore, providerCacher)

			index, err := cl.Find(context.Background(), tc.contextID, provider, *testutil.TestURL, nil)
			if tc.expectedErr != nil {
				require.EqualError(t, err, tc.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
			testutil.RequireEqualIndex(t, tc.expectedIndex, index)
			finalState := tc.finalState
			if finalState == nil {
				finalState = make(map[string]blobindex.ShardedDagIndexView)
			}
			require.Equal(t, len(finalState), len(mockStore.indexes))
			for c, index := range mockStore.indexes {
				expectedIndex := finalState[c]
				testutil.RequireEqualIndex(t, expectedIndex, index)
			}
		})
	}
}

// MockShardedDagIndexStore is a mock implementation of the ShardedDagIndexStore interface
type MockShardedDagIndexStore struct {
	setErr, getErr error
	indexes        map[string]blobindex.ShardedDagIndexView
}

var _ types.ShardedDagIndexStore = &MockShardedDagIndexStore{}

// SetExpirable implements types.ShardedDagIndexStore.
func (m *MockShardedDagIndexStore) SetExpirable(ctx context.Context, contextID types.EncodedContextID, expires bool) error {
	return nil
}

func (m *MockShardedDagIndexStore) Get(ctx context.Context, contextID types.EncodedContextID) (blobindex.ShardedDagIndexView, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	index, exists := m.indexes[string(contextID)]
	if !exists {
		return nil, types.ErrKeyNotFound
	}
	return index, nil
}

func (m *MockShardedDagIndexStore) Set(ctx context.Context, contextID types.EncodedContextID, index blobindex.ShardedDagIndexView, expire bool) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.indexes[string(contextID)] = index
	return nil
}

type mockBlobIndexLookup struct {
	index blobindex.ShardedDagIndexView
	err   error
}

func (m *mockBlobIndexLookup) Find(ctx context.Context, contextID types.EncodedContextID, provider model.ProviderResult, fetchURL url.URL, rng *metadata.Range) (blobindex.ShardedDagIndexView, error) {
	return m.index, m.err
}

type mockCachingQueue struct {
	err error
}

// QueueProviderCaching implements blobindexlookup.ProviderCacher.
func (m *mockCachingQueue) Queue(ctx context.Context, job providercacher.ProviderCachingJob) error {
	return m.err
}
