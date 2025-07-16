// Copyright 2025 Canonical.

package jujucommands_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	qt "github.com/frankban/quicktest"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"golang.org/x/crypto/ssh"

	"github.com/canonical/jimm/v3/internal/jujucommands"
)

func getsMeSomeKeysBrah(c *qt.C) ([]byte, []byte) {
	// Generate RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	c.Assert(err, qt.IsNil)

	// Getz a priv key pemmy
	privPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		},
	)

	// OpenSSH pub key so Juju doesn't scream
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	c.Assert(err, qt.IsNil)

	return ssh.MarshalAuthorizedKey(pub), privPEM
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_Validate(c *qt.C) {
	p := jujucommands.BootstrapCmdParams{}

	c.Assert(p.Validate(), qt.ErrorMatches, ".*cloud \\[and region\\] name cannot be empty.*")

	p.CloudNameAndRegion = "testregion/testcloud"

	c.Assert(p.Validate(), qt.ErrorMatches, ".*controller name name cannot be empty.*")

	p.ControllerName = "my-controller"

	c.Assert(p.Validate(), qt.ErrorMatches, ".*login-token-refresh-url cannot be empty.*")

	p.LoginTokenRefreshURL = "myurl.com"

	c.Assert(p.Validate(), qt.IsNil)

	p.AgentVersion = "bad version"

	c.Assert(p.Validate(), qt.ErrorMatches, "invalid version \"bad version\"")

	p.AgentVersion = "1.1.1"

	c.Assert(p.Validate(), qt.IsNil)

	p.BootstrapTimeout = -1

	c.Assert(p.Validate(), qt.ErrorMatches, "bootstrap timeout cannot be less than or equal to 0")

	p.BootstrapTimeout = 1

	c.Assert(p.Validate(), qt.IsNil)
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_BuildBootstrapCmdStr(c *qt.C) {
	p := jujucommands.BootstrapCmdParams{
		CloudNameAndRegion:   "testregion/testcloud",
		ControllerName:       "my-controller",
		AgentVersion:         "1.1.1",
		BootstrapTimeout:     1000,
		LoginTokenRefreshURL: "myurl.com",
	}

	c.Assert(
		p.BuildBootstrapCmdStr(),
		qt.Equals,
		"bootstrap --login-token-refresh-url=myurl.com --agent-version=1.1.1 --config bootstrap-timeout=1000 testregion/testcloud my-controller",
	)

	p.AgentVersion = ""

	c.Assert(
		p.BuildBootstrapCmdStr(),
		qt.Equals,
		"bootstrap --login-token-refresh-url=myurl.com --config bootstrap-timeout=1000 testregion/testcloud my-controller",
	)

	p.BootstrapTimeout = 0

	c.Assert(
		p.BuildBootstrapCmdStr(),
		qt.Equals,
		"bootstrap --login-token-refresh-url=myurl.com testregion/testcloud my-controller",
	)
}

func (s *jujucommandsSuite) TestBootstrapCmdParams_RunBootstrapCmd(c *qt.C) {
	c.Patch(jujucommands.RunCmdWithOutputRetriever, func(store jujuclient.ClientStore, cmdAndArgs string) (<-chan jujucommands.OutputLine, error) {
		// Return chan that has one line inside
		return nil, nil
	})

	p := jujucommands.BootstrapCmdParams{
		CloudNameAndRegion:   "testregion/testcloud",
		ControllerName:       "my-controller",
		AgentVersion:         "1.1.1",
		BootstrapTimeout:     1000,
		LoginTokenRefreshURL: "myurl.com",
	}

	testCtx := c.Context()

	personalCloud := jujucloud.Cloud{
		Type: "lxd",
		AuthTypes: jujucloud.AuthTypes{
			jujucloud.CertificateAuthType,
		},
		// Some fake addr.
		Endpoint: "https://127.0.0.1:8443",
		Regions: []jujucloud.Region{
			{
				Name: "default",
				// Some fake addr.
				Endpoint: "https://127.0.0.1:8443",
			},
		},
	}

	cloudCred := *jujucloud.NewEmptyCloudCredential()
	cloudCred.AuthCredentials["default"] = jujucloud.NewCredential(jujucloud.CertificateAuthType, map[string]string{})

	pub, priv := getsMeSomeKeysBrah(c)
	_, _, cleanup, err := jujucommands.RunBootstrapCmd(
		testCtx,
		p,
		personalCloud,
		cloudCred,
		pub,
		priv,
	)
	c.Cleanup(func() {
		cleanup()
	})

	c.Assert(err, qt.IsNil)
}
