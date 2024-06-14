// This package exists to hold JIMM's OpenFGA authorisation model.
// It embeds the auth model and provides it for tests.
package openfga

import (
	_ "embed"
)

//go:embed authorisation_model.json
var AuthModelFile []byte
