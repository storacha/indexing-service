package aws

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/internal/testutil"
	"github.com/storacha/indexing-service/pkg/service/legacy"
	istypes "github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestDynamoProviderBlockIndexTable(t *testing.T) {
	ctx := context.Background()
	endpoint := createDynamo(t)
	dynamoClient := newDynamoClient(t, endpoint)

	tableName := "blocks-cars-position-" + uuid.NewString()
	createTable(t, dynamoClient, tableName)

	t.Run("query existing item", func(t *testing.T) {
		digest := testutil.RandomMultihash()
		path := fmt.Sprintf("http://test.example.com/%s.blob", digestutil.Format(digest))
		offset := rand.IntN(1024 * 1024 * 128)
		length := rand.IntN(1024*1024*2) + 1

		_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item: map[string]types.AttributeValue{
				"blockmultihash": &types.AttributeValueMemberS{Value: digestutil.Format(digest)},
				"carpath":        &types.AttributeValueMemberS{Value: path},
				"offset":         &types.AttributeValueMemberN{Value: fmt.Sprint(offset)},
				"length":         &types.AttributeValueMemberN{Value: fmt.Sprint(length)},
			},
		})
		require.NoError(t, err)

		store := NewDynamoProviderBlockIndexTable(dynamoClient, tableName)

		results, err := store.Query(ctx, digest)
		require.NoError(t, err)
		require.Equal(t, 1, len(results))
		require.Equal(t, path, results[0].CarPath)
		require.Equal(t, uint64(offset), results[0].Offset)
		require.Equal(t, uint64(length), results[0].Length)
	})

	t.Run("query multiple existing items for same digest", func(t *testing.T) {
		root := testutil.RandomCID()
		digest := link.ToCID(root).Hash()

		items := []struct {
			path   string
			offset int
			length int
		}{
			{
				path:   fmt.Sprintf("http://test.example.com/%s.blob", digestutil.Format(digest)),
				offset: rand.IntN(1024 * 1024 * 128),
				length: rand.IntN(1024*1024*2) + 1,
			},
			{
				path:   fmt.Sprintf("http://test.example.com/%s.car", root.String()),
				offset: rand.IntN(1024 * 1024 * 128),
				length: rand.IntN(1024*1024*2) + 1,
			},
		}

		for _, i := range items {
			_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
				TableName: aws.String(tableName),
				Item: map[string]types.AttributeValue{
					"blockmultihash": &types.AttributeValueMemberS{Value: digestutil.Format(digest)},
					"carpath":        &types.AttributeValueMemberS{Value: i.path},
					"offset":         &types.AttributeValueMemberN{Value: fmt.Sprint(i.offset)},
					"length":         &types.AttributeValueMemberN{Value: fmt.Sprint(i.length)},
				},
			})
			require.NoError(t, err)
		}

		store := NewDynamoProviderBlockIndexTable(dynamoClient, tableName)

		results, err := store.Query(ctx, digest)
		require.NoError(t, err)
		require.Equal(t, len(items), len(results))

		for _, i := range items {
			require.True(t, slices.ContainsFunc(results, func(r legacy.BlockIndexRecord) bool {
				return r.CarPath == i.path && r.Offset == uint64(i.offset) && r.Length == uint64(i.length)
			}))
		}
	})

	t.Run("query not found", func(t *testing.T) {
		digest := testutil.RandomMultihash()
		store := NewDynamoProviderBlockIndexTable(dynamoClient, tableName)
		_, err := store.Query(ctx, digest)
		require.Error(t, err)
		require.True(t, errors.Is(err, istypes.ErrKeyNotFound))
	})
}

func createDynamo(t *testing.T) url.URL {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "amazon/dynamodb-local:latest",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForExposedPort(),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	testcontainers.CleanupContainer(t, container)
	require.NoError(t, err)

	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	return *testutil.Must(url.Parse("http://" + endpoint))(t)
}

func newDynamoClient(t *testing.T, endpoint url.URL) *dynamodb.Client {
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "DUMMYIDEXAMPLE",
				SecretAccessKey: "DUMMYEXAMPLEKEY",
			},
		}),
		func(o *config.LoadOptions) error {
			o.Region = "us-east-1"
			return nil
		},
	)

	require.NoError(t, err)
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		base := endpoint.String()
		o.BaseEndpoint = &base
	})
}

func createTable(t *testing.T, c *dynamodb.Client, tableName string) {
	_, err := c.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName:   aws.String(tableName),
		BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("blockmultihash"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("carpath"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("blockmultihash"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("carpath"),
				KeyType:       types.KeyTypeRange,
			},
		},
	})
	require.NoError(t, err)
}
