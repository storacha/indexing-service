package aws

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestDynamoMigratedShardChecker(t *testing.T) {
	if os.Getenv("CI") != "" && runtime.GOOS != "linux" {
		t.SkipNow()
	}

	ctx := context.Background()
	endpoint := createDynamo(t)
	dynamoClient := newDynamoClient(t, endpoint)

	storeTable := "store-" + uuid.NewString()
	blobRegistryTable := "blob-registry-" + uuid.NewString()
	allocationsTable := "allocations-" + uuid.NewString()
	createStoreTable(t, dynamoClient, storeTable)
	createBlobRegistryTable(t, dynamoClient, blobRegistryTable)
	createAllocationsTable(t, dynamoClient, allocationsTable)
	allocationsStore := NewDynamoAllocationsTable(dynamoClient, allocationsTable)
	checker := NewDynamoMigratedShardChecker(dynamoClient, dynamoClient, blobRegistryTable, storeTable, allocationsStore)

	t.Run("exists in store table", func(t *testing.T) {

		cid := testutil.RandomCID()
		space := testutil.RandomPrincipal().DID()
		_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(storeTable),
			Item: map[string]types.AttributeValue{
				"link":  &types.AttributeValueMemberS{Value: cid.String()},
				"space": &types.AttributeValueMemberS{Value: space.DID().String()},
			},
		})
		require.NoError(t, err)

		has, err := checker.ShardMigrated(ctx, cid)
		require.NoError(t, err)
		require.True(t, has, "expected shard to be migrated in store table")
	})

	t.Run("exists in blob registry table", func(t *testing.T) {

		cid := testutil.RandomCID()
		space := testutil.RandomPrincipal().DID()
		_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(blobRegistryTable),
			Item: map[string]types.AttributeValue{
				"digest": &types.AttributeValueMemberS{Value: cid.(cidlink.Link).Cid.Hash().B58String()},
				"space":  &types.AttributeValueMemberS{Value: space.DID().String()},
			},
		})
		require.NoError(t, err)

		has, err := checker.ShardMigrated(ctx, cid)
		require.NoError(t, err)
		require.True(t, has, "expected shard to be migrated in store table")
	})

	t.Run("exists in allocations table", func(t *testing.T) {

		cid := testutil.RandomCID()
		space := testutil.RandomPrincipal().DID()
		_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(allocationsTable),
			Item: map[string]types.AttributeValue{
				"multihash": &types.AttributeValueMemberS{Value: digestutil.Format(cid.(cidlink.Link).Cid.Hash())},
				"space":     &types.AttributeValueMemberS{Value: space.DID().String()},
			},
		})
		require.NoError(t, err)

		has, err := checker.ShardMigrated(ctx, cid)
		require.NoError(t, err)
		require.True(t, has, "expected shard to be migrated in allocations table")
	})

	t.Run("does not exist in any table", func(t *testing.T) {
		cid := testutil.RandomCID()
		has, err := checker.ShardMigrated(ctx, cid)
		require.NoError(t, err)
		require.False(t, has, "expected shard to not be migrated in any table")
	})
}

func createStoreTable(t *testing.T, c *dynamodb.Client, tableName string) {
	_, err := c.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("cid"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("link"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("space"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("link"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("space"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("space"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("link"),
				KeyType:       types.KeyTypeRange,
			},
		},
	})
	require.NoError(t, err)
}

func createBlobRegistryTable(t *testing.T, c *dynamodb.Client, tableName string) {
	_, err := c.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("digest"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("digest"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("space"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("digest"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("space"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("space"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("digest"),
				KeyType:       types.KeyTypeRange,
			},
		},
	})
	require.NoError(t, err)
}
