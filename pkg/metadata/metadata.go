/*
	 Package metadata implements protocols for publishing content claims on IPNI

	  The goal is to enable partial publishing of content claims to IPNI

		The rules for publishing content claims records to IPNI are as follows:

		Content claims should be published to IPNI by the original issuer of the content claim.

		The ContextID for the content claim should be the cid of the content the claim is about,
		except in the case of a location commitment, where the content ID should be:
			hash(audience public key, content cid multihash)

		The claim record MUST be able to be looked up on IPNI from the content cid multihash (or double encryption thereof)

		The claim MAY be able to be looked up by additional multihashes, particularly in the case of the IndexClaim, where
		the record should be accessible from any multihash inside the index

		The metadata for the claim is structured to maximize utility of the record while minimizing size

		To generally respect the 100 byte maximum size for IPNI records, we do not encode the claim itself, but rather its CID.

		The full claim must be retrievable through an http multiaddr of the provider which contains path segments of the form `{claim}`.
		To retrieve the claim, replace every `{claim}` with the string encoding of the claim CID

		For a location commitment, the content must retrievable through an http multiaddr of the provider
		which contains path segments of the form `{shard}`. To retrieve the claim, replace every `{shard}` with the string encoding
		of the shard cid in the metadata, or if not present, the CIDv1 encoding using RAW codec of the multihash used to lookup the record
		Additionally, if a Range parameter is present in the metadata, it should be translated into a range HTTP header when retrieving
		content

		However, in order to enable faster chaining of requests and general processing, we add additional fields to encode
		specific information from the full claim.

		This enables a client to quickly read the record and take action based on information in the claim before it has retrieved the full claim
*/
package metadata

import (
	"bytes"
	// for import
	_ "embed"
	"fmt"
	"io"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	ipnimd "github.com/ipni/go-libipni/metadata"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-varint"
)

var (
	_ ipnimd.Protocol = (*IndexClaimMetadata)(nil)

	//go:embed metadata.ipldsch
	schemaBytes []byte
)

var nodePrototypes = map[multicodec.Code]schema.TypedPrototype{}

func init() {
	typeSystem, err := ipld.LoadSchemaBytes(schemaBytes)
	if err != nil {
		panic(fmt.Errorf("failed to load schema: %w", err))
	}
	nodePrototypes[IndexClaimID] = bindnode.Prototype((*IndexClaimMetadata)(nil), typeSystem.TypeByName("IndexClaimMetadata"))
	nodePrototypes[EqualsClaimID] = bindnode.Prototype((*EqualsClaimMetadata)(nil), typeSystem.TypeByName("EqualsClaimMetadata"))
	nodePrototypes[LocationCommitmentID] = bindnode.Prototype((*LocationCommitmentMetadata)(nil), typeSystem.TypeByName("LocationCommitmentMetadata"))
}

// metadata identifiers
// currently we just use experimental codecs for now

// IndexClaimID is the multicodec for index claims
const IndexClaimID = 0x3E0000

// EqualsClaimID is the multicodec for equals claims
const EqualsClaimID = 0x3E0001

// LocationCommitmentID is the multicodec for location commitments
const LocationCommitmentID = 0x3E0002

var MetadataContext ipnimd.MetadataContext

func init() {
	mdctx := ipnimd.Default
	mdctx = mdctx.WithProtocol(IndexClaimID, func() ipnimd.Protocol { return &IndexClaimMetadata{} })
	mdctx = mdctx.WithProtocol(EqualsClaimID, func() ipnimd.Protocol { return &EqualsClaimMetadata{} })
	mdctx = mdctx.WithProtocol(LocationCommitmentID, func() ipnimd.Protocol { return &LocationCommitmentMetadata{} })
	MetadataContext = mdctx
	// WithProtocol creates a _new_ context. Assign it to Default so claim
	// metadata can be decoded elsewhere by refrencing libipni metadata.Default
	// i.e. ipni-publisher
	ipnimd.Default = mdctx
}

type HasClaim interface {
	GetClaim() cid.Cid
}

/*
	 IndexClaimMetadata represents metadata for an index claim
		Index claim metadata
*/
type IndexClaimMetadata struct {
	// Index represents the cid of the index for this claim
	Index cid.Cid
	// Expiration as unix epoch in seconds
	Expiration int64
	// Claim indicates the cid of the claim - the claim should be fetchable by combining the http multiaddr of the provider with the claim cid
	Claim cid.Cid
}

