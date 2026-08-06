package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/zk/shielded"
)

func main() {
	out := flag.String("out", "", "write vectors to this path instead of stdout")
	flag.Parse()

	data, err := shielded.DeterministicTestVectorsJSON()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate vectors: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')
	if *out == "" {
		os.Stdout.Write(data)
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write vectors: %v\n", err)
		os.Exit(1)
	}
}
