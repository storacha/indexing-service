package datamodel

import (
	// for schema import
	_ "embed"
	"fmt"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/schema"
)

var (
	//go:embed queryresult.ipldsch
	queryResultBytes []byte
	queryResultType  schema.Type
)

func init() {
	typeSystem, err := ipld.LoadSchemaBytes(queryResultBytes)
	if err != nil {
		panic(fmt.Errorf("failed to load schema: %w", err))
	}
	queryResultType = typeSystem.TypeByName("QueryResult")
}

// QueryResultType is the schema for a QueryResult
func QueryResultType() schema.Type {
	return queryResultType
}

// QueryResultModel is the golang structure for encoding query results
type QueryResultModel struct {
	Result0_1 *QueryResultModel0_1
	Result0_2 *QueryResultModel0_2
}

// QueryResultModel0_1 describes the found claims and indexes for a given query
type QueryResultModel0_1 struct {
	Claims  []ipld.Link
	Indexes *IndexesModel
}

// QueryResultModel0_2 describes the found claims and indexes for a given query
type QueryResultModel0_2 struct {
	Claims   []ipld.Link
	Indexes  *IndexesModel
	Messages []string
}

// IndexesModel maps encoded context IDs to index links
type IndexesModel struct {
	Keys   []string
	Values map[string]ipld.Link
}
