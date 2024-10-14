package contentclaims

import (
	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/capability/assert"
	"github.com/storacha/indexing-service/pkg/types"
)

var log = logging.Logger("contentclaims")

func NewService(indexer types.Service) map[ucan.Ability]server.ServiceMethod[assert.Unit] {
	return map[ucan.Ability]server.ServiceMethod[assert.Unit]{
		assert.Equals.Can(): server.Provide(
			assert.Equals,
			func(cap ucan.Capability[assert.EqualsCaveats], inv invocation.Invocation, ctx server.InvocationContext) (assert.Unit, receipt.Effects, error) {
				log.Errorf("TODO: implement me")
				return assert.Unit{}, nil, nil
			},
		),
		assert.Index.Can(): server.Provide(
			assert.Index,
			func(cap ucan.Capability[assert.IndexCaveats], inv invocation.Invocation, ctx server.InvocationContext) (assert.Unit, receipt.Effects, error) {
				log.Errorf("TODO: implement me")
				return assert.Unit{}, nil, nil
			},
		),
		assert.Location.Can(): server.Provide(
			assert.Location,
			func(cap ucan.Capability[assert.LocationCaveats], inv invocation.Invocation, ctx server.InvocationContext) (assert.Unit, receipt.Effects, error) {
				log.Errorf("TODO: implement me")
				return assert.Unit{}, nil, nil
			},
		),
	}
}
