package testutil

import (
	crand "crypto/rand"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/url"
	"strconv"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/find/model"
	ipnimeta "github.com/ipni/go-libipni/metadata"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	mh "github.com/multiformats/go-multihash"
	cassert "github.com/storacha/go-libstoracha/capabilities/assert"
	ctypes "github.com/storacha/go-libstoracha/capabilities/types"
	"github.com/storacha/go-libstoracha/metadata"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld/block"
	"github.com/storacha/go-ucanto/principal"
	"github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/ucan"
	"github.com/storacha/indexing-service/pkg/blobindex"
)

func RandomBytes(size int) []byte {
	bytes := make([]byte, size)
	_, _ = crand.Read(bytes)
	return bytes
}

// RandomCAR creates a CAR with a single block of random bytes of the specified
// size. It returns the link of the root block, the hash of the CAR itself and
// the bytes of the CAR.
func RandomCAR(size int) (datamodel.Link, mh.Multihash, []byte) {
	bytes := RandomBytes(size)
	c, _ := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(bytes)

	root := cidlink.Link{Cid: c}
	r := car.Encode([]datamodel.Link{root}, func(yield func(block.Block, error) bool) {
		yield(block.NewBlock(root, bytes), nil)
	})
	carBytes, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	carDigest, err := mh.Sum(carBytes, mh.SHA2_256, -1)
	if err != nil {
		panic(err)
	}
	return root, carDigest, carBytes
}

var seedSeq int64

func RandomPeer() peer.ID {
	src := rand.NewSource(seedSeq)
	seedSeq++
	r := rand.New(src)
	_, publicKey, _ := crypto.GenerateEd25519Key(r)
	peerID, _ := peer.IDFromPublicKey(publicKey)
	return peerID
}

func RandomPrincipal() ucan.Principal {
	return RandomSigner()
}

func RandomSigner() principal.Signer {
	id, err := signer.Generate()
	if err != nil {
		panic(err)
	}
	return id
}

func RandomMultiaddr() multiaddr.Multiaddr {
	// generate a random ipv4 address
	addr := &net.TCPAddr{IP: net.IPv4(byte(rand.Intn(255)), byte(rand.Intn(255)), byte(rand.Intn(255)), byte(rand.Intn(255))), Port: rand.Intn(65535)}
	maddr, err := manet.FromIP(addr.IP)
	if err != nil {
		panic(err)
	}
	port, err := multiaddr.NewComponent(multiaddr.ProtocolWithCode(multiaddr.P_TCP).Name, strconv.Itoa(addr.Port))
	if err != nil {
		panic(err)
	}
	return multiaddr.Join(maddr, port)
}

func RandomCID() datamodel.Link {
	bytes := RandomBytes(10)
	c, _ := cid.Prefix{
		Version:  1,
		Codec:    cid.Raw,
		MhType:   mh.SHA2_256,
		MhLength: -1,
	}.Sum(bytes)
	return cidlink.Link{Cid: c}
}

func RandomMultihash() mh.Multihash {
	return RandomCID().(cidlink.Link).Hash()
}

func RandomMultihashes(count int) []mh.Multihash {
	if count <= 0 {
		panic(errors.New("count must be greater than 0"))
	}
	mhs := make([]mh.Multihash, 0, count)
	for range count {
		mhs = append(mhs, RandomMultihash())
	}
	return mhs
}

func RandomLocationClaim() ucan.Capability[cassert.LocationCaveats] {
	return cassert.Location.New(Service.DID().String(), cassert.LocationCaveats{
		Content:  ctypes.FromHash(RandomMultihash()),
		Location: []url.URL{*TestURL},
	})
}

func RandomLocationDelegation() delegation.Delegation {
	did, err := signer.Generate()
	if err != nil {
		panic(err)
	}
	delegation, err := delegation.Delegate(Service, did, []ucan.Capability[cassert.LocationCaveats]{RandomLocationClaim()})
	if err != nil {
		panic(err)
	}
	return delegation
}

func RandomIndexClaim() ucan.Capability[cassert.IndexCaveats] {
	return cassert.Index.New(Service.DID().String(), cassert.IndexCaveats{
		Content: RandomCID(),
		Index:   RandomCID(),
	})
}

func RandomIndexDelegation() delegation.Delegation {
	delegation, err := delegation.Delegate(Service, Service, []ucan.Capability[cassert.IndexCaveats]{RandomIndexClaim()})
	if err != nil {
		panic(err)
	}
	return delegation
}

func RandomEqualsClaim() ucan.Capability[cassert.EqualsCaveats] {
	return cassert.Equals.New(Service.DID().String(), cassert.EqualsCaveats{
		Content: ctypes.FromHash(RandomMultihash()),
		Equals:  RandomCID(),
	})
}

func RandomEqualsDelegation() delegation.Delegation {
	delegation, err := delegation.Delegate(Service, Service, []ucan.Capability[cassert.EqualsCaveats]{RandomEqualsClaim()})
	if err != nil {
		panic(err)
	}
	return delegation
}

func RandomProviderResult() model.ProviderResult {
	return model.ProviderResult{
		ContextID: RandomBytes(10),
		Metadata:  RandomBytes(10),
		Provider: &peer.AddrInfo{
			ID: RandomPeer(),
			Addrs: []multiaddr.Multiaddr{
				RandomMultiaddr(),
				RandomMultiaddr(),
			},
		},
	}
}

func RandomBitswapProviderResult() model.ProviderResult {
	pr := RandomProviderResult()
	bitswapMeta, _ := ipnimeta.Bitswap{}.MarshalBinary()
	pr.Metadata = bitswapMeta
	return pr
}

func RandomIndexClaimProviderResult() model.ProviderResult {
	indexMeta := metadata.IndexClaimMetadata{
		Index:      RandomCID().(cidlink.Link).Cid,
		Expiration: 0,
		Claim:      RandomCID().(cidlink.Link).Cid,
	}
	metaBytes, _ := indexMeta.MarshalBinary()

	pr := RandomProviderResult()
	pr.Metadata = metaBytes
	return pr
}

func RandomLocationCommitmentProviderResult() model.ProviderResult {
	shard := RandomCID().(cidlink.Link).Cid
	locationMeta := metadata.LocationCommitmentMetadata{
		Shard:      &shard,
		Range:      &metadata.Range{Offset: 128},
		Expiration: 0,
		Claim:      RandomCID().(cidlink.Link).Cid,
	}
	metaBytes, _ := locationMeta.MarshalBinary()

	pr := RandomProviderResult()
	pr.Metadata = metaBytes
	return pr
}

func RandomShardedDagIndexView(size int) (mh.Multihash, blobindex.ShardedDagIndexView) {
	root, digest, bytes := RandomCAR(size)
	shard, err := blobindex.FromShardArchives(root, [][]byte{bytes})
	if err != nil {
		panic(err)
	}
	return digest, shard
}
