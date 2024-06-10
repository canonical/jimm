// Copyright 2023 Canonical Ltd.
package jimm

var NewOpenFGAClient = newOpenFGAClient

// GetCleanups export `Service.cleanups` field for testing purposes.
func (s *Service) GetCleanups() []func() error {
	return s.cleanups
}
