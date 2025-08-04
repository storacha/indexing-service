package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/ipld/go-ipld-prime"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
)

type DynamoMigratedShardChecker struct {
	storeTableClient        dynamodb.QueryAPIClient
	blobRegistryTableClient dynamodb.QueryAPIClient
	blobRegistryTableName   string
	storeTableName          string
	allocationsStore        AllocationsStore
}

func (d *DynamoMigratedShardChecker) storeTableShardMigrated(ctx context.Context, shard ipld.Link) (bool, error) {

	keyEx := expression.Key("link").Equal(expression.Value(shard.String()))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return false, err
	}

	o, err := d.storeTableClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.storeTableName),
		IndexName:                 aws.String("cid"),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      expr.Projection(),
		Select:                    dynamotypes.SelectCount,
	})
	if err != nil {
		return false, fmt.Errorf("querying store table: %w", err)
	}
	return o.Count > 0, nil
}

func (d *DynamoMigratedShardChecker) blobRegistryTableShardMigrated(ctx context.Context, shard ipld.Link) (bool, error) {
	cl, ok := shard.(cidlink.Link)
	if !ok {
		return false, fmt.Errorf("shard is not a CID link: %T", shard)
	}

	keyEx := expression.Key("digest").Equal(expression.Value(cl.Cid.Hash().B58String()))
	expr, err := expression.NewBuilder().WithKeyCondition(keyEx).Build()
	if err != nil {
		return false, err
	}

	o, err := d.blobRegistryTableClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.blobRegistryTableName),
		IndexName:                 aws.String("digest"),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		KeyConditionExpression:    expr.KeyCondition(),
		ProjectionExpression:      expr.Projection(),
		Select:                    dynamotypes.SelectCount,
	})
	if err != nil {
		return false, fmt.Errorf("querying store table: %w", err)
	}
	return o.Count > 0, nil
}

func (d *DynamoMigratedShardChecker) ShardMigrated(ctx context.Context, shard ipld.Link) (bool, error) {
	if migrated, err := d.storeTableShardMigrated(ctx, shard); err != nil {
		return false, err
	} else if migrated {
		return true, nil
	}
	if migrated, err := d.blobRegistryTableShardMigrated(ctx, shard); err != nil {
		return false, err
	} else if migrated {
		return true, nil
	}
	return d.allocationsStore.Has(ctx, shard.(cidlink.Link).Cid.Hash())
}

func NewDynamoMigratedShardChecker(storeTableClient dynamodb.QueryAPIClient, blobRegistryTableClient dynamodb.QueryAPIClient, blobRegistryTableName, storeTableName string, allocationsStore AllocationsStore) *DynamoMigratedShardChecker {
	return &DynamoMigratedShardChecker{
		storeTableClient:        storeTableClient,
		blobRegistryTableClient: blobRegistryTableClient,
		blobRegistryTableName:   blobRegistryTableName,
		storeTableName:          storeTableName,
		allocationsStore:        allocationsStore,
	}
}
