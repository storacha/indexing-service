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
	"github.com/storacha/indexing-service/pkg/types"
)

// Some blocks are in MANY CARs. The blockIndexQueryLimit here is set to the same blockIndexQueryLimit that
// was previously applied in the legacy content claims service when fetching
// items from this same table.
// https://github.com/storacha/content-claims/blob/d837231389d60fa50c3d36b421bf3062cc7350ce/packages/infra/src/lib/store/block-index.js#L16C7-L16C12
const blockIndexQueryLimit = 25

type DynamoProviderBlockIndexTable struct {
	client    dynamodb.QueryAPIClient
	tableName string
}

type blockIndexItem struct {
	CarPath string `dynamodbav:"carpath"`
	Offset  uint64 `dynamodbav:"offset"`
	Length  uint64 `dynamodbav:"length"`
}

func (d *DynamoProviderBlockIndexTable) Query(ctx context.Context, digest multihash.Multihash) ([]BlockIndexRecord, error) {
	digestAttr, err := attributevalue.Marshal(digestutil.Format(digest))
	if err != nil {
		return nil, err
	}

	keyEx := expression.Key("blockmultihash").Equal(expression.Value(digestAttr))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return nil, err
	}

	records := []BlockIndexRecord{}

	queryPaginator := dynamodb.NewQueryPaginator(d.client, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      expr.Projection(),
		Limit:                     aws.Int32(blockIndexQueryLimit),
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
			records = append(records, BlockIndexRecord(item))
		}

		if len(records) >= blockIndexQueryLimit {
			break
		}
	}

	if len(records) == 0 {
		return nil, types.ErrKeyNotFound
	}

	return records, nil
}

var _ BlockIndexStore = (*DynamoProviderBlockIndexTable)(nil)

func NewDynamoProviderBlockIndexTable(client dynamodb.QueryAPIClient, tableName string) *DynamoProviderBlockIndexTable {
	return &DynamoProviderBlockIndexTable{client, tableName}
}
