package main

import (
	"fmt"
	"os"

	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

func usage() {
	fmt.Print(`cadence

Usage:
  cadence version
  cadence doctor
  cadence dictate
  cadence transcribe <file>
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version.Version)
	case "dictate":
		fmt.Println("dictation session requested")
	case "transcribe":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "missing audio file")
			os.Exit(2)
		}
		fmt.Printf("transcription requested: %s\n", os.Args[2])
	default:
		usage()
	}
}
