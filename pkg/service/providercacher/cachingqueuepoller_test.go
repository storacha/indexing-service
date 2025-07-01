package providercacher_test

import (
	"context"
	"errors"
	"sync"
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

	poller, err := providercacher.NewCachingQueuePoller(
		mockQueue,
		mockCacher,
	)
	require.NoError(t, err)

	// Start the poller
	poller.Start()

	// Give it a moment to start
	time.Sleep(20 * time.Millisecond)

	// Stop the poller
	poller.Stop()
}

func TestCachingQueuePoller_BatchProcessing(t *testing.T) {
	const (
		numJobs       = 11
		batchSize     = 2
		fullBatches   = numJobs / batchSize
		lastBatchSize = numJobs % batchSize
	)

	// Create a single test job that will be used for all batches
	testJob := providercacher.ProviderCachingJob{
		ID:       "test-job",
		Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("test-peer")}},
		Index:    blobindex.NewShardedDagIndexView(nil, 0),
	}

	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	// Set up ReadJobs to return batches of the same job
	batch := make([]providercacher.ProviderCachingJob, batchSize)
	for i := range batchSize {
		batch[i] = testJob
	}
	// Return a number of full batches...
	mockQueue.EXPECT().ReadJobs(mock.Anything, batchSize).Return(batch, nil).Times(fullBatches)
	// ...a final partial batch...
	mockQueue.EXPECT().ReadJobs(mock.Anything, batchSize).Return(batch[:lastBatchSize], nil).Once()
	// ...and block to simulate long-polling with no more jobs available
	mockQueue.EXPECT().ReadJobs(mock.Anything, batchSize).Return([]providercacher.ProviderCachingJob{}, nil).
		Run(func(ctx context.Context, _ int) {
			<-ctx.Done()
		}).
		Return([]providercacher.ProviderCachingJob{}, nil).
		Once()

	// Set up processing barrier to ensure jobs run concurrently
	var wg sync.WaitGroup
	wg.Add(numJobs)

	// Expect CacheProviderForIndexRecords for each job
	mockCacher.EXPECT().
		CacheProviderForIndexRecords(mock.Anything, testJob.Provider, testJob.Index).
		Run(func(ctx context.Context, _ model.ProviderResult, _ blobindex.ShardedDagIndexView) {
			defer wg.Done() // Signal that this job completed processing
		}).
		Return(nil).
		Times(numJobs) // Expect this to be called numJobs times

	// Expect DeleteJob for each job
	mockQueue.EXPECT().
		DeleteJob(mock.Anything, testJob.ID).
		Return(nil).
		Times(numJobs) // Expect this to be called numJobs times

	poller, err := providercacher.NewCachingQueuePoller(
		mockQueue,
		mockCacher,
		providercacher.WithJobBatchSize(batchSize),
	)
	require.NoError(t, err)

	// Start the poller
	poller.Start()

	// Wait for all jobs to start processing
	wg.Wait()

	// Stop the poller
	poller.Stop()
}

func TestCachingQueuePoller_FailedJobsAreNotDeleted(t *testing.T) {
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
	// The first call returns our test jobs, subsequent calls block because there are no more jobs available
	mockQueue.EXPECT().ReadJobs(mock.Anything, mock.Anything).Return(
		[]providercacher.ProviderCachingJob{successfulJob, failedJob}, nil,
	).Once()
	mockQueue.EXPECT().ReadJobs(mock.Anything, mock.Anything).Return([]providercacher.ProviderCachingJob{}, nil).
		Run(func(ctx context.Context, _ int) {
			<-ctx.Done()
		}).
		Return([]providercacher.ProviderCachingJob{}, nil).
		Once()

	// Expect CacheProviderForIndexRecords to be called for both jobs, return an error for failedJob
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, successfulJob.Provider, successfulJob.Index).
		Return(nil).Once()
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, failedJob.Provider, failedJob.Index).
		Return(errors.New("processing error")).Once()

	// Expect DeleteJob to be called only for successfulJob
	mockQueue.EXPECT().DeleteJob(mock.Anything, successfulJob.ID).Return(nil).Once()
	// DeleteJob should NOT be called when processing fails.
	// Adding no explicit expectation here ensures that the test will fail if DeleteJob is called.

	poller, err := providercacher.NewCachingQueuePoller(
		mockQueue,
		mockCacher,
	)
	require.NoError(t, err)

	// Start the poller
	poller.Start()

	// Wait for the job to be processed
	time.Sleep(50 * time.Millisecond)

	// Stop the poller
	poller.Stop()
}
