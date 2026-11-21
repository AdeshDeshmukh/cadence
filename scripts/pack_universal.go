//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/AdeshDeshmukh/cadence/pkg/build"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <arm64-binary> <amd64-binary> <output-universal-binary>\n", os.Args[0])
		os.Exit(1)
	}

	arm64Path := os.Args[1]
	amd64Path := os.Args[2]
	outPath := os.Args[3]

	if err := build.CreateUniversal(arm64Path, amd64Path, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating universal binary: %v\n", err)
		os.Exit(1)
	}

	info, err := build.InspectMachO(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error inspecting universal binary: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Universal binary created successfully: %s (%v, %d bytes)\n", outPath, info.Architectures, info.Size)
}
