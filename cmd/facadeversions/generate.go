// Copyright 2026 Canonical.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/canonical/jimm/v3/cmd/facadeversions/generator"
	"github.com/canonical/jimm/v3/internal/jujuapi"
)

func generate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	outPath := fs.String("o", "", "output Go file path")
	pkgName := fs.String("package", "jujuapi", "package name for the generated file")
	varName := fs.String("var", "SupportedFacadeVersions", "variable name for the generated map")
	_ = fs.Parse(args)

	if *outPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -o output path")
		os.Exit(2)
	}

	facades := jujuapi.SupportedFacades()
	src, err := generator.Generate(facades, generator.Options{
		PackageName: *pkgName,
		VarName:     *varName,
		Source:      "internal/jujuapi.SupportedFacades()",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	tmp := *outPath + ".tmp"
	//nolint:gosec // Don't enforce 0o600 or less permission.
	if err := os.WriteFile(tmp, src, 0o664); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, *outPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
