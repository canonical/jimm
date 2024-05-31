// Copyright 2023 Canonical Ltd.
package jimm

var NewOpenFGAClient = newOpenFGAClient

func (s *Service) GetCleanups() []func() {
	return s.cleanups
}
