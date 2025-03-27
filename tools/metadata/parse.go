// Parse base64 encoded metadata and print it.
//
// This program takes the base64 `Metadata` value encoded in a response from
// IPNI. e.g. https://cid.contact/cid/bafybeighzj4zh5ptwqvdndazjxoq7t3abr5cat3iekx6xttdquj7waeihm
//
// Usage: go run ./tools/metadata/parse.go <base64_encoded_metadata>
//
// Examples:
//
// go run ./tools/metadata/parse.go gID4AaNhY9gqWCUAAXESIMW3kLyk7pHCD2de1EPfTaItNlGvQ7FoUIl2VYlit9DUYWUAYWnYKlgmAAGCBBIg7pPaVlvlZ4ROzdm676yzyA66LvU3RjucTsUxz6nCDsU=
// Type:   0x3e0000 (Index Claim)
// Value:  &{Index:bagbaiera52j5uvs34vtyitwn3g5o7lftzahlulxvg5ddxhcoyuy47kocb3cq Expiration:0 Claim:bafyreigfw6ilzjhoshba6z262rb56tncfu3fdl2dwfufbclwkwewfn6q2q}
//
// go run ./tools/metadata/parse.go gBI=
// Type:   0x900 (Transport Bitswap)
// Value:  &{}
//
// go run ./tools/metadata/parse.go oBIA
// Type:   0x920 (Transport IPFS Gateway HTTP)
// Value:  &{}
package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/multiformats/go-multicodec"
	"github.com/storacha/go-libstoracha/metadata"
)

var protos = map[multicodec.Code]string{
	// Storacha
	metadata.LocationCommitmentID: "Location Commitment",
	metadata.IndexClaimID:         "Index Claim",
	metadata.EqualsClaimID:        "Equals Claim",
	// General
	multicodec.TransportBitswap:             "Transport Bitswap",
	multicodec.TransportIpfsGatewayHttp:     "Transport IPFS Gateway HTTP",
	multicodec.TransportGraphsyncFilecoinv1: "Transport Graphsync Filecoin v1",
}

func main() {
	in := os.Args[1]
	if in == "" {
		panic("missing base64 metadata argument")
	}

	metaBytes, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(fmt.Errorf("decoding base64: %w", err))
	}

	md := metadata.MetadataContext.New()
	err = md.UnmarshalBinary(metaBytes)
	if err != nil {
		panic(fmt.Errorf("decoding metadata: %w", err))
	}

	for p, n := range protos {
		data := md.Get(p)
		if data != nil {
			fmt.Printf("Type:\t0x%x (%s)\n", int(p), n)
			fmt.Printf("Value:\t%+v\n", data)
			return
		}
	}

	fmt.Println("Unknown metadata")
}
