package main

import (
	"fmt"
	"os"

	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
)

func main() {
	signer, err := ed25519.Generate()
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	asString, err := ed25519.Format(signer)
	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	fmt.Printf("# %s\n", signer.DID().String())
	fmt.Println(asString)
}
