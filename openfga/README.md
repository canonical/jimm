# openfga

## Introduction
The OpenFGA package holds our authorisation model and a go embed to pass the auth model into tests.
It also holds
tests to ensure the authorisation model is working correctly.

## Requirements

### VSCode Extension
Name: OpenFGA
Id: openfga.openfga-vscode
Description: Language support for OpenFGA authorization models
Version: 0.2.24
Publisher: OpenFGA
VS Marketplace Link: https://marketplace.visualstudio.com/items?itemName=openfga.openfga-vscode

### OpenFGA CLI
go install github.com/openfga/cli/cmd/fga@latest

## Adding / modifying [to] the authorsation model
1. Open the authorisation_model.fga 
2. Make your modification
3. Run: `make transform-auth-model`
6. Add tests to tests.fga.yaml - Learn more [here](https://openfga.dev/docs/modeling/testing)
7. Run them via: `make test-auth-model`

## Test Structure
In order to avoid the potential entanglement of separate tests the tuples are artifically split into groups using this naming convention: (type):(2-letter test name)-(type)-(id)
The GitHub action supports running all tests in a directory, but keeping them in a single file improves the local development experience because the CLI does not.