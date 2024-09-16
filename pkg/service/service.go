package service

import (
	"net/url"

	mh "github.com/multiformats/go-multihash"
	"github.com/storacha-network/go-ucanto/did"
	"github.com/storacha-network/indexing-service/pkg/blobindex"
)

type Match struct {
	Subject []did.DID
}

type Query struct {
	Hashes []mh.Multihash
	Match  Match
}

type Range struct {
	Offset uint64
	Length uint64
}

type QueryResult struct {
	Hash  mh.Multihash
	Shard url.URL
	Range []Range
}

type IndexingService interface {
	Query(Query) []QueryResult
	AddIndex(SpaceDID did.DID, index blobindex.BlobIndex) AddResult
	AddLocation(SpaceID)
}
