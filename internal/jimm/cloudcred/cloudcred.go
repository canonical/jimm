// Copyright 2024 Canonical.

//go:generate go run generate.go -o attr.go

package cloudcred

import "fmt"

// IsVisibleAttribute returns whether a cloud-credential attribute is known
// not to be hidden and can therefore does not need to be redacted.
func IsVisibleAttribute(provider, authtype, attribute string) bool {
	return attr[fmt.Sprintf("%s\x1e%s\x1e%s", provider, authtype, attribute)]
}
