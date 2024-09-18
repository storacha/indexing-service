package datamodel

import (
	"fmt"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/schema"
)

//go:embed assert.ipldsch
var assert []byte

var assertTS *schema.TypeSystem

func init() {
	ts, err := ipld.LoadSchemaBytes(assert)
	if err != nil {
		panic(fmt.Errorf("loading sharded dag index schema: %w", err))
	}
	assertTS = ts
}

func LocationCaveatsType() schema.Type {
	return assertTS.TypeByName("LocationCaveats")
}

func InclusionCaveatsType() schema.Type {
	return assertTS.TypeByName("InclusionCaveats")
}

func IndexCaveatsType() schema.Type {
	return assertTS.TypeByName("IndexCaveats")
}

func PartitionCaveatsType() schema.Type {
	return assertTS.TypeByName("PartitionCaveats")
}

func RelationCaveatsType() schema.Type {
	return assertTS.TypeByName("RelationCaveats")
}

func EqualsCaveatsType() schema.Type {
	return assertTS.TypeByName("EqualsCaveats")
}

func DigestType() schema.Type {
	return assertTS.TypeByName("Digest")
}

type Range struct {
	Offset uint64
	Length *uint64
}

type LocationCaveatsModel struct {
	Content  datamodel.Node
	Location []string
	Range    *Range
}

type DigestModel struct {
	Digest []byte
}

type InclusionCaveatsModel struct {
	Content  datamodel.Node
	Includes ipld.Link
	Proof    *ipld.Link
}

type IndexCaveatsModel struct {
	Content ipld.Link
	Index   ipld.Link
}

type PartitionCaveatsModel struct {
	Content datamodel.Node
	Blocks  *ipld.Link
	Parts   []ipld.Link
}

type RelationPartInclusionModel struct {
	Content ipld.Link
	Parts   *[]ipld.Link
}

type RelationPartModel struct {
	Content  ipld.Link
	Includes *RelationPartInclusionModel
}

type RelationCaveatsModel struct {
	Content  datamodel.Node
	Children []ipld.Link
	Parts    []RelationPartModel
}

type EqualsCaveatsModel struct {
	Content datamodel.Node
	Equals  ipld.Link
}
