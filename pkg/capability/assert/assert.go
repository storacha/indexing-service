package assert

import (
	"net/url"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/fluent/qp"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/core/result/failure"
	"github.com/storacha/go-ucanto/core/schema"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/validator"
	adm "github.com/storacha/indexing-service/pkg/capability/assert/datamodel"
)

// export const assert = capability({
//   can: 'assert/*',
//   with: URI.match({ protocol: 'did:' })
// })

type HasMultihash interface {
	hasMultihash()
	ToIPLD() (datamodel.Node, error)
	Hash() mh.Multihash
}

type link struct {
	link datamodel.Link
}

func (l link) hasMultihash() {}

func (l link) Hash() mh.Multihash {
	return l.link.(cidlink.Link).Cid.Hash()
}

func (l link) ToIPLD() (datamodel.Node, error) {
	return basicnode.NewLink(l.link), nil
}

func Link(l datamodel.Link) (HasMultihash, failure.Failure) {
	return link{l}, nil
}

type digest mh.Multihash

func (d digest) hasMultihash() {}

func (d digest) Hash() mh.Multihash {
	return mh.Multihash(d)
}

func (d digest) ToIPLD() (datamodel.Node, error) {
	return qp.BuildMap(basicnode.Prototype.Map, 1, func(ma datamodel.MapAssembler) {
		qp.MapEntry(ma, "digest", qp.Bytes(d))
	})
}

func Digest(d adm.DigestModel) (HasMultihash, failure.Failure) {
	return digest(d.Digest), nil
}

func FromHash(mh mh.Multihash) HasMultihash {
	return digest(mh)
}

var linkOrDigest = schema.Or(schema.Mapped(schema.Link(), Link), schema.Mapped(schema.Struct[adm.DigestModel](adm.DigestType(), nil), Digest))

type LocationCaveats struct {
	Content  HasMultihash
	Location []url.URL
	Range    *adm.Range
	Space    did.DID
}

func (lc LocationCaveats) ToIPLD() (datamodel.Node, error) {
	cn, err := lc.Content.ToIPLD()
	if err != nil {
		return nil, err
	}

	asStrings := make([]string, 0, len(lc.Location))
	for _, location := range lc.Location {
		asStrings = append(asStrings, location.String())
	}

	md := &adm.LocationCaveatsModel{
		Content:  cn,
		Location: asStrings,
		Range:    lc.Range,
		Space:    lc.Space.Bytes(),
	}
	return ipld.WrapWithRecovery(md, adm.LocationCaveatsType())
}

const LocationAbility = "assert/location"

var Location = validator.NewCapability(LocationAbility, schema.DIDString(),
	schema.Mapped(schema.Struct[adm.LocationCaveatsModel](adm.LocationCaveatsType(), nil), func(model adm.LocationCaveatsModel) (LocationCaveats, failure.Failure) {
		hasMultihash, err := linkOrDigest.Read(model.Content)
		if err != nil {
			return LocationCaveats{}, err
		}
		location := make([]url.URL, 0, len(model.Location))
		for _, l := range model.Location {
			url, err := schema.URI().Read(l)
			if err != nil {
				return LocationCaveats{}, err
			}
			location = append(location, url)
		}

		space := did.Undef
		if len(model.Space) > 0 {
			var serr error
			space, serr = did.Decode(model.Space)
			if serr != nil {
				return LocationCaveats{}, failure.FromError(serr)
			}
		}
		return LocationCaveats{
			Content:  hasMultihash,
			Location: location,
			Range:    model.Range,
			Space:    space,
		}, nil
	}), nil)

/**
 * Claims that a CID includes the contents claimed in another CID.
 */

type InclusionCaveats struct {
	Content  HasMultihash
	Includes ipld.Link
	Proof    *ipld.Link
}

func (ic InclusionCaveats) ToIPLD() (datamodel.Node, error) {
	cn, err := ic.Content.ToIPLD()
	if err != nil {
		return nil, err
	}

	md := &adm.InclusionCaveatsModel{
		Content:  cn,
		Includes: ic.Includes,
		Proof:    ic.Proof,
	}
	return ipld.WrapWithRecovery(md, adm.InclusionCaveatsType())
}

