package openfga

import (
	_ "embed"
)

//go:embed authorisation_model.json
var AuthModelFile []byte
