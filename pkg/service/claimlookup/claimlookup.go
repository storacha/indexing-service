package claimlookup

import (
	"github.com/ipni/go-libipni/find/model"
	"github.com/storacha-network/go-ucanto/core/delegation"
)

// ClaimLookup is used to get full claims from a claim cid
// I'll be honest, I'm not exactly sure whether these claims should be stored or simply synthesized
// from the information in IPNI combined with having private keys stored in this service
// I THINK it's possible you can synthesize Index & Equals claims from the information in IPNI + a private key
// Location commitments are more complicated cause they really ought to be signed by the storer of the commitment?
type ClaimLookup interface {
	LookupClaim(model.ProviderResult) (delegation.Delegation, error)
}
