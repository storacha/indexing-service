package contentclaims

import (
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/indexing-service/pkg/types"
)

func NewUCANServer(id principal.Signer, service types.Publisher) (server.ServerView, error) {
	ucanService := NewUCANService(service)
	var opts []server.Option
	for ability, method := range ucanService {
		opts = append(opts, server.WithServiceMethod(ability, method))
	}
	return server.NewServer(id, opts...)
}
