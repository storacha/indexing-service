package datamodel

import (
	_ "embed"
	"fmt"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/schema"
)

//go:embed claim.ipldsch
var claimSchema []byte

var claimTypeSystem *schema.TypeSystem

func init() {
	ts, err := ipld.LoadSchemaBytes(claimSchema)
	if err != nil {
		panic(fmt.Errorf("loading claim schema: %w", err))
	}
	claimTypeSystem = ts
}

func CacheCaveatsType() schema.Type {
	return claimTypeSystem.TypeByName("CacheCaveats")
}

type CacheCaveatsModel struct {
	Claim    datamodel.Link
	Provider ProviderModel
}

type ProviderModel struct {
	Addresses [][]byte
}
