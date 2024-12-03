package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/types"
)

// DynamoContentToClaimsMapper uses a DynamoDB table to map content hashes to the corresponding claims
type DynamoContentToClaimsMapper struct {
	c         dynamodb.QueryAPIClient
	tableName string
}

func NewDynamoContentToClaimsMapper(queryClient dynamodb.QueryAPIClient, tableName string) DynamoContentToClaimsMapper {
	return DynamoContentToClaimsMapper{
		c:         queryClient,
		tableName: tableName,
	}
}

type contentClaimItem struct {
	Content    string        `dynamodbav:"content"`
	Claim      string        `dynamodbav:"claim"`
	Expiration time.Duration `dynamodbav:"expiration"`
}

// GetClaim returns claim CIDs for a given content hash. Implements ContentToClaimMapper
func (dm DynamoContentToClaimsMapper) GetClaims(ctx context.Context, contentHash multihash.Multihash) ([]cid.Cid, error) {
	hash, err := attributevalue.Marshal(contentHash.B58String())
	if err != nil {
		return nil, err
	}

	keyEx := expression.Key("content").Equal(expression.Value(hash))
	proj := expression.NamesList(expression.Name("claim"))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).WithProjection(proj).Build()
	if err != nil {
		return nil, err
	}

	claimsCids := []cid.Cid{}

	queryPaginator := dynamodb.NewQueryPaginator(dm.c, &dynamodb.QueryInput{
		TableName:                 aws.String(dm.tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      expr.Projection(),
	})

	for queryPaginator.HasMorePages() {
		response, err := queryPaginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		contentClaimItems := []contentClaimItem{}
		err = attributevalue.UnmarshalListOfMaps(response.Items, &contentClaimItems)
		if err != nil {
			return nil, fmt.Errorf("deserializing items: %w", err)
		}

		for _, contentClaimItem := range contentClaimItems {
			claimCid, err := cid.Parse(contentClaimItem.Claim)
			if err != nil {
				return nil, fmt.Errorf("parsing claim CID: %w", err)
			}

			claimsCids = append(claimsCids, claimCid)
		}
	}

	if len(claimsCids) == 0 {
		return nil, types.ErrKeyNotFound
	}

	return claimsCids, nil
}
