package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/digestutil"
)

type DynamoAllocationsTable struct {
	client    dynamodb.QueryAPIClient
	tableName string
}

func (d *DynamoAllocationsTable) Has(ctx context.Context, digest multihash.Multihash) (bool, error) {
	digestAttr, err := attributevalue.Marshal(digestutil.Format(digest))
	if err != nil {
		return false, err
	}

	keyEx := expression.Key("multihash").Equal(expression.Value(digestAttr))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return false, err
	}

	result, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		IndexName:                 aws.String("multihash"),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		Select:                    types.SelectCount,
	})

	if err != nil {
		return false, err
	}

	return result.Count > 0, nil
}

func NewDynamoAllocationsTable(client dynamodb.QueryAPIClient, tableName string) *DynamoAllocationsTable {
	return &DynamoAllocationsTable{client, tableName}
}
