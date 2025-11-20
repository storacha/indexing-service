package providercacher_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ipni/go-libipni/find/model"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/queuepoller"
	"github.com/storacha/indexing-service/pkg/service/providercacher"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCachingQueuePoller_StartStop(t *testing.T) {
	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	mockQueue.EXPECT().Read(mock.Anything, mock.Anything).Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil)

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
	testJob := queuepoller.WithID[providercacher.ProviderCachingJob]{
		ID: "test-job",
		Job: providercacher.ProviderCachingJob{
			Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("test-peer")}},
			Index:    blobindex.NewShardedDagIndexView(nil, 0),
		},
	}

	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	// Set up Read to return batches of the same job
	batch := make([]queuepoller.WithID[providercacher.ProviderCachingJob], batchSize)
	for i := range batchSize {
		batch[i] = testJob
	}
	// Return a number of full batches...
	mockQueue.EXPECT().Read(mock.Anything, batchSize).Return(batch, nil).Times(fullBatches)
	// ...a final partial batch...
	mockQueue.EXPECT().Read(mock.Anything, batchSize).Return(batch[:lastBatchSize], nil).Once()
	// ...and block to simulate long-polling with no more jobs available
	mockQueue.EXPECT().Read(mock.Anything, batchSize).Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil).
		Run(func(ctx context.Context, _ int) {
			<-ctx.Done()
		}).
		Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil).
		Once()

	// Set up processing barrier to ensure jobs run concurrently
	var wg sync.WaitGroup
	wg.Add(numJobs)

	// Expect CacheProviderForIndexRecords for each job
	mockCacher.EXPECT().
		CacheProviderForIndexRecords(mock.Anything, testJob.Job.Provider, testJob.Job.Index).
		Run(func(ctx context.Context, _ model.ProviderResult, _ blobindex.ShardedDagIndexView) {
			defer wg.Done() // Signal that this job completed processing
		}).
		Return(nil).
		Times(numJobs) // Expect this to be called numJobs times

	// Expect Delete for each job
	mockQueue.EXPECT().
		Delete(mock.Anything, testJob.ID).
		Return(nil).
		Times(numJobs) // Expect this to be called numJobs times

	poller, err := providercacher.NewCachingQueuePoller(
		mockQueue,
		mockCacher,
		queuepoller.WithJobBatchSize(batchSize),
	)
	require.NoError(t, err)

	// Start the poller
	poller.Start()

	// Wait for all jobs to start processing
	wg.Wait()

	// Stop the poller
	poller.Stop()
}

func TestCachingQueuePoller_FailedJobsAreRetried(t *testing.T) {
	// Setup test data
	successfulJob := queuepoller.WithID[providercacher.ProviderCachingJob]{
		ID: "successful-job",
		Job: providercacher.ProviderCachingJob{
			Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("successful-peer")}},
			Index:    blobindex.NewShardedDagIndexView(nil, 0),
		},
	}

	failedJob := queuepoller.WithID[providercacher.ProviderCachingJob]{
		ID: "failed-job",
		Job: providercacher.ProviderCachingJob{
			Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("failed-peer")}},
			Index:    blobindex.NewShardedDagIndexView(nil, 0),
		},
	}

	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	// Expect Read to be called.
	// The first call returns our test jobs, subsequent calls block because there are no more jobs available
	mockQueue.EXPECT().Read(mock.Anything, mock.Anything).Return(
		[]queuepoller.WithID[providercacher.ProviderCachingJob]{successfulJob, failedJob}, nil,
	).Once()
	mockQueue.EXPECT().Read(mock.Anything, mock.Anything).Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil).
		Run(func(ctx context.Context, _ int) {
			<-ctx.Done()
		}).
		Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil).
		Once()

	// Expect CacheProviderForIndexRecords to be called for both jobs, return an error for failedJob
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, successfulJob.Job.Provider, successfulJob.Job.Index).
		Return(nil).Once()
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, failedJob.Job.Provider, failedJob.Job.Index).
		Return(errors.New("processing error")).Once()

	// Expect Delete to be called only for successfulJob
	mockQueue.EXPECT().Delete(mock.Anything, successfulJob.ID).Return(nil).Once()
	// Delete should NOT be called when processing fails.
	// Adding no explicit expectation here ensures that the test will fail if Delete is called.
	// However, the job should be released as it didn't time out.
	mockQueue.EXPECT().Release(mock.Anything, failedJob.ID).Return(nil).Once()

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

func TestCachingQueuePoller_JobsTimingOutAreNotRetried(t *testing.T) {
	// Setup test data
	bigJob := queuepoller.WithID[providercacher.ProviderCachingJob]{
		ID: "big-job",
		Job: providercacher.ProviderCachingJob{
			Provider: model.ProviderResult{Provider: &peer.AddrInfo{ID: peer.ID("peer")}},
			Index:    blobindex.NewShardedDagIndexView(nil, 0),
		},
	}

	// Setup mocks
	mockQueue := providercacher.NewMockCachingQueue(t)
	mockCacher := providercacher.NewMockProviderCacher(t)

	// Expect Read to be called.
	// The first call returns our test job, subsequent calls block because there are no more jobs available
	mockQueue.EXPECT().Read(mock.Anything, mock.Anything).Return(
		[]queuepoller.WithID[providercacher.ProviderCachingJob]{bigJob}, nil,
	).Once()
	mockQueue.EXPECT().Read(mock.Anything, mock.Anything).Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil).
		Run(func(ctx context.Context, _ int) {
			<-ctx.Done()
		}).
		Return([]queuepoller.WithID[providercacher.ProviderCachingJob]{}, nil).
		Once()

	// Expect CacheProviderForIndexRecords to be called for the job, returns a time out error
	mockCacher.EXPECT().CacheProviderForIndexRecords(mock.Anything, bigJob.Job.Provider, bigJob.Job.Index).
		Return(context.DeadlineExceeded).Once()

	// Expect Delete to be called because the error is a time out
	mockQueue.EXPECT().Delete(mock.Anything, bigJob.ID).Return(nil).Once()
	// Release should NOT be called in case of a timeout to avoid retries.
	// Adding no explicit expectation here ensures that the test will fail if Release is called.

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
