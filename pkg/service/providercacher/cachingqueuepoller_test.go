package providercacher_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCachingQueuePoller_StartStop(t *testing.T) {
	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	mockQueue.EXPECT().ReadJobs(mock.Anything, mock.Anything).Return([]providercacher.ProviderCachingJob{}, nil)

	// Create poller with short poll interval for faster tests
	poller, err := providercacher.NewCachingQueuePoller(
		mockQueue,
		mockCacher,
		providercacher.WithPollInterval(10*time.Millisecond),
	)
	require.NoError(t, err)

	// Start the poller
	poller.Start()

	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	// Stop the poller
	poller.Stop()
}

func TestCachingQueuePoller_ProcessJobs(t *testing.T) {
	// Setup test data
	successfulJob := providercacher.ProviderCachingJob{
		ID:       "successful-job",
		Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("successful-peer")}},
		Index:    blobindex.NewShardedDagIndexView(nil, 0),
	}

	failedJob := providercacher.ProviderCachingJob{
		ID:       "failed-job",
		Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("failed-peer")}},
		Index:    blobindex.NewShardedDagIndexView(nil, 0),
	}

	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	// Expect ReadJobs to be called.
	// The first call returns our test jobs, subsequent calls return no new jobs
	mockQueue.EXPECT().ReadJobs(mock.Anything, mock.Anything).Return(
		[]providercacher.ProviderCachingJob{successfulJob, failedJob}, nil,
	).Once()
	mockQueue.EXPECT().ReadJobs(mock.Anything, mock.Anything).Return([]providercacher.ProviderCachingJob{}, nil)

	// Expect CacheProviderForIndexRecords to be called for both jobs, return an error for failedJob
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, successfulJob.Provider, successfulJob.Index).
		Return(nil).Once()
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, failedJob.Provider, failedJob.Index).
		Return(errors.New("processing error")).Once()

	// Expect DeleteJob to be called only for successfulJob
	mockQueue.EXPECT().DeleteJob(mock.Anything, successfulJob.ID).Return(nil).Once()
	// DeleteJob should NOT be called when processing fails.
	// Adding no explicit expectation here ensures that the test will fail if DeleteJob is called.

	// Create poller
	poller, err := providercacher.NewCachingQueuePoller(
		mockQueue,
		mockCacher,
		providercacher.WithPollInterval(10*time.Millisecond),
	)
	require.NoError(t, err)

	// Start the poller
	poller.Start()

	// Wait for the job to be processed
	time.Sleep(50 * time.Millisecond)

	// Stop the poller
	poller.Stop()
}