const InclusionAbility = "assert/inclusion"

var Inclusion = validator.NewCapability(InclusionAbility, schema.DIDString(),
	schema.Mapped(schema.Struct[adm.InclusionCaveatsModel](adm.InclusionCaveatsType(), nil), func(model adm.InclusionCaveatsModel) (InclusionCaveats, failure.Failure) {
		hasMultihash, err := linkOrDigest.Read(model.Content)
		if err != nil {
			return InclusionCaveats{}, err
		}
		includes, err := schema.Link(schema.WithVersion(1)).Read(model.Includes)
		if err != nil {
			return InclusionCaveats{}, err
		}
		proof := model.Proof
		if proof != nil {
			output, err := schema.Link(schema.WithVersion(1)).Read(*model.Proof)
			if err != nil {
				return InclusionCaveats{}, err
			}
			proof = &output
		}
		return InclusionCaveats{
			Content:  hasMultihash,
			Includes: includes,
			Proof:    proof}, nil
	}), nil)

/**
 * Claims that a content graph can be found in blob(s) that are identified and
 * indexed in the given index CID.
 */

type IndexCaveats struct {
	Content ipld.Link
	Index   ipld.Link
}

func (ic IndexCaveats) ToIPLD() (datamodel.Node, error) {

	md := &adm.IndexCaveatsModel{
		Content: ic.Content,
		Index:   ic.Index,
	}
	return ipld.WrapWithRecovery(md, adm.IndexCaveatsType())
}

const IndexAbility = "assert/index"

var IndexCaveatsReader = schema.Mapped(schema.Struct[adm.IndexCaveatsModel](adm.IndexCaveatsType(), nil), func(model adm.IndexCaveatsModel) (IndexCaveats, failure.Failure) {
	content, err := schema.Link().Read(model.Content)
	if err != nil {
		return IndexCaveats{}, err
	}
	index, err := schema.Link(schema.WithVersion(1)).Read(model.Index)
	if err != nil {
		return IndexCaveats{}, err
	}
	return IndexCaveats{content, index}, nil
})

var Index = validator.NewCapability(IndexAbility, schema.DIDString(), IndexCaveatsReader, nil)

/**
 * Claims that a CID's graph can be read from the blocks found in parts.
 */

type PartitionCaveats struct {
	Content HasMultihash
	Blocks  *ipld.Link
	Parts   []ipld.Link
}

func (pc PartitionCaveats) ToIPLD() (datamodel.Node, error) {
	cn, err := pc.Content.ToIPLD()
	if err != nil {
		return nil, err
	}

	md := &adm.PartitionCaveatsModel{
		Content: cn,
		Blocks:  pc.Blocks,
		Parts:   pc.Parts,
	}
	return ipld.WrapWithRecovery(md, adm.PartitionCaveatsType())
}

const PartitionAbility = "assert/partition"

var Partition = validator.NewCapability(
	PartitionAbility,
	schema.DIDString(),
	schema.Mapped(schema.Struct[adm.PartitionCaveatsModel](adm.PartitionCaveatsType(), nil), func(model adm.PartitionCaveatsModel) (PartitionCaveats, failure.Failure) {
		hasMultihash, err := linkOrDigest.Read(model.Content)
		if err != nil {
			return PartitionCaveats{}, err
		}

		blocks := model.Blocks
		if blocks != nil {
			output, err := schema.Link(schema.WithVersion(1)).Read(*model.Blocks)
			if err != nil {
				return PartitionCaveats{}, err
			}
			blocks = &output
		}
		parts := make([]ipld.Link, 0, len(model.Parts))
		for _, p := range model.Parts {
			part, err := schema.Link(schema.WithVersion(1)).Read(p)
			if err != nil {
				return PartitionCaveats{}, err
			}
			parts = append(parts, part)
		}
		return PartitionCaveats{
			Content: hasMultihash,
			Blocks:  blocks,
			Parts:   parts}, nil
	}), nil)

/**
 * Claims that a CID links to other CIDs.
 */

type RelationPartInclusion struct {
	Content ipld.Link
	Parts   *[]ipld.Link
}

type RelationPart struct {
	Content  ipld.Link
	Includes *RelationPartInclusion
}

