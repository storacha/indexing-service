package main

import (
	"bytes"
	"fmt"
	"net/url"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/capabilities/assert"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/core/ipld"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/indexing-service/pkg/blobindex"
	"github.com/storacha/indexing-service/pkg/client"
	"github.com/storacha/indexing-service/pkg/types"
	"github.com/urfave/cli/v2"
)

var queryCmd = &cli.Command{
	Name:  "query",
	Usage: "query an indexing server and print out the results",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "url",
			Aliases: []string{"u"},
			Value:   "https://indexer.storacha.network",
			Usage:   "URL of the indexer to query.",
		},
		&cli.StringFlag{
			Name:    "space",
			Aliases: []string{"s"},
			Usage:   "DID of a space to filter results by.",
		},
		&cli.StringFlag{
			Name:    "type",
			Aliases: []string{"t"},
			Usage:   "type of query to perform ['standard' | 'location' | 'index_or_location']",
			Value:   "standard",
		},
	},
	Action: func(cCtx *cli.Context) error {
		serviceURL, err := url.Parse(cCtx.String("url"))
		if err != nil {
			return fmt.Errorf("parsing service URL: %w", err)
		}

		serviceDID, err := did.Parse(fmt.Sprintf("did:web:%s", serviceURL.Hostname()))
		if err != nil {
			return fmt.Errorf("parsing service DID: %w", err)
		}

		c, err := client.New(serviceDID, *serviceURL)
		if err != nil {
			return fmt.Errorf("creating client: %w", err)
		}

		var cids []cid.Cid
		for _, arg := range cCtx.Args().Slice() {
			cid, err := parseCID(arg)
			if err != nil {
				return fmt.Errorf("parsing CID/multihash: %w", err)
			}
			cids = append(cids, cid)
		}
		if len(cids) == 0 {
			return fmt.Errorf("missing CID/multihash for query: %w", err)
		}

		var digests []multihash.Multihash
		for _, cid := range cids {
			digests = append(digests, cid.Hash())
		}

		var spaces []did.DID
		if cCtx.IsSet("space") {
			space, err := did.Parse(cCtx.String("space"))
			if err != nil {
				return fmt.Errorf("parsing space DID: %w", err)
			}
			spaces = append(spaces, space)
		}

		queryType := types.QueryTypeStandard
		if cCtx.IsSet("type") {
			queryType, err = types.ParseQueryType(cCtx.String("type"))
			if err != nil {
				return fmt.Errorf("error in query type: %w", err)
			}
		}

		qr, err := c.QueryClaims(cCtx.Context, types.Query{
			Type:   queryType,
			Hashes: digests,
			Match:  types.Match{Subject: spaces},
		})
		if err != nil {
			return fmt.Errorf("querying service: %w", err)
		}

		blocks, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(qr.Blocks()))
		if err != nil {
			return fmt.Errorf("reading result blocks: %w", err)
		}

		blockmap := map[ipld.Link]ipld.Block{}
		for b, err := range qr.Blocks() {
			if err != nil {
				return fmt.Errorf("iterating blocks: %w", err)
			}
			blockmap[b.Link()] = b
		}

		fmt.Println("")
		fmt.Println("Query:")
		fmt.Printf("  Hashes (%d):\n", len(digests))
		for _, digest := range digests {
			fmt.Printf("    %s\n", formatDigest(digest))
		}
		if len(spaces) > 0 {
			fmt.Printf("  Spaces (%d):\n", len(spaces))
			for _, space := range spaces {
				fmt.Printf("    %s\n", space.String())
			}
		}
		fmt.Println("")
		fmt.Println("Results:")
		fmt.Printf("  Claims (%d):\n", len(qr.Claims()))
		for _, root := range qr.Claims() {
			claim, err := delegation.NewDelegationView(root, blocks)
			if err != nil {
				return fmt.Errorf("decoding delegation: %w", err)
			}

			fmt.Printf("    %s\n", claim.Link())
			fmt.Println("      Type:")
			fmt.Printf("        %s\n", claim.Capabilities()[0].Can())
			switch claim.Capabilities()[0].Can() {
			case assert.LocationAbility:
				nb, err := assert.LocationCaveatsReader.Read(claim.Capabilities()[0].Nb())
				if err != nil {
					return fmt.Errorf("reading %s caveats: %w", assert.LocationAbility, err)
				}
				fmt.Println("      Content:")
				fmt.Printf("        %s\n", formatDigest(nb.Content.Hash()))
				if nb.Space != did.Undef {
					fmt.Println("      Space:")
					fmt.Printf("        %s\n", nb.Space.String())
				}
				fmt.Println("      Locations:")
				for _, location := range nb.Location {
					fmt.Printf("        %s\n", location.String())
				}
				if nb.Range != nil {
					fmt.Printf("      Range: %d-", nb.Range.Offset)
					if nb.Range.Length != nil {
						fmt.Printf("%d\n", nb.Range.Offset+*nb.Range.Length)
					} else {
						fmt.Println("")
					}
				}
			case assert.EqualsAbility:
				nb, err := assert.EqualsCaveatsReader.Read(claim.Capabilities()[0].Nb())
				if err != nil {
					return fmt.Errorf("reading %s caveats: %w", assert.LocationAbility, err)
				}
				fmt.Println("      Content:")
				fmt.Printf("        %s\n", formatDigest(nb.Content.Hash()))
				fmt.Println("      Equals:")
				fmt.Printf("        %s\n", nb.Equals)
			case assert.IndexAbility:
				nb, err := assert.IndexCaveatsReader.Read(claim.Capabilities()[0].Nb())
				if err != nil {
					return fmt.Errorf("reading %s caveats: %w", assert.LocationAbility, err)
				}
				fmt.Println("      Content:")
				fmt.Printf("        %s\n", nb.Content)
				fmt.Println("      Index:")
				fmt.Printf("        %s\n", nb.Index)
			default:
				fmt.Println("      (Unknown Claim)")
			}
		}

		fmt.Println("")
		fmt.Printf("  Indexes (%d):\n", len(qr.Indexes()))
		for _, root := range qr.Indexes() {
			blk, ok, err := blocks.Get(root)
			if err != nil {
				return fmt.Errorf("getting index block: %w", err)
			}
			if !ok {
				return fmt.Errorf("missing index block: %w", err)
			}
			index, err := blobindex.Extract(bytes.NewReader(blk.Bytes()))
			if err != nil {
				return fmt.Errorf("decoding index: %w", err)
			}

			fmt.Printf("    %s\n", root)
			fmt.Println("      Content:")
			fmt.Printf("        %s\n", index.Content())
			fmt.Printf("      Shards (%d):\n", index.Shards().Size())
			for shard, slices := range index.Shards().Iterator() {
				fmt.Printf("        %s\n", formatDigest(shard))
				fmt.Printf("          Slices (%d):\n", slices.Size())
				for digest, position := range slices.Iterator() {
					fmt.Printf("            %s @ %d-%d\n", formatDigest(digest), position.Offset, position.Offset+position.Length)
				}
			}
		}

		return nil
	},
}

func parseCID(input string) (cid.Cid, error) {
	c, err := cid.Parse(input)
	if err == nil {
		return c, nil
	}

	_, b, err := multibase.Decode(input)
	if err != nil {
		return cid.Undef, err
	}

	_, digest, err := multihash.MHFromBytes(b)
	if err != nil {
		return cid.Undef, err
	}

	return cid.NewCidV1(cid.Raw, digest), nil
}

func formatDigest(digest multihash.Multihash) string {
	str, _ := multibase.Encode(multibase.Base58BTC, digest)
	return str
}
