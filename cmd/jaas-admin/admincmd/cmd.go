// Copyright 2015-2016 Canonical Ltd.

package admincmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/juju/aclstore/aclclient"
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"github.com/CanonicalLtd/jimm/jemclient"
	"github.com/CanonicalLtd/jimm/params"
)

// jujuLoggingConfigEnvKey matches osenv.JujuLoggingConfigEnvKey
// in the Juju project.
const jujuLoggingConfigEnvKey = "JUJU_LOGGING_CONFIG"

var cmdDoc = `
The jaas admin command provides access to the managing server.
The commands are at present for testing purposes only
and are not stable in any form.

The location of the managing server can be specified
as an environment variable:

	JIMM_URL=<managing server URL>

or as a command line flag on the admin subcommands
(note that this does not work when used on the jaas
admin command itself).

	--jimm-url <managing server URL>

The latter takes precedence over the former.
`

// New returns a command that can execute jaas-admin
// commands.
func New() cmd.Command {
	supercmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "admin",
		UsagePrefix: "jaas",
		Doc:         cmdDoc,
		Purpose:     "access the managing server",
		Log: &cmd.Log{
			DefaultConfig: os.Getenv(jujuLoggingConfigEnvKey),
		},
	})
	supercmd.Register(newAddControllerCommand())
	supercmd.Register(newControllersCommand())
	supercmd.RegisterAlias("list-controllers", "controllers", nil)
	supercmd.Register(newDeprecateControllerCommand())
	supercmd.Register(newGrantCommand())
	supercmd.Register(newModelsCommand())
	supercmd.RegisterAlias("list-models", "models", nil)
	supercmd.Register(newRemoveCommand())
	supercmd.Register(newRevokeCommand())

	return supercmd
}

// commandBase holds the basis for commands.
type commandBase struct {
	modelcmd.CommandBase
	jimmURL string
}

func (c *commandBase) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.jimmURL, "jimm-url", "", "URL of managing server (defaults to $JIMM_URL)")
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
	bakeryClient.VisitWebPage = httpbakery.OpenWebBrowser
	return jar, bakeryClient, nil
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
			Doer:    bakeryDoer{bakeryClient},
		}),
		jar:  jar,
		ctxt: ctxt,
	}, nil
}

// bakeryDoer wraps a gopkg.in/macaroon-bakery.v2-unstable/httpbakey.Client so
// that it behaves correctly as a gopkg.in/httprequest.v1.Doer.
type bakeryDoer struct {
	client *httpbakery.Client
}

func (d bakeryDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body == nil {
		return d.client.Do(req)
	}
	body, ok := req.Body.(io.ReadSeeker)
	if !ok {
		return nil, errgo.New("unsupported request body type")
	}
	req1 := *req
	req1.Body = nil
	return d.client.DoWithBody(&req1, body)
}

const jimmServerURL = "https://jimm.jujucharms.com"

// serverURL returns the JIMM server URL.
// The returned value can be overridden by setting the JIMM_URL variable.
func (c *commandBase) serverURL() string {
	if c.jimmURL != "" {
		return c.jimmURL
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

var _ gnuflag.Value = (*entityPathsValue)(nil)
