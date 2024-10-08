// Copyright 2024 Canonical.

package service

var NewOpenFGAClient = newOpenFGAClient

// GetCleanups export `Service.cleanups` field for testing purposes.
func (s *Service) GetCleanups() []func() error {
	return s.cleanups
}
