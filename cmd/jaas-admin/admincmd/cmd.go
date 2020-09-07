// Copyright 2015-2016 Canonical Ltd.

package admincmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/juju/aclstore/aclclient"
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	cookiejar "github.com/juju/persistent-cookiejar"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/CanonicalLtd/jimm/jemclient"
	"github.com/CanonicalLtd/jimm/params"
)

// jujuLoggingConfigEnvKey matches osenv.JujuLoggingConfigEnvKey
// in the Juju project.
const jujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"

const jimmServerURL = "https://jimm.jujucharms.com"

var cmdDoc = `
The jaas command provides access to the managing server. The commands 
are at present for testing purposes only and are not stable in any form.

The location of the managing server can be specified
as an environment variable:

	JIMM_URL=<managing server URL>

or as a command line flag:

	--jimm-url <managing server URL>

The latter takes precedence over the former.
`

// New returns a command that can execute jaas commands.
func New() cmd.Command {
	base := new(commandBase)
	supercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "jaas",
		Doc:         cmdDoc,
		Purpose:     "access the managing server",
		GlobalFlags: base,
		Log: &cmd.Log{
			DefaultConfig: os.Getenv(jujuLoggingConfigEnvKey),
		},
	})
	supercmd.Register(newAddControllerCommand(base))
	supercmd.Register(newControllersCommand(base))
	supercmd.RegisterAlias("list-controllers", "controllers", nil)
	supercmd.Register(newDeprecateControllerCommand(base))
	supercmd.Register(newGrantCommand(base))
	supercmd.Register(newModelsCommand(base))
	supercmd.RegisterAlias("list-models", "models", nil)
	supercmd.Register(newRemoveCommand(base))
	supercmd.Register(newRevokeCommand(base))

	return supercmd
}

type commandBase struct {
	cmd.CommandBase

	url string
}

// AddFlags implements cmd.FlagAdder.
func (cmd *commandBase) AddFlags(f *gnuflag.FlagSet) {
	f.StringVar(&cmd.url, "jimm-url", "", "URL of managing server (defaults to $JIMM_URL)")
}

// newClient creates and return a JEM client with access to
// the associated cookie jar used to save authorization
// macaroons. If authUsername and authPassword are provided, the resulting
// client will use HTTP basic auth with the given credentials.
// The returned client should be closed after use.
func (c *commandBase) newClient(ctxt *cmd.Context) (*client, error) {
	jar, bakeryClient, err := c.newBakeryClient()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &client{
		Client: jemclient.New(jemclient.NewParams{
			BaseURL: c.serverURL(),
			Client:  bakeryClient,
		}),
		jar:  jar,
		ctxt: ctxt,
	}, nil
}

func (c *commandBase) newBakeryClient() (*cookiejar.Jar, *httpbakery.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, errgo.Mask(err)
	}
	bakeryClient := httpbakery.NewClient()
	bakeryClient.Client.Jar = jar
	bakeryClient.AddInteractor(httpbakery.WebBrowserInteractor{})
	return jar, bakeryClient, nil
}

type client struct {
	*jemclient.Client
	jar  *cookiejar.Jar
	ctxt *cmd.Context
}

func (c *client) Close() {
	if err := c.jar.Save(); err != nil {
		fmt.Fprintf(c.ctxt.Stderr, "cannot save cookies: %v", err)
	}
}

// newACLClient creates and return a aclClient with access to
// the associated cookie jar used to save authorization
// macaroons. If authUsername and authPassword are provided, the resulting
// client will use HTTP basic auth with the given credentials.
// The returned client should be closed after use.
func (c *commandBase) newACLClient(ctxt *cmd.Context) (*aclClient, error) {
	jar, bakeryClient, err := c.newBakeryClient()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &aclClient{
		Client: aclclient.New(aclclient.NewParams{
			BaseURL: c.serverURL() + "/admin/acls",
			Doer:    bakeryClient,
		}),
		jar:  jar,
		ctxt: ctxt,
	}, nil
}

type aclClient struct {
	*aclclient.Client
	jar  *cookiejar.Jar
	ctxt *cmd.Context
}

func (c *aclClient) Close() {
	if err := c.jar.Save(); err != nil {
		fmt.Fprintf(c.ctxt.Stderr, "cannot save cookies: %v", err)
	}
}

// serverURL returns the JIMM server URL.
// The returned value can be overridden by setting the JIMM_URL variable.
func (c *commandBase) serverURL() string {
	if c.url != "" {
		return c.url
	}
	if url := os.Getenv("JIMM_URL"); url != "" {
		return url
	}
	return jimmServerURL
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

func wrapContext(ctxt *cmd.Context) (_ context.Context, cancel func()) {
	ctx, cancel := context.WithCancel(context.Background())
	sc := make(chan os.Signal)
	ctxt.InterruptNotify(sc)
	go func() {
		defer ctxt.StopInterruptNotify(sc)
		select {
		case <-sc:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

var _ gnuflag.Value = (*entityPathsValue)(nil)
