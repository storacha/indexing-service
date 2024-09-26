/*
	 Package metadata implements the metadata protocol for publishing content claims on IPNI

	  The goal of the content claims transport protocol is to provide a way to index content claims on IPNI

		The rules for publishing content claims records to IPNI are as follows:

		Content claims should be published to IPNI by the original issuer of the content claim.

		The ContextID for the content claim should be the cid of the content the claim is about,
		except in the case of a location commitment, where the content ID should be:
			hash(audience public key, content cid multihash)

		The claim record MUST be able to be looked up on IPNI from the content cid multihash (or double encryption thereof)

		The claim MAY be able to be looked up by additional multihashes, particularly in the case of the IndexClaim, where
		the record should be accessible from any multihash inside the index

		The metadata for the claim is structured to maximize utility of the record while minimizing size

		To generally respect the 100 byte maximum size for IPNI records, we encode the claim type as an integer, rather than as string

		The claim itself, being too large to fit in metadata, is referenced only by its CID. The full claim must be retrievable by
		combining the http multiaddr of the provider + the claim CID

		However, in order to enable faster chaining of requests and general processing, we add a shortcut bytes field,
		which encodes specific information from the full claim and is interpreted based on the claim type.

		This enables a client to quickly read the record and take action based on information in the claim before it has retrieved the full claim
*/
package metadata

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	ipnimd "github.com/ipni/go-libipni/metadata"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-varint"
	"github.com/storacha-network/indexing-service/pkg/capability/assert"
)

var (
	_ ipnimd.Protocol = (*ContentClaimMetadata)(nil)

	//go:embed metadata.ipldsch
	schemaBytes          []byte
	contentClaimMetadata schema.TypedPrototype
)

func init() {
	typeSystem, err := ipld.LoadSchemaBytes(schemaBytes)
	if err != nil {
		panic(fmt.Errorf("failed to load schema: %w", err))
	}
	t := typeSystem.TypeByName("ContentClaimMetadata")
	contentClaimMetadata = bindnode.Prototype((*ContentClaimMetadata)(nil), t)
}

// currently we just use experimental codecs for now
const TransportContentClaim = 0x3E0000

type ClaimType uint64

const (
	LocationCommitment ClaimType = iota
	IndexClaim
	EqualsClaim
)

var ClaimNames = map[ClaimType]string{
	LocationCommitment: assert.LocationAbility,
	IndexClaim:         assert.IndexAbility,
	EqualsClaim:        assert.EqualsAbility,
}

func (a ClaimType) String() string {
	return ClaimNames[a]
}

// ContentClaimMetadata represents metadata for a content claim
type ContentClaimMetadata struct {
	// kind of claim
	ClaimType ClaimType
	// based on the claim type, this can be used to access the key information in the claim without fetching the whole claim
	ShortCut []byte
	// ClaimCID indicates the cid of the claim - the claim should be fetchable by combining the http multiaddr of the provider with the claim cid
	ClaimCID cid.Cid
}

func (ccm *ContentClaimMetadata) ID() multicodec.Code {
	return TransportContentClaim
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (ccm *ContentClaimMetadata) MarshalBinary() ([]byte, error) {
	buf := bytes.NewBuffer(varint.ToUvarint(uint64(ccm.ID())))
	nd := bindnode.Wrap(ccm, contentClaimMetadata.Type())
	if err := dagcbor.Encode(nd, buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (ccm *ContentClaimMetadata) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	_, err := ccm.ReadFrom(r)
	return err
}

func (ccm *ContentClaimMetadata) ReadFrom(r io.Reader) (n int64, err error) {
	cr := &countingReader{r: r}
	v, err := varint.ReadUvarint(cr)
	if err != nil {
		return cr.readCount, err
	}
	id := multicodec.Code(v)
	if id != TransportContentClaim {
		return cr.readCount, fmt.Errorf("transport id does not match %s: %s", TransportContentClaim, id)
	}

	nb := contentClaimMetadata.NewBuilder()
	err = dagcbor.Decode(nb, cr)
	if err != nil {
		return cr.readCount, err
	}
	nd := nb.Build()
	read := bindnode.Unwrap(nd).(*ContentClaimMetadata)
	ccm.ClaimType = read.ClaimType
	ccm.ShortCut = read.ShortCut
	ccm.ClaimCID = read.ClaimCID
	return cr.readCount, nil
}

var ErrUnrecognizedAssertion = errors.New("unrecognized assertion type")

type ClaimPreview interface {
	isClaimPreview()
}

type LocationCommitmentPreview struct {
	Location url.URL
}

func (LocationCommitmentPreview) isClaimPreview() {}

type IndexClaimPreview struct {
	Index cid.Cid
}

func (IndexClaimPreview) isClaimPreview() {}

type EqualsClaimPreview struct {
	Equals cid.Cid
}

func (EqualsClaimPreview) isClaimPreview() {}

// ClaimPreview uses the claim type and short cut field to construct a preview of relevant data in the full claim
func (ccm *ContentClaimMetadata) ClaimPreview() (ClaimPreview, error) {
	switch ccm.ClaimType {
	case LocationCommitment:
		location, err := url.ParseRequestURI(string(ccm.ShortCut))
		if err != nil {
			return nil, err
		}
		return LocationCommitmentPreview{
			Location: *location,
		}, nil
	case IndexClaim:
		_, index, err := cid.CidFromBytes(ccm.ShortCut)
		if err != nil {
			return nil, err
		}
		return IndexClaimPreview{
			Index: index,
		}, nil
	case EqualsClaim:
		_, equals, err := cid.CidFromBytes(ccm.ShortCut)
		if err != nil {
			return nil, err
		}
		return EqualsClaimPreview{
			Equals: equals,
		}, nil
	default:
		return nil, ErrUnrecognizedAssertion
	}
}

// copied from go-libipni
var (
	_ io.Reader     = (*countingReader)(nil)
	_ io.ByteReader = (*countingReader)(nil)
)

type countingReader struct {
	readCount int64
	r         io.Reader
}

func (c *countingReader) ReadByte() (byte, error) {
	b := []byte{0}
	_, err := c.Read(b)
	return b[0], err
}

func (c *countingReader) Read(b []byte) (n int, err error) {
	read, err := c.r.Read(b)
	c.readCount += int64(read)
	return read, err
}
