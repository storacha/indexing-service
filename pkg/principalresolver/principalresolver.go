package principalresolver

import (
	"errors"

	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/validator"
)

type resolver struct {
	mapping map[did.DID]did.DID
}

func (r *resolver) ResolveDIDKey(input did.DID) (did.DID, validator.UnresolvedDID) {
	dk, ok := r.mapping[input]
	if !ok {
		return did.Undef, validator.NewDIDKeyResolutionError(input, errors.New("not found in mapping"))
	}
	return dk, nil
}

func New(smap map[string]string) (validator.PrincipalResolver, error) {
	dmap := map[did.DID]did.DID{}
	for k, v := range smap {
		dk, err := did.Parse(k)
		if err != nil {
			return nil, err
		}
		dv, err := did.Parse(v)
		if err != nil {
			return nil, err
		}
		dmap[dk] = dv
	}
	return &resolver{dmap}, nil
}
