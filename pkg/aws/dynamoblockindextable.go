package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	multihash "github.com/multiformats/go-multihash"
	"github.com/storacha/indexing-service/pkg/internal/digestutil"
	"github.com/storacha/indexing-service/pkg/service/legacy"
	"github.com/storacha/indexing-service/pkg/types"
)

type DynamoProviderBlockIndexTable struct {
	client    dynamodb.QueryAPIClient
	tableName string
}

type blockIndexItem struct {
	CarPath string `dynamodbav:"carpath"`
	Offset  uint64 `dynamodbav:"offset"`
	Length  uint64 `dynamodbav:"length"`
}

func (d *DynamoProviderBlockIndexTable) Query(ctx context.Context, digest multihash.Multihash) ([]legacy.BlockIndexRecord, error) {
	digestAttr, err := attributevalue.Marshal(digestutil.Format(digest))
	if err != nil {
		return nil, err
	}

	keyEx := expression.Key("blockmultihash").Equal(expression.Value(digestAttr))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, err
	}

	records := []legacy.BlockIndexRecord{}

	queryPaginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
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

		items := []blockIndexItem{}
		err = attributevalue.UnmarshalListOfMaps(response.Items, &items)
		if err != nil {
			return nil, fmt.Errorf("deserializing items: %w", err)
		}

		for _, item := range items {
			records = append(records, legacy.BlockIndexRecord(item))
		}
	}

	if len(records) == 0 {
		return nil, types.ErrKeyNotFound
	}

	return records, nil
}

var _ legacy.BlockIndexStore = (*DynamoProviderBlockIndexTable)(nil)

func NewDynamoProviderBlockIndexTable(client dynamodb.QueryAPIClient, tableName string) *DynamoProviderBlockIndexTable {
	return &DynamoProviderBlockIndexTable{client, tableName}
}
