// Copyright 2020 Canonical Ltd.

// go-dqlite only works properly if the go-sqlite3 package is built with
// the libsqlite3 tag. The stub in the this package marks the dqlite test
// as skipped, so that it is clear the test has not run.
// +build !libsqlite3
//go:build !libsqlite3

package db_test

import "testing"

func TestDQLite(t *testing.T) {
	t.Skip("dqlite requires '-tags libsqlite3'")
}
