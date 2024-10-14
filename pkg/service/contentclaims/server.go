package contentclaims

import (
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/indexing-service/pkg/types"
)

func NewServer(id principal.Signer, indexer types.Service) (server.ServerView, error) {
	service := NewService(indexer)
	var opts []server.Option
	for ability, method := range service {
		opts = append(opts, server.WithServiceMethod(ability, method))
	}
	return server.NewServer(id, opts...)
}
