package publisher

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"os"
	"path/filepath"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	ipldschema "github.com/ipld/go-ipld-prime/schema"
	"github.com/ipni/go-libipni/dagsync/ipnisync/head"
	"github.com/ipni/go-libipni/ingest/schema"
	"github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/ipld/codec/json"
	"github.com/storacha/go-ucanto/core/ipld/hash/sha256"
)

var ErrNotFound = errors.New("not found")

const (
	headKey = "head"
)

// maxEntryChunkSize is the maximum number of multihashes each advertisement
// entry chunk may contain.
var maxEntryChunkSize = 16384

type Store interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Put(ctx context.Context, key string, data io.Reader) error
}

type ProviderContextTable interface {
	Get(ctx context.Context, p peer.ID, contextID []byte) ([]byte, error)
	Put(ctx context.Context, p peer.ID, contextID []byte, data []byte) error
	Delete(ctx context.Context, p peer.ID, contextID []byte) error
}

type AdvertStore interface {
	Encode(ctx context.Context, id ipld.Link, w io.Writer) error
	PutAdvert(ctx context.Context, ad schema.Advertisement) (ipld.Link, error)
	// Advert retrieves an existing advert from the store.
	Advert(ctx context.Context, id ipld.Link) (schema.Advertisement, error)
	// Entries returns an iterable of multihashes from the store for the
	// given root of an existing advertisement entries chain.
	Entries(ctx context.Context, root ipld.Link) iter.Seq2[multihash.Multihash, error]
	// PutEntries writes a given set of multihash entries to do the store and returns the root cid
	PutEntries(ctx context.Context, entries iter.Seq[multihash.Multihash]) (ipld.Link, error)
	Head(ctx context.Context) (*head.SignedHead, error)
	PutHead(ctx context.Context, newHead *head.SignedHead) (ipld.Link, error)
	EncodeHead(ctx context.Context, w io.Writer) error
	ChunkLinkForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) (ipld.Link, error)
	PutChunkLinkForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte, adCid ipld.Link) error
	DeleteChunkLinkForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) error
	MetadataForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) (metadata.Metadata, error)
	PutMetadataForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte, md metadata.Metadata) error
	DeleteMetadataForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) error
}

type AdStore struct {
	store      Store
	chunkLinks ProviderContextTable
	metadata   ProviderContextTable
}

var _ AdvertStore = (*AdStore)(nil)

func (s *AdStore) PutAdvert(ctx context.Context, ad schema.Advertisement) (ipld.Link, error) {
	return PutAdvert(ctx, s.store, ad)
}

func (s *AdStore) Advert(ctx context.Context, id ipld.Link) (schema.Advertisement, error) {
	return Advert(ctx, s.store, id)
}

func (s *AdStore) Entries(ctx context.Context, root ipld.Link) iter.Seq2[multihash.Multihash, error] {
	return Entries(ctx, s.store, root)
}

func (s *AdStore) PutEntries(ctx context.Context, mhs iter.Seq[multihash.Multihash]) (ipld.Link, error) {
	return PutEntries(ctx, s.store, mhs, maxEntryChunkSize)
}

func (s *AdStore) Encode(ctx context.Context, id datamodel.Link, w io.Writer) error {
	return Encode(ctx, s.store, id, w)
}

func (s *AdStore) ChunkLinkForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) (datamodel.Link, error) {
	return ChunkLink(ctx, s.chunkLinks, p, contextID)
}

func (s *AdStore) PutChunkLinkForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte, chunkLink datamodel.Link) error {
	return PutChunkLink(ctx, s.chunkLinks, p, contextID, chunkLink)
}

func (s *AdStore) DeleteChunkLinkForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) error {
	return s.chunkLinks.Delete(ctx, p, contextID)
}

func (s *AdStore) Head(ctx context.Context) (*head.SignedHead, error) {
	return Head(ctx, s.store)
}

func (s *AdStore) PutHead(ctx context.Context, newHead *head.SignedHead) (datamodel.Link, error) {
	return PutHead(ctx, s.store, newHead)
}

