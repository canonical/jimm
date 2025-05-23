// Copyright 2025 Canonical.

package config

// configManager exposes the configuration of the JIMM controller when
// the ControllerConfig facade is called on the controller root.
// At the moment it only exposes a map, but in the future we could fetch
// the configuration from the database or other sources.
type configManager struct {
	config ControllerConfig
}

// ControllerConfig holds the configuration to be returned when the ControllerConfig
// facade is called on the controller root.
type ControllerConfig struct {
	// ControllerUUID is the UUID of the JIMM controller.
	ControllerUUID string

	// PublicDNSName is returned to Juju clients on login.
	// It is the hostname that will be used during TLS verification.
	PublicDNSName string

	// SSHPort is the port for SSH connections.
	SSHPort int

	// SSHPublicHostKey is the host key for SSH connections.
	SSHPublicHostKey string
}

// NewConfigManager creates a new config manager with the given configuration.
func NewConfigManager(config ControllerConfig) (*configManager, error) {
	return &configManager{
		config: config,
	}, nil
}

// GetConfig returns the configuration for the controller converted in the jujuparams format.
func (c *configManager) GetConfig() (ControllerConfig, error) {
	return c.config, nil
}
