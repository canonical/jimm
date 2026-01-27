// Copyright 2026 Canonical.

package main

import (
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "compare":
			compare(os.Args[2:])
			return
		case "generate":
			generate(os.Args[2:])
			return
		}
	}
	// Default to generate for go:generate friendliness.
	generate(os.Args[1:])
}
