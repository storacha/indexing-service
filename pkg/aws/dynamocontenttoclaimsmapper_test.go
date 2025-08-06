package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ipfs/go-cid"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/indexing-service/pkg/internal/extmocks"
	"github.com/storacha/indexing-service/pkg/internal/link"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetClaims(t *testing.T) {
	testTable := "sometable"

	t.Run("happy path", func(t *testing.T) {
		mockDynamoDBClient := extmocks.NewMockDynamoDBQueryClient(t)
		dynamoDBMapper := NewDynamoContentToClaimsMapper(mockDynamoDBClient, testTable)

		contentHash := testutil.RandomMultihash(t)
		locationClaimCID := link.ToCID(testutil.RandomCID(t))
		indexClaimCID := link.ToCID(testutil.RandomCID(t))

		locationClaim, err := attributevalue.MarshalMap(contentClaimItem{
			Content: contentHash.String(),
			Claim:   locationClaimCID.String(),
		})
		require.NoError(t, err)

		indexClaim, err := attributevalue.MarshalMap(contentClaimItem{
			Content: contentHash.String(),
			Claim:   indexClaimCID.String(),
		})
		require.NoError(t, err)

		ctx := context.Background()

		mockDynamoDBClient.On("Query", ctx, mock.Anything, mock.Anything).Return(&dynamodb.QueryOutput{
			Count: 2,
			Items: []map[string]dbtypes.AttributeValue{locationClaim, indexClaim},
		}, nil)

		cids, err := dynamoDBMapper.GetClaims(ctx, contentHash)

		require.NoError(t, err)
		require.Equal(t, []cid.Cid{locationClaimCID, indexClaimCID}, cids)

		mockDynamoDBClient.AssertExpectations(t)
	})

	t.Run("returns ErrKeyNotFound when there are no results in the DB", func(t *testing.T) {
		mockDynamoDBClient := extmocks.NewMockDynamoDBQueryClient(t)
		dynamoDBMapper := NewDynamoContentToClaimsMapper(mockDynamoDBClient, testTable)

		ctx := context.Background()

		mockDynamoDBClient.On("Query", ctx, mock.Anything, mock.Anything).Return(&dynamodb.QueryOutput{
			Count: 0,
		}, nil)

		_, err := dynamoDBMapper.GetClaims(ctx, testutil.RandomMultihash(t))

		require.Equal(t, types.ErrKeyNotFound, err)

		mockDynamoDBClient.AssertExpectations(t)
	})
}
