package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamoTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/ipni-publisher/pkg/store"
)

// DynamoContentToClaimMapper uses a DynamoDB table to map content hashes to the corresponding claims
type DynamoContentToClaimMapper struct {
	c         *dynamodb.Client
	tableName string
}

func NewDynamoContentToClaimMapper(awsCfg aws.Config, tableName string) DynamoContentToClaimMapper {
	return DynamoContentToClaimMapper{
		c:         dynamodb.NewFromConfig(awsCfg),
		tableName: tableName,
	}
}

type contentClaimItem struct {
	Content    string        `dynamodbav:"content"`
	Claim      string        `dynamodbav:"claim"`
	Expiration time.Duration `dynamodbav:"expiration"`
}

// GetClaim returns the claim CID for a given content hash. Implements ContentToClaimMapper
func (dm DynamoContentToClaimMapper) GetClaim(ctx context.Context, contentHash multihash.Multihash) (cid.Cid, error) {
	hash, err := attributevalue.Marshal(contentHash)
	if err != nil {
		return cid.Cid{}, err
	}

	key := map[string]dynamoTypes.AttributeValue{"content": hash}
	response, err := dm.c.GetItem(ctx, &dynamodb.GetItemInput{
		Key:                  key,
		TableName:            aws.String(dm.tableName),
		ProjectionExpression: aws.String("claim"),
	})
	if err != nil {
		return cid.Cid{}, fmt.Errorf("retrieving item: %w", err)
	}

	if response.Item == nil {
		return cid.Cid{}, store.NewErrNotFound(ErrDynamoRecordNotFound)
	}

	contentClaimItem := contentClaimItem{}
	err = attributevalue.UnmarshalMap(response.Item, &contentClaimItem)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("deserializing item: %w", err)
	}

	claimCid, err := cid.Parse(contentClaimItem.Claim)
	if err != nil {
		return cid.Cid{}, fmt.Errorf("parsing claim CID: %w", err)
	}

	return claimCid, nil
}
