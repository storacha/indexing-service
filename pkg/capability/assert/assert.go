package assert

import (
	"net/url"

	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/fluent/qp"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/core/ipld"
	"github.com/storacha-network/go-ucanto/core/result/failure"
	"github.com/storacha-network/go-ucanto/core/schema"
	"github.com/storacha-network/go-ucanto/validator"
	adm "github.com/storacha-network/indexing-service/pkg/capability/assert/datamodel"
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

type digest struct {
	Digest mh.Multihash
}

func (d digest) hasMultihash() {}

func (d digest) Hash() mh.Multihash {
	return d.Digest
}

func (d digest) ToIPLD() (datamodel.Node, error) {
	return qp.BuildMap(basicnode.Prototype.Map, 1, func(ma datamodel.MapAssembler) {
		qp.MapEntry(ma, "digest", qp.Bytes(d.Digest))
	})
}

func Digest(d adm.DigestModel) (HasMultihash, failure.Failure) {
	return digest{Digest: d.Digest}, nil
}

var linkOrDigest = schema.Or(schema.Mapped(schema.Link(), Link), schema.Mapped(schema.Struct[adm.DigestModel](adm.DigestType(), nil), Digest))

type LocationCaveats struct {
	Content  HasMultihash
	Location []url.URL
	Range    *adm.Range
}

func (lc LocationCaveats) Build() (datamodel.Node, error) {
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
		return LocationCaveats{
			Content:  hasMultihash,
			Location: location,
			Range:    model.Range,
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

// /**
//  * Claims that a content graph can be found in blob(s) that are identified and
//  * indexed in the given index CID.
//  */
// export const index = capability({
//   can: 'assert/index',
//   with: URI.match({ protocol: 'did:' }),
//   nb: Schema.struct({
//     /** DAG root CID */
//     content: Schema.link(),
//     /**
//      * Link to a Content Archive that contains the index.
//      * e.g. `index/sharded/dag@0.1`
//      * @see https://github.com/w3s-project/specs/blob/main/w3-index.md
//      */
//     index: Schema.link({ version: 1 })
//   })
// })

// /**
//  * Claims that a CID's graph can be read from the blocks found in parts.
//  */
// export const partition = capability({
//   can: 'assert/partition',
//   with: URI.match({ protocol: 'did:' }),
//   nb: Schema.struct({
//     /** Content root CID */
//     content: linkOrDigest(),
//     /** CIDs CID */
//     blocks: Schema.link({ version: 1 }).optional(),
//     parts: Schema.array(Schema.link({ version: 1 }))
//   })
// })

// /**
//  * Claims that a CID links to other CIDs.
//  */
// export const relation = capability({
//   can: 'assert/relation',
//   with: URI.match({ protocol: 'did:' }),
//   nb: Schema.struct({
//     content: linkOrDigest(),
//     /** CIDs this content links to directly. */
//     children: Schema.array(Schema.link()),
//     /** Parts this content and it's children can be read from. */
//     parts: Schema.array(Schema.struct({
//       content: Schema.link({ version: 1 }),
//       /** CID of contents (CARv2 index) included in this part. */
//       includes: Schema.struct({
//         content: Schema.link({ version: 1 }),
//         /** CIDs of parts this index may be found in. */
//         parts: Schema.array(Schema.link({ version: 1 })).optional()
//       }).optional()
//     }))
//   })
// })

// /**
//  * Claim data is referred to by another CID and/or multihash. e.g CAR CID & CommP CID
//  */
// export const equals = capability({
//   can: 'assert/equals',
//   with: URI.match({ protocol: 'did:' }),
//   nb: Schema.struct({
//     content: linkOrDigest(),
//     equals: Schema.link()
//   })
// })