func (s *AdStore) EncodeHead(ctx context.Context, w io.Writer) error {
	return EncodeHead(ctx, s.store, w)
}

func (s *AdStore) MetadataForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) (metadata.Metadata, error) {
	return Metadata(ctx, s.metadata, p, contextID)
}

func (s *AdStore) PutMetadataForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte, md metadata.Metadata) error {
	return PutMetadata(ctx, s.metadata, p, contextID, md)
}

func (s *AdStore) DeleteMetadataForProviderAndContextID(ctx context.Context, p peer.ID, contextID []byte) error {
	return s.metadata.Delete(ctx, p, contextID)
}

func NewAdvertStore(store Store, chunkLinks, metadata ProviderContextTable) *AdStore {
	return &AdStore{store, chunkLinks, metadata}
}

func Advert(ctx context.Context, ds Store, id ipld.Link) (schema.Advertisement, error) {
	var ad schema.Advertisement
	r, err := ds.Get(ctx, id.String())
	if err != nil {
		return ad, err
	}
	defer r.Close()
	v, err := io.ReadAll(r)
	if err != nil {
		return ad, err
	}
	ad, err = schema.BytesToAdvertisement(asCID(id), v)
	if err != nil {
		return ad, err
	}
	return ad, nil
}

func PutAdvert(ctx context.Context, ds Store, adv schema.Advertisement) (ipld.Link, error) {
	return store(ctx, ds, &adv, schema.AdvertisementPrototype.Type())
}

func PutEntries(ctx context.Context, ds Store, entries iter.Seq[multihash.Multihash], chunkSize int) (next ipld.Link, err error) {
	mhs := make([]multihash.Multihash, 0, chunkSize)
	var mhCount, chunkCount int
	for mh := range entries {
		mhs = append(mhs, mh)
		mhCount++
		if len(mhs) >= chunkSize {
			next, err = store(ctx, ds, toChunk(mhs, next), schema.EntryChunkPrototype.Type())
			if err != nil {
				return nil, err
			}
			chunkCount++
			// NewLinkedListOfMhs makes it own copy, so safe to reuse mhs
			mhs = mhs[:0]
		}
	}
	if len(mhs) != 0 {
		next, err = store(ctx, ds, toChunk(mhs, next), schema.EntryChunkPrototype.Type())
		if err != nil {
			return nil, err
		}
		chunkCount++
	}

	log.Infow("Generated linked chunks of multihashes", "totalMhCount", mhCount, "chunkCount", chunkCount)
	return next, nil
}

func Entries(ctx context.Context, ds Store, root ipld.Link) iter.Seq2[multihash.Multihash, error] {
	return func(yield func(multihash.Multihash, error) bool) {
		cur := root
		for cur != nil && cur != schema.NoEntries {
			r, err := ds.Get(ctx, cur.String())
			if err != nil {
				yield(nil, err)
				return
			}
			defer r.Close()
			v, err := io.ReadAll(r)
			if err != nil {
				yield(nil, err)
				return
			}
			ent, err := schema.BytesToEntryChunk(asCID(cur), v)
			if err != nil {
				yield(nil, err)
				return
			}

			for _, d := range ent.Entries {
				if !yield(d, nil) {
					return
				}
			}

			cur = ent.Next
		}
	}
}

func Encode(ctx context.Context, ds Store, lnk ipld.Link, w io.Writer) error {
	r, err := ds.Get(ctx, lnk.String())
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(w, r)
	return err
}

func Head(ctx context.Context, ds Store) (*head.SignedHead, error) {
	r, err := ds.Get(ctx, headKey)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return head.Decode(r)
}

func EncodeHead(ctx context.Context, ds Store, w io.Writer) error {
	r, err := ds.Get(ctx, headKey)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = io.Copy(w, r)
	return err
}

func PutHead(ctx context.Context, ds Store, newHead *head.SignedHead) (datamodel.Link, error) {
	blk, err := block.Encode(newHead, head.SignedHeadPrototype.Type(), json.Codec, sha256.Hasher)
	if err != nil {
		return nil, err
	}
	err = ds.Put(ctx, headKey, bytes.NewReader(blk.Bytes()))
	if err != nil {
		return nil, err
	}
	return blk.Link(), nil
}

