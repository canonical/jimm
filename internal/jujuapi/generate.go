// Copyright 2026 Canonical.

package jujuapi

//go:generate go run github.com/canonical/jimm/v3/cmd/facadeversions -o facadeversions.go -package jujuapi -var SupportedFacadeVersions
//go:generate go tool mockgen -package jujuapi -typed -mock_names streamLoginManager=MockStreamLoginManager -destination ./streamauthentication_mock_test.go . streamLoginManager
