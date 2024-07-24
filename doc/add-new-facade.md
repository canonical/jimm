# Adding new facades to JIMM

To add a new facade to JIMM, follow these steps:

1. Define methods for the facade in packages:
      - `internal/jujuclient` (if not a JIMM facade)

      - `internal/jimm`

      - `internal/jujuapi`

2. Register the new facade by creating or modifying an entry of the `facadeInit` map under the correct name in an `init()` function in its file in the `internal/jujuapi` package.

3. Update the JIMM API client in the [jimmapi](https://github.com/canonical/jimmapi) repository.

4. Add a corresponding command to [jimmctl](/cmd/jimmctl)
