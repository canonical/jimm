// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"os"
	"path"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/persistent-cookiejar"
	"github.com/juju/utils"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/errgo.v1"
	esform "gopkg.in/juju/environschema.v1/form"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery/form"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem")

// jujuLoggingConfigEnvKey matches osenv.JujuLoggingConfigEnvKey
// in the Juju project.
const jujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"

var cmdDoc = `
The juju jem command provides access to the JEM server.
The commands are at present for testing purposes only
and are not stable in any form.

The location of the JEM state server can be specified
as an environment variable:

	JUJU_JEM=<JEM server URL>

or as a command line flag:

	--jem-url <JEM server URL>

The latter takes precedence over the former.

Note that any juju state server used by JEM must
have hosted environments enabled by bootstrapping
it with the JUJU_DEV_FEATURE_FLAGS environment
variable set to include "jes".
`

// New returns a command that can execute juju-jem
// commands.
func New() cmd.Command {
	supercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "jem",
		UsagePrefix: "juju",
		Doc:         cmdDoc,
		Purpose:     "access the JEM server",
		Log: &cmd.Log{
			DefaultConfig: os.Getenv(jujuLoggingConfigEnvKey),
		},
	})
	supercmd.Register(&addServerCommand{})
	supercmd.Register(&changePermCommand{})
	supercmd.Register(&createCommand{})
	supercmd.Register(&createTemplateCommand{})
	supercmd.Register(&getCommand{})
	supercmd.Register(&listCommand{})
	supercmd.Register(&listServersCommand{})
	supercmd.Register(&listTemplatesCommand{})

	return supercmd
}

// environInfo returns information on the local environment
// with the given name. If envName is empty, the default
// environment will be used.
func environInfo(envName string) (configstore.EnvironInfo, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, errgo.Notef(err, "cannot get default configstore")
	}
	if envName == "" {
		envName, err = envcmd.GetDefaultEnvironment()
		if err != nil {
			return nil, errgo.Notef(err, "cannot find name of default environment")
		}
	}
	info, err := store.ReadInfo(envName)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read info for environment %q", envName)
	}
	return info, nil
}

// commandBase holds the basis for JEM commands.
type commandBase struct {
	cmd.CommandBase
	jemURL string
}

func (c *commandBase) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.jemURL, "jem-url", "", "URL of JEM server (defaults to $JUJU_JEM)")
}

// jemClient embeds a jemclient.Client and holds its associated cookie jar.
type jemClient struct {
	*jemclient.Client
	jar *cookiejar.Jar
}

// newClient creates and return a JEM client with access to
// the associated cookie jar used to save authorization
// macaroons. If authUsername and authPassword are provided, the resulting
// client will use HTTP basic auth with the given credentials.
func (c *commandBase) newClient(ctxt *cmd.Context) (*jemClient, error) {
	cookieFile := cookieFile()
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		// Can never happen in current implementation.
		panic(err)
	}
	if err := jar.Load(cookieFile); err != nil {
		return nil, errgo.Mask(err)
	}
	httpClient := httpbakery.NewHTTPClient()
	httpClient.Jar = jar
	httpbakeryClient := &httpbakery.Client{
		VisitWebPage: httpbakery.OpenWebBrowser,
		Client:       httpClient,
	}
	form.SetUpAuth(httpbakeryClient, &esform.IOFiller{
		In:  ctxt.Stdin,
		Out: ctxt.Stderr,
	})
	jclient := jemclient.New(jemclient.NewParams{
		BaseURL: c.serverURL(),
		Client:  httpbakeryClient,
	})
	return &jemClient{
		Client: jclient,
		jar:    jar,
	}, nil
}

// Close closes the jem client, saving any HTTP cookies.
func (c *jemClient) Close() {
	if err := c.jar.Save(); err != nil {
		logger.Errorf("cannot save cookies: %v", err)
	}
}

const jemServerURL = "https://api.jujucharms.com/jem"

