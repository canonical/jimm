// Copyright 2024 Canonical.
//
// jujuclient2 holds a juju client for usage within JIMM.
//
// It only exposes facade methods that JIMM actively uses and
// should more be required, the respective interfaces within client.go
// should be updated
//
// Within dialer.go is a SimpleConnector, with a custom LoginProvider
// specifically for JWT login. This is something specific to JIMM.
//
// Within cache_dialer.go is a cache implementation for connections.
// It ensures that two routines do not connect independently and instead
// share the same connection PER controller.
package jujuclient2
