// Copyright 2025 Canonical.

package service

var (
	NewOpenFGAClient           = newOpenFGAClient
	ParseURLWithOptionalScheme = parseURLWithOptionalScheme
)

// GetCleanups export `Service.cleanups` field for testing purposes.
func (s *Service) GetCleanups() []func() error {
	return s.cleanups
}
