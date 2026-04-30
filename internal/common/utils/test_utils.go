// Copyright 2024 Canonical.
package utils

//go:fix inline
func IntToPointer(i int) *int {
	return new(i)
}

//go:fix inline
func StringToPointer(s string) *string {
	return new(s)
}
