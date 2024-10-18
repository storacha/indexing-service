package contentclaims

import (
	"context"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/invocation"
	"github.com/storacha/go-ucanto/core/receipt"
	"github.com/storacha/go-ucanto/principal/ed25519/verifier"
	"github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/capability/assert"
	"github.com/storacha/indexing-service/pkg/capability/claim"
	"github.com/storacha/indexing-service/pkg/types"
)

var log = logging.Logger("contentclaims")

func NewService(indexer types.Service) map[ucan.Ability]server.ServiceMethod[assert.Unit] {
	return map[ucan.Ability]server.ServiceMethod[assert.Unit]{
		assert.EqualsAbility: server.Provide(
			assert.Equals,
			func(cap ucan.Capability[assert.EqualsCaveats], inv invocation.Invocation, ctx server.InvocationContext) (assert.Unit, receipt.Effects, error) {
				err := indexer.PublishClaim(context.TODO(), inv)
				return assert.Unit{}, nil, err
			},
		),
		assert.IndexAbility: server.Provide(
			assert.Index,
			func(cap ucan.Capability[assert.IndexCaveats], inv invocation.Invocation, ctx server.InvocationContext) (assert.Unit, receipt.Effects, error) {
				err := indexer.PublishClaim(context.TODO(), inv)
				if err != nil {
					log.Errorf("publishing index claim: %w", err)
				}
				return assert.Unit{}, nil, err
			},
		),
		claim.CacheAbility: server.Provide(
			claim.Cache,
			func(cap ucan.Capability[claim.CacheCaveats], inv invocation.Invocation, ctx server.InvocationContext) (assert.Unit, receipt.Effects, error) {
				peerid, err := toPeerID(inv.Issuer())
				if err != nil {
					return assert.Unit{}, nil, err
				}

				provider := peer.AddrInfo{ID: peerid, Addrs: cap.Nb().Provider.Addresses}

				bs, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(inv.Blocks()))
				if err != nil {
					return assert.Unit{}, nil, err
				}

				rootbl, ok, err := bs.Get(cap.Nb().Claim)
				if err != nil {
					return assert.Unit{}, nil, err
				}
				if !ok {
					return assert.Unit{}, nil, NewMissingClaimError()
				}

				claim := delegation.NewDelegation(rootbl, bs)
				err = indexer.CacheClaim(context.TODO(), provider, claim)
				return assert.Unit{}, nil, err
			},
		),
	}
}

func toPeerID(principal ucan.Principal) (peer.ID, error) {
	vfr, err := verifier.Decode(principal.DID().Bytes())
	if err != nil {
		return "", err
	}
	pub, err := crypto.UnmarshalEd25519PublicKey(vfr.Raw())
	if err != nil {
		return "", err
	}
	return peer.IDFromPublicKey(pub)
}