type RelationCaveats struct {
	Content  HasMultihash
	Children []ipld.Link
	Parts    []RelationPart
}

func (rc RelationCaveats) ToIPLD() (datamodel.Node, error) {
	cn, err := rc.Content.ToIPLD()
	if err != nil {
		return nil, err
	}

	parts := make([]adm.RelationPartModel, 0, len(rc.Parts))
	for _, part := range rc.Parts {
		var includes *adm.RelationPartInclusionModel
		if part.Includes != nil {
			includes = &adm.RelationPartInclusionModel{
				Content: part.Includes.Content,
				Parts:   part.Includes.Parts,
			}
		}
		parts = append(parts, adm.RelationPartModel{
			Content:  part.Content,
			Includes: includes,
		})
	}
	md := &adm.RelationCaveatsModel{
		Content:  cn,
		Children: rc.Children,
		Parts:    parts,
	}
	return ipld.WrapWithRecovery(md, adm.RelationCaveatsType())
}

const RelationAbility = "assert/relation"

var Relation = validator.NewCapability(
	RelationAbility,
	schema.DIDString(),
	schema.Mapped(schema.Struct[adm.RelationCaveatsModel](adm.RelationCaveatsType(), nil), func(model adm.RelationCaveatsModel) (RelationCaveats, failure.Failure) {
		hasMultihash, err := linkOrDigest.Read(model.Content)
		if err != nil {
			return RelationCaveats{}, err
		}
		parts := make([]RelationPart, 0, len(model.Parts))
		for _, part := range model.Parts {
			var includes *RelationPartInclusion
			if part.Includes != nil {
				content, err := schema.Link(schema.WithVersion(1)).Read(part.Content)
				if err != nil {
					return RelationCaveats{}, err
				}
				var parts *[]ipld.Link
				if part.Includes.Parts != nil {
					*parts = make([]datamodel.Link, 0, len(*part.Includes.Parts))
					for _, p := range *part.Includes.Parts {
						part, err := schema.Link(schema.WithVersion(1)).Read(p)
						if err != nil {
							return RelationCaveats{}, err
						}
						*parts = append(*parts, part)
					}
				}
				includes = &RelationPartInclusion{
					Content: content,
					Parts:   parts,
				}
			}
			content, err := schema.Link(schema.WithVersion(1)).Read(part.Content)
			if err != nil {
				return RelationCaveats{}, err
			}
			parts = append(parts, RelationPart{
				Content:  content,
				Includes: includes,
			})
		}
		return RelationCaveats{
			Content:  hasMultihash,
			Children: model.Children,
			Parts:    parts,
		}, nil
	}),
	nil,
)

/**
 * Claim data is referred to by another CID and/or multihash. e.g CAR CID & CommP CID
 */

type EqualsCaveats struct {
	Content HasMultihash
	Equals  ipld.Link
}

func (ec EqualsCaveats) ToIPLD() (datamodel.Node, error) {
	content, err := ec.Content.ToIPLD()
	if err != nil {
		return nil, err
	}

	md := &adm.EqualsCaveatsModel{
		Content: content,
		Equals:  ec.Equals,
	}
	return ipld.WrapWithRecovery(md, adm.EqualsCaveatsType())
}

const EqualsAbility = "assert/equals"

var Equals = validator.NewCapability(
	EqualsAbility,
	schema.DIDString(),
	schema.Mapped(schema.Struct[adm.EqualsCaveatsModel](adm.EqualsCaveatsType(), nil), func(model adm.EqualsCaveatsModel) (EqualsCaveats, failure.Failure) {
		hasMultihash, err := linkOrDigest.Read(model.Content)
		if err != nil {
			return EqualsCaveats{}, err
		}
		return EqualsCaveats{
			Content: hasMultihash,
			Equals:  model.Equals,
		}, nil
	}),
	nil,
)

// Unit is a success type that can be used when there is no data to return from
// a capability handler.
type Unit struct{}

func (u Unit) ToIPLD() (datamodel.Node, error) {
	np := basicnode.Prototype.Any
	nb := np.NewBuilder()
	ma, err := nb.BeginMap(0)
	if err != nil {
		return nil, err
	}
	ma.Finish()
	return nb.Build(), nil
}
