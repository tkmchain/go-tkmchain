// Copyright 2026 The go-tkmchain Authors
// This file is part of the go-tkmchain library.

//go:build !cgo || !randomx
// +build !cgo !randomx

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "randomx-extminer requires cgo and the randomx build tag")
	os.Exit(1)
}
