# JIMM Go SDK

JIMM (Juju Intelligent Model Manager) Go SDK provides a programmatic interface to the JIMM API.
JIMM enables management of multiple Juju controllers and their associated models through an unified interface.

## Features

- Add and remove controllers
- Manage clouds associated with controllers
- Query and manipulate model status
- Handle audit logs and events
- Manage user groups and access control
- Perform cross-model queries
- Migrate models between controllers
- Manage service accounts and credentials

## Installation

To use the JIMM Go SDK in your Go project, run:

```bash
go get github.com/canonical/jimm-go-sdk
```

## Usage

Here's a quick example of how to create a JIMM API client and use it:

```go
import (
    "github.com/canonical/jimm-go-sdk/api"
    "github.com/canonical/jimm-go-sdk/api/params"
)

// Create a new JIMM API client
client := api.NewClient(yourAPICaller)

// Add a new controller
req := &params.RemoveControllerRequest{
    Name:  "example-controller",
    Force: false,
}

info, err := client.RemoveController(req)
if err != nil {
    // Handle error
}
```

## Documentation

For detailed documentation on available methods and their parameters, please refer to [pkg.go.dev](https://pkg.go.dev/github.com/canonical/jimm-go-sdk)
