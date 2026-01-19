package aws

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/storacha/go-libstoracha/digestutil"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/stretchr/testify/require"
)

func TestDynamoAllocationsTable(t *testing.T) {
	if os.Getenv("CI") != "" && runtime.GOOS != "linux" {
		t.SkipNow()
	}

	ctx := context.Background()
	endpoint := createDynamo(t)
	dynamoClient := newDynamoClient(t, endpoint)

	tableName := "prod-w3infra-allocations"
	createAllocationsTable(t, dynamoClient, tableName)

	t.Run("query existing item", func(t *testing.T) {
		digest := testutil.RandomMultihash(t)
		insertedAt := time.Now().String()
		space := testutil.RandomPrincipal(t).DID().String()
		invocation := testutil.RandomCID(t).String()
		size := "123"
		_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item: map[string]types.AttributeValue{
				"multihash":  &types.AttributeValueMemberS{Value: digestutil.Format(digest)},
				"space":      &types.AttributeValueMemberS{Value: space},
				"size":       &types.AttributeValueMemberN{Value: size},
				"invocation": &types.AttributeValueMemberS{Value: invocation},
				"insertedAt": &types.AttributeValueMemberS{Value: insertedAt},
			},
		})
		require.NoError(t, err)

		store := NewDynamoAllocationsTable(dynamoClient, tableName)

		has, err := store.Has(ctx, digest)
		require.NoError(t, err)
		require.True(t, has)
	})

	t.Run("query multiple existing items for same digest", func(t *testing.T) {
		root := testutil.RandomCID(t)
		digest := link.ToCID(root).Hash()

		items := []struct {
			space      string
			size       string
			invocation string
			insertedAt string
		}{
			{
				space:      testutil.RandomPrincipal(t).DID().String(),
				size:       fmt.Sprintf("%d", rand.IntN(1024*1024*128)),
				invocation: testutil.RandomCID(t).String(),
				insertedAt: time.Now().AddDate(-1, 0, 0).String(),
			},
			{
				space:      testutil.RandomPrincipal(t).DID().String(),
				size:       fmt.Sprintf("%d", rand.IntN(1024*1024*128)),
				invocation: testutil.RandomCID(t).String(),
				insertedAt: time.Now().String(),
			},
		}

		for _, i := range items {
			_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: aws.String(tableName),
				Item: map[string]types.AttributeValue{
					"multihash":  &types.AttributeValueMemberS{Value: digestutil.Format(digest)},
					"space":      &types.AttributeValueMemberS{Value: i.space},
					"size":       &types.AttributeValueMemberN{Value: i.size},
					"invocation": &types.AttributeValueMemberS{Value: i.invocation},
					"insertedAt": &types.AttributeValueMemberS{Value: i.insertedAt},
				},
			})
			require.NoError(t, err)
		}

		store := NewDynamoAllocationsTable(dynamoClient, tableName)

		has, err := store.Has(ctx, digest)
		require.NoError(t, err)
		require.True(t, has)
	})

	t.Run("query not found", func(t *testing.T) {
		digest := testutil.RandomMultihash(t)
		store := NewDynamoAllocationsTable(dynamoClient, tableName)
		has, err := store.Has(ctx, digest)
		require.NoError(t, err)
		require.False(t, has)
	})
}

func createAllocationsTable(t *testing.T, c *dynamodb.Client, tableName string) {
	_, err := c.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName:   aws.String(tableName),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("space"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("multihash"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("insertedAt"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("space"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("multihash"),
				KeyType:       types.KeyTypeRange,
			},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("multihash"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("multihash"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("space"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					NonKeyAttributes: []string{"space", "insertedAt"},
					ProjectionType:   types.ProjectionTypeInclude,
				},
			},
			{
				IndexName: aws.String("insertedAt"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("insertedAt"),
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
	})
	require.NoError(t, err)
}
