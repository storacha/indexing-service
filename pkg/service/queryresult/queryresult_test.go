package queryresult

import (
	"io"
	"testing"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multicodec"
	multihash "github.com/multiformats/go-multihash/core"
	"github.com/storacha/go-libstoracha/blobindex"
	"github.com/storacha/go-libstoracha/bytemap"
	"github.com/storacha/go-libstoracha/testutil"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestSerde(t *testing.T) {
	claim := testutil.RandomLocationDelegation(t)
	claims := map[cid.Cid]delegation.Delegation{
		claim.Link().(cidlink.Link).Cid: claim,
	}

	_, index := testutil.RandomShardedDagIndexView(t, 138)
	indexBytes, err := io.ReadAll(testutil.Must(index.Archive())(t))
	require.NoError(t, err)
	indexLink, err := cid.Prefix{
		Version:  1,
		Codec:    uint64(multicodec.Car),
		MhType:   multihash.SHA2_256,
		MhLength: -1,
	}.Sum(indexBytes)
	require.NoError(t, err)

	contextID := types.ContextID{Hash: indexLink.Hash()}
	indexes := bytemap.NewByteMap[types.EncodedContextID, blobindex.ShardedDagIndexView](-1)
	indexes.Set(testutil.Must(contextID.ToEncoded())(t), index)

	msgs := []string{"hello world"}

	qr, err := Build(claims, indexes, WithMessage(msgs...))
	require.NoError(t, err)

	qr, err = Extract(Archive(qr))
	require.NoError(t, err)

	require.Len(t, qr.Claims(), 1)
	require.Equal(t, qr.Claims()[0].String(), claim.Link().String())
	require.Len(t, qr.Indexes(), 1)
	require.Equal(t, qr.Indexes()[0].String(), indexLink.String())
	require.Equal(t, qr.Messages(), msgs)
}
