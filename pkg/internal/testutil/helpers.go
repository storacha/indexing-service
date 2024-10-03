package testutil

import (
	"testing"

	"github.com/storacha-network/go-ucanto/core/delegation"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
	"github.com/stretchr/testify/require"
)

// Must takes return values from a function and returns the non-error one. If
// the error value is non-nil then it fails the test
func Must[T any](val T, err error) func(*testing.T) T {
	return func(t *testing.T) T {
		require.NoError(t, err)
		return val
	}
}

// Must2 takes return values from a 3 return function and returns the non-error ones. If
// the error value is non-nil then it fails the test.
func Must2[T, U any](val1 T, val2 U, err error) func(*testing.T) (T, U) {
	return func(t *testing.T) (T, U) {
		require.NoError(t, err)
		return val1, val2
	}
}

// RequireEqualIndex compares two sharded dag indexes to verify their equality
func RequireEqualIndex(t *testing.T, expectedIndex blobindex.ShardedDagIndexView, actualIndex blobindex.ShardedDagIndexView) {
	if expectedIndex == nil {
		require.Nil(t, actualIndex)
		return
	}
	require.NotZero(t, actualIndex.Shards().Size())
	require.Equal(t, expectedIndex.Shards().Size(), actualIndex.Shards().Size())
	for key, shard := range actualIndex.Shards().Iterator() {
		require.True(t, expectedIndex.Shards().Has(key))
		expectedShard := expectedIndex.Shards().Get(key)
		require.Equal(t, expectedShard.Size(), shard.Size())
		for mh, position := range shard.Iterator() {
			require.True(t, expectedShard.Has(mh))
			require.Equal(t, expectedShard.Get(mh), position)
		}
	}
}

// RequireEqualDelegation compares two delegations to verify their equality
func RequireEqualDelegation(t *testing.T, expectedDelegation delegation.Delegation, actualDelegation delegation.Delegation) {
	if expectedDelegation == nil {
		require.Nil(t, actualDelegation)
		return
	}
	require.Equal(t, expectedDelegation.Issuer(), actualDelegation.Issuer())
	require.Equal(t, expectedDelegation.Audience(), actualDelegation.Audience())
	require.Equal(t, expectedDelegation.Capabilities(), actualDelegation.Capabilities())
	require.Equal(t, expectedDelegation.Expiration(), actualDelegation.Expiration())
	require.Equal(t, expectedDelegation.Signature(), actualDelegation.Signature())
	require.Equal(t, expectedDelegation.Version(), actualDelegation.Version())
	require.Equal(t, expectedDelegation.Facts(), actualDelegation.Facts())
	require.Equal(t, expectedDelegation.Nonce(), actualDelegation.Nonce())
	require.Equal(t, expectedDelegation.NotBefore(), actualDelegation.NotBefore())
	require.Equal(t, expectedDelegation.Proofs(), actualDelegation.Proofs())
}
