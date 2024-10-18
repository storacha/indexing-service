package claim

import (
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/multiformats/go-multiaddr"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/core/schema"
	"github.com/storacha/go-ucanto/validator"
	cdm "github.com/storacha/indexing-service/pkg/capability/claim/datamodel"
)

type CacheCaveats struct {
	Claim    ipld.Link
	Provider Provider
}

type Provider struct {
	Addresses []multiaddr.Multiaddr
}

func (cc CacheCaveats) ToIPLD() (datamodel.Node, error) {
	var addrs [][]byte
	for _, addr := range cc.Provider.Addresses {
		addrs = append(addrs, addr.Bytes())
	}

	model := cdm.CacheCaveatsModel{
		Claim:    cc.Claim,
		Provider: cdm.ProviderModel{Addresses: addrs},
	}
	return ipld.WrapWithRecovery(&model, cdm.CacheCaveatsType())
}

const CacheAbility = "claim/cache"

var CacheCaveatsReader = schema.Mapped(schema.Struct[cdm.CacheCaveatsModel](cdm.CacheCaveatsType(), nil), func(model cdm.CacheCaveatsModel) (CacheCaveats, failure.Failure) {
	provider := Provider{}
	for _, bytes := range model.Provider.Addresses {
		addr, err := multiaddr.NewMultiaddrBytes(bytes)
		if err != nil {
			return CacheCaveats{}, failure.FromError(err)
		}
		provider.Addresses = append(provider.Addresses, addr)
	}
	return CacheCaveats{model.Claim, provider}, nil
})

var Cache = validator.NewCapability(CacheAbility, schema.DIDString(), CacheCaveatsReader, nil)