func (i *IndexClaimMetadata) ID() multicodec.Code {
	return IndexClaimID
}
func (i *IndexClaimMetadata) MarshalBinary() ([]byte, error)            { return marshalBinary(i) }
func (i *IndexClaimMetadata) UnmarshalBinary(data []byte) error         { return unmarshalBinary(i, data) }
func (i *IndexClaimMetadata) ReadFrom(r io.Reader) (n int64, err error) { return readFrom(i, r) }
func (i *IndexClaimMetadata) GetClaim() cid.Cid {
	return i.Claim
}

// EqualsClaimMetadata represents metadata for an equals claim
type EqualsClaimMetadata struct {
	// Equals represents an equivalent cid to the content cid that was used for lookup
	Equals cid.Cid
	// Expiration as unix epoch in seconds
	Expiration int64
	// Claim indicates the cid of the claim - the claim should be fetchable by combining the http multiaddr of the provider with the claim cid
	Claim cid.Cid
}

func (e *EqualsClaimMetadata) ID() multicodec.Code {
	return EqualsClaimID
}
func (e *EqualsClaimMetadata) MarshalBinary() ([]byte, error)            { return marshalBinary(e) }
func (e *EqualsClaimMetadata) UnmarshalBinary(data []byte) error         { return unmarshalBinary(e, data) }
func (e *EqualsClaimMetadata) ReadFrom(r io.Reader) (n int64, err error) { return readFrom(e, r) }
func (e *EqualsClaimMetadata) GetClaim() cid.Cid {
	return e.Claim
}

type Range struct {
	Offset uint64
	Length *uint64
}

// LocationCommitmentMetadata represents metadata for an equals claim
type LocationCommitmentMetadata struct {
	// Shard is an optional alternate cid to use to lookup this location -- if the looked up shard is part of a larger shard
	Shard *cid.Cid
	// Range is an optional byte range within a shard
	Range *Range
	// Expiration as unix epoch in seconds
	Expiration int64
	// Claim indicates the cid of the claim - the claim should be fetchable by combining the http multiaddr of the provider with the claim cid
	Claim cid.Cid
}

func (l *LocationCommitmentMetadata) ID() multicodec.Code {
	return LocationCommitmentID
}
func (l *LocationCommitmentMetadata) MarshalBinary() ([]byte, error) { return marshalBinary(l) }
func (l *LocationCommitmentMetadata) UnmarshalBinary(data []byte) error {
	return unmarshalBinary(l, data)
}
func (l *LocationCommitmentMetadata) ReadFrom(r io.Reader) (n int64, err error) {
	return readFrom(l, r)
}
func (l *LocationCommitmentMetadata) GetClaim() cid.Cid {
	return l.Claim
}

type hasID[T any] interface {
	*T
	ID() multicodec.Code
}

func marshalBinary(metadata ipnimd.Protocol) ([]byte, error) {
	buf := bytes.NewBuffer(varint.ToUvarint(uint64(metadata.ID())))
	nd := bindnode.Wrap(metadata, nodePrototypes[metadata.ID()].Type())
	if err := dagcbor.Encode(nd, buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unmarshalBinary[PT hasID[T], T any](val PT, data []byte) error {
	r := bytes.NewReader(data)
	_, err := readFrom(val, r)
	return err
}

func readFrom[PT hasID[T], T any](val PT, r io.Reader) (int64, error) {
	cr := &countingReader{r: r}
	v, err := varint.ReadUvarint(cr)
	if err != nil {
		return cr.readCount, err
	}
	id := multicodec.Code(v)
	if id != val.ID() {
		return cr.readCount, fmt.Errorf("transport id does not match %s: %s", val.ID(), id)
	}

	fmt.Println(val.ID(), nodePrototypes[val.ID()])
	nb := nodePrototypes[val.ID()].NewBuilder()
	err = dagcbor.Decode(nb, cr)
	if err != nil {
		return cr.readCount, err
	}
	nd := nb.Build()
	read := bindnode.Unwrap(nd).(PT)
	*val = *read
	return cr.readCount, nil
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
