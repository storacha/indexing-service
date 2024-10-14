package publisher

import (
	"context"
	"iter"

	"github.com/ipld/go-ipld-prime"
	ipldschema "github.com/ipld/go-ipld-prime/schema"
	"github.com/ipld/go-ipld-prime/storage"
	"github.com/ipni/go-libipni/ingest/schema"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/core/ipld/codec/json"
	"github.com/storacha/go-ucanto/core/ipld/hash/sha256"
)

type Store interface {
	storage.WritableStorage
	storage.ReadableStorage
}
type AdvertStore interface {
	PutAdvert(ctx context.Context, ad schema.Advertisement) (ipld.Link, error)
	// Advert retrieves an existing advert from the store.
	Advert(ctx context.Context, id ipld.Link) (schema.Advertisement, error)
	// Entries returns an iterable of multihashes from the store for the
	// given root of an existing advertisement entries chain.
	Entries(ctx context.Context, root ipld.Link) iter.Seq2[multihash.Multihash, error]

	// PutEntries writes a given set of multihash entries to do the store and returns the root cid
	PutEntries(ctx context.Context, entries iter.Seq[multihash.Multihash]) (ipld.Link, error)
}

type AdStore struct {
	adverts Store
	entries Store
}

func (s *AdStore) PutAdvert(ctx context.Context, ad schema.Advertisement) (ipld.Link, error) {
	return PutAdvert(ctx, s.adverts, ad)
}

func (s *AdStore) Advert(ctx context.Context, id ipld.Link) (schema.Advertisement, error) {
	return Advert(ctx, s.adverts, id)
}

func (s *AdStore) Entries(ctx context.Context, root ipld.Link) iter.Seq2[multihash.Multihash, error] {
	return Entries(ctx, s.entries, root)
}

func (s *AdStore) PutEntries(ctx context.Context, mhs iter.Seq[multihash.Multihash]) (ipld.Link, error) {
	return PutEntries(ctx, s.entries, mhs, maxEntryChunkSize)
}
func NewAdvertStore(adverts, entries Store) *AdStore {
	return &AdStore{adverts, entries}
}

func Advert(ctx context.Context, ds Store, id ipld.Link) (schema.Advertisement, error) {
	var ad schema.Advertisement
	v, err := ds.Get(ctx, id.String())
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
	return store(ctx, ds, adv, schema.AdvertisementPrototype.Type())
}

func PutEntries(ctx context.Context, ds Store, entries iter.Seq[multihash.Multihash], chunkSize int) (next ipld.Link, err error) {
	mhs := make([]multihash.Multihash, 0, chunkSize)
	var mhCount, chunkCount int
	for mh := range entries {
		mhs = append(mhs, mh)
		mhCount++
		if len(mhs) >= chunkSize {
			next, err = store(ctx, ds, toChunk(mhs, next), schema.AdvertisementPrototype.Type())
			if err != nil {
				return nil, err
			}
			chunkCount++
			// NewLinkedListOfMhs makes it own copy, so safe to reuse mhs
			mhs = mhs[:0]
		}
	}
	if len(mhs) != 0 {
		next, err = store(ctx, ds, toChunk(mhs, next), schema.AdvertisementPrototype.Type())
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
			v, err := ds.Get(ctx, cur.String())
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

func store(ctx context.Context, ds Store, value any, typ ipldschema.Type) (ipld.Link, error) {
	blk, err := block.Encode(value, typ, json.Codec, sha256.Hasher)
	if err != nil {
		return nil, err
	}
	err = ds.Put(ctx, blk.Link().String(), blk.Bytes())
	if err != nil {
		return nil, err
	}
	return blk.Link(), nil
}

func toChunk(mhs []multihash.Multihash, next ipld.Link) schema.EntryChunk {
	chunk := schema.EntryChunk{
		Entries: mhs,
	}
	if next != nil {
		chunk.Next = next
	}
	return chunk
}
