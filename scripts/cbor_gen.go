package main

import (
	"fmt"
	"os"
	"path/filepath"

	blobindex "github.com/storacha/indexing-service/pkg/blobindex/datamodel"
	cborgen "github.com/whyrusleeping/cbor-gen"
)

func runCborGen() error {
	genName, err := filepath.Abs("./pkg/blobindex/datamodel/shardeddagindex_cbor_gen.go")
	if err != nil {
		return err
	}
	return cborgen.Gen{
		MaxArrayLength:  2 << 16, // Maximum length for arrays = 131072
		MaxByteLength:   2 << 20, // Maximum length for byte slices
		MaxStringLength: 2 << 20, // Maximum length for strings
	}.WriteTupleEncodersToFile(genName, "datamodeltype", blobindex.PositionModel{}, blobindex.BlobSliceModel{}, blobindex.BlobIndexModel{})
}

func main() {
	fmt.Print("Generating Cbor Marshal/Unmarshal...")

	if err := runCborGen(); err != nil {
		fmt.Println("Failed: ")
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}
