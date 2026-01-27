# facadeversions tool

This tool has two related purposes:

1. **Generate a reference file**: emit `internal/jujuapi/facadeversions.go` from the facade version registrations in `internal/jujuapi` (via `jujuapi.SupportedFacades()`).
2. **Compare against Juju**: diff JIMM’s supported facade versions against Juju’s reference set from `github.com/juju/juju/api.SupportedFacadeVersions()`.

## Generate

JIMM keeps a generated snapshot of supported facade versions for easy inspection.

To regenerate it:

```bash
go generate ./internal/jujuapi
```

Or run the generator directly:

```bash
go run ./cmd/facadeversions -o internal/jujuapi/facadeversions.go -package jujuapi -var SupportedFacadeVersions
```

## Compare

To print a full diff between Juju and JIMM:

```bash
go run ./cmd/facadeversions compare
```

For CI (fail only when JIMM’s *highest* supported version for a facade is behind Juju’s):

```bash
go run ./cmd/facadeversions compare --error-on-version-lag
```

When `--error-on-version-lag` is set, output is restricted to the lagging facades only and the command exits non-zero if any lag is detected.

