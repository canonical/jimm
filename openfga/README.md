# openfga

## Introduction
The OpenFGA package holds our authorisation model and a go embed. It also holds
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
3. Open the Command Pallette using Ctrl+Shift+P (Windows) or Command+Shift+P (OSX)
4. Select OpenFGA: Transform DSL to JSON
5. Save the file over the existing authorisation_model.json
6. Add tests to tests.fga.yaml - Learn more [here](https://openfga.dev/docs/modeling/testing)
7. Run them via: `make test-auth-model`