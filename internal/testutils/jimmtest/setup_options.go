package jimmtest

// SetupOption configures the various Setup* test-environment helpers.
//
// The options are intentionally shared across setup layers so callers can pass
// the same options through SetupWebsocketEnv -> SetupJimmWithControllers ->
// SetupJimmEnv without repeating per-layer modifier structs.
type SetupOption func(*SetupOptions)

// SetupOptions holds the normalized configuration derived from SetupOption.
//
// Fields are unexported; callers should use the With* helpers.
type SetupOptions struct {
	useRealAuthN     bool
	useHardcodedJWKS bool
}

func applySetupOptions(opts []SetupOption) SetupOptions {
	var o SetupOptions
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&o)
	}
	return o
}

// WithRealAuthN controls whether the test env uses the real authentication
// service (true) or the mock OAuth authenticator (false).
func WithRealAuthN() SetupOption {
	return func(o *SetupOptions) {
		o.useRealAuthN = true
	}
}

// WithHardcodedJWKS controls whether the test env seeds a hard-coded JWKS into
// the credential store (true) or starts the JWKS rotator (false).
func WithHardcodedJWKS() SetupOption {
	return func(o *SetupOptions) {
		o.useHardcodedJWKS = true
	}
}
