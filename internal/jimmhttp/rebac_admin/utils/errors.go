// Copyright 2024 Canonical.
package utils

import "errors"

// MultiErr handles cases where multiple errors need to be collected.
type MultiErr struct {
	errors []error
}

// AppendError stores a new error on a slice of existing errors.
func (m *MultiErr) AppendError(err error) {
	m.errors = append(m.errors, err)
}

// Error returns a single error that is the concatention of all the collected errors.
func (m *MultiErr) Error() error {
	return errors.Join(m.errors...)
}

// String returns the string format of all collected errors.
func (m *MultiErr) String() string {
	if err := m.Error(); err != nil {
		return err.Error()
	}
	return ""
}
