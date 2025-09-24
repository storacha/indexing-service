package contentclaims

import (
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/indexing-service/pkg/types"
)

func NewUCANServer(id principal.Signer, service types.Publisher, options ...server.Option) (server.ServerView[server.Service], error) {
	ucanService := NewUCANService(service)
	for ability, method := range ucanService {
		options = append(options, server.WithServiceMethod(ability, method))
	}
	return server.NewServer(id, options...)
}
