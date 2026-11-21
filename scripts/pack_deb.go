//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/AdeshDeshmukh/cadence/pkg/packaging"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "Usage: %s <cadence-bin> <cadenced-bin> <service-unit> <out-deb-path>\n", os.Args[0])
		os.Exit(1)
	}

	cadenceBin := os.Args[1]
	cadencedBin := os.Args[2]
	serviceUnit := os.Args[3]
	outPath := os.Args[4]

	spec := packaging.DebSpec{
		Package:      "cadence",
		Version:      "1.0.0",
		Architecture: "amd64",
		Maintainer:   "Adesh Deshmukh <adeshkd123@gmail.com>",
		Description:  "Local AI-first voice dictation CLI and developer engine",
	}

	if err := packaging.BuildDebArchive(spec, cadenceBin, cadencedBin, serviceUnit, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error building .deb archive: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated .deb package: %s\n", outPath)
}