func ChunkLink(ctx context.Context, ds ProviderContextTable, p peer.ID, contextID []byte) (datamodel.Link, error) {
	data, err := ds.Get(ctx, p, contextID)
	if err != nil {
		return nil, err
	}
	_, c, err := cid.CidFromBytes(data)
	if err != nil {
		return nil, err
	}
	return cidlink.Link{Cid: c}, nil
}

func PutChunkLink(ctx context.Context, ds ProviderContextTable, p peer.ID, contextID []byte, lnk datamodel.Link) error {
	return ds.Put(ctx, p, contextID, []byte(lnk.Binary()))
}

func Metadata(ctx context.Context, ds ProviderContextTable, p peer.ID, contextID []byte) (metadata.Metadata, error) {
	md := metadata.Default.New()
	data, err := ds.Get(ctx, p, contextID)
	if err != nil {
		return md, err
	}
	if err := md.UnmarshalBinary(data); err != nil {
		return md, err
	}
	return md, nil
}

func PutMetadata(ctx context.Context, ds ProviderContextTable, p peer.ID, contextID []byte, md metadata.Metadata) error {
	data, err := md.MarshalBinary()
	if err != nil {
		return err
	}
	return ds.Put(ctx, p, contextID, data)
}

func store(ctx context.Context, ds Store, value any, typ ipldschema.Type) (ipld.Link, error) {
	blk, err := block.Encode(value, typ, json.Codec, sha256.Hasher)
	if err != nil {
		return nil, err
	}
	err = ds.Put(ctx, blk.Link().String(), bytes.NewReader(blk.Bytes()))
	if err != nil {
		return nil, err
	}
	return blk.Link(), nil
}

func toChunk(mhs []multihash.Multihash, next ipld.Link) *schema.EntryChunk {
	chunk := schema.EntryChunk{
		Entries: mhs,
	}
	if next != nil {
		chunk.Next = next
	}
	return &chunk
}

func IsNotFound(err error) bool {
	// solve for the unfortuante lack of standards on not found errors
	return errors.Is(err, datastore.ErrNotFound) || errors.Is(err, ErrNotFound)
}

func providerContextKey(provider peer.ID, contextID []byte) datastore.Key {
	return datastore.NewKey(provider.String() + "/" + string(contextID))
}

type dsProviderContextTable struct {
	ds datastore.Batching
}

func (d *dsProviderContextTable) Delete(ctx context.Context, p peer.ID, contextID []byte) error {
	return d.ds.Delete(ctx, providerContextKey(p, contextID))
}

func (d *dsProviderContextTable) Get(ctx context.Context, p peer.ID, contextID []byte) ([]byte, error) {
	return d.ds.Get(ctx, providerContextKey(p, contextID))
}

func (d *dsProviderContextTable) Put(ctx context.Context, p peer.ID, contextID []byte, data []byte) error {
	return d.ds.Put(ctx, providerContextKey(p, contextID), data)
}

var _ ProviderContextTable = (*dsProviderContextTable)(nil)

type dsStoreAdapter struct {
	ds datastore.Batching
}

// Get implements Store.
func (d *dsStoreAdapter) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	data, err := d.ds.Get(ctx, datastore.NewKey(key))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Put implements Store.
func (d *dsStoreAdapter) Put(ctx context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return d.ds.Put(ctx, datastore.NewKey(key), data)

}

var _ Store = (*dsStoreAdapter)(nil)

type directoryStore struct {
	directory string
}

// Get implements Store.
func (d *directoryStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := filepath.Abs(filepath.Join(d.directory, key))
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Put implements Store.
func (d *directoryStore) Put(ctx context.Context, key string, data io.Reader) error {
	path, err := filepath.Abs(filepath.Join(d.directory, key))
	if err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, data)
	return err
}

var _ Store = (*directoryStore)(nil)