// serverURL returns the JEM server URL.
// The returned value can be overridden by setting the JUJU_JEM
// environment variable.
func (c *commandBase) serverURL() string {
	if c.jemURL != "" {
		return c.jemURL
	}
	if url := os.Getenv("JUJU_JEM"); url != "" {
		return url
	}
	return jemServerURL
}

// cookieFile returns the path to the cookie used to store authorization
// macaroons. The returned value can be overridden by setting the
// JUJU_COOKIEFILE environment variable.
func cookieFile() string {
	if file := os.Getenv("JUJU_COOKIEFILE"); file != "" {
		return file
	}
	return path.Join(utils.Home(), ".go-cookies")
}

// entityPathValue holds an EntityPath that
// can be used as a flag value.
type entityPathValue struct {
	params.EntityPath
}

// Set implements gnuflag.Value.Set, enabling entityPathValue
// to be used as a custom flag value.
// The String method is implemented by EntityPath itself.
func (v *entityPathValue) Set(p string) error {
	if err := v.EntityPath.UnmarshalText([]byte(p)); err != nil {
		return errgo.Notef(err, "invalid entity path %q", p)
	}
	return nil
}

var _ gnuflag.Value = (*entityPathValue)(nil)

// entityPathValue holds a slice of EntityPaths that
// can be used as a flag value. Paths are comma separated,
// and at least one must be specified.
type entityPathsValue struct {
	paths []params.EntityPath
}

// Set implements gnuflag.Value.Set, enabling entityPathsValue
// to be used as a custom flag value.
func (v *entityPathsValue) Set(p string) error {
	parts := strings.Split(p, ",")
	if parts[0] == "" {
		return errgo.New("empty entity paths")
	}
	paths := make([]params.EntityPath, len(parts))
	for i, part := range parts {
		if err := paths[i].UnmarshalText([]byte(part)); err != nil {
			return errgo.Notef(err, "invalid entity path %q", part)
		}
	}
	v.paths = paths
	return nil
}

// String implements gnuflag.Value.String, enabling entityPathsValue
// to be used as a custom flag value.
func (v *entityPathsValue) String() string {
	ss := make([]string, len(v.paths))
	for i, p := range v.paths {
		ss[i] = p.String()
	}
	return strings.Join(ss, ",")
}

var _ gnuflag.Value = (*entityPathsValue)(nil)

// writeEnvironment runs the given getEnv function
// and writes the result as a local environment .jenv
// file with the given local name, saving also the
// given access password.
func writeEnvironment(localName string, getEnv func() (*params.EnvironmentResponse, error)) error {
	store, err := configstore.Default()
	if err != nil {
		return errgo.Notef(err, "cannot get default configstore")
	}
	// Check that the environment doesn't exist already.
	_, err = store.ReadInfo(localName)
	if err == nil {
		return errgo.Notef(err, "local environment %q already exists", localName)
	}
	if !errors.IsNotFound(err) {
		return errgo.Notef(err, "cannot check for existing local environment")
	}

	resp, err := getEnv()
	if err != nil {
		return errgo.Mask(err)
	}

	// First try to connect to the environment to ensure
	// that the response is somewhat sane.
	apiInfo := &api.Info{
		Tag:        names.NewUserTag(resp.User),
		Password:   resp.Password,
		Addrs:      resp.HostPorts,
		CACert:     resp.CACert,
		EnvironTag: names.NewEnvironTag(resp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	if err != nil {
		return errgo.Notef(err, "cannot open environment")
	}
	st.Close()

	envInfo := store.CreateInfo(localName)
	envInfo.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   resp.HostPorts,
		CACert:      resp.CACert,
		EnvironUUID: resp.UUID,
		ServerUUID:  resp.ServerUUID,
	})
	envInfo.SetAPICredentials(configstore.APICredentials{
		User:     resp.User,
		Password: resp.Password,
	})
	if err := envInfo.Write(); err != nil {
		return errgo.Notef(err, "cannot write environ info")
	}
	return nil
}
