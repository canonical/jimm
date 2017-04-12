// Copyright 2015-2016 Canonical Ltd.

package admincmd

import (
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
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
	supercmd.Register(newGrantCommand())
	supercmd.Register(newModelsCommand())
	supercmd.RegisterAlias("list-models", "models", nil)
	supercmd.Register(newLocationsCommand())
	supercmd.Register(newRemoveCommand())
	supercmd.Register(newRevokeCommand())

	return supercmd
}

// commandBase holds the basis for commands.
type commandBase struct {
	modelcmd.JujuCommandBase
	jimmURL string
}

func (c *commandBase) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.jimmURL, "jimm-url", "", "URL of managing server (defaults to $JIMM_URL)")
}

// newClient creates and return a JEM client with access to
// the associated cookie jar used to save authorization
// macaroons. If authUsername and authPassword are provided, the resulting
// client will use HTTP basic auth with the given credentials.
func (c *commandBase) newClient(ctxt *cmd.Context) (*jemclient.Client, error) {
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	bakeryClient.VisitWebPage = httpbakery.OpenWebBrowser
	bakeryClient.WebPageVisitor = nil
	return jemclient.New(jemclient.NewParams{
		BaseURL: c.serverURL(),
		Client:  bakeryClient,
	}), nil
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

// ensureController ensures that the given named controller exists in
// the store with the given details, creating one if necessary.
func ensureController(store jujuclient.ClientStore, controllerName string, ctl jujuclient.ControllerDetails) error {
	oldCtl, err := store.ControllerByName(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errgo.Mask(err)
	}
	if err != nil || oldCtl.ControllerUUID == ctl.ControllerUUID {
		// The controller doesn't exist or it exists with the same UUID.
		// In both these cases, update its details which will create
		// it if needed.
		if err := store.UpdateController(controllerName, ctl); err != nil {
			return errgo.Notef(err, "cannot update controller %q", controllerName)
		}
		return nil
	}
	// The controller already exists with a different UUID.
	// This is a problem. Return an error and get the user
	// to sort it out.
	// TODO if there are no accounts models stored under the controller,
	// we *could* just replace the controller details, but that's
	// probably a bad idea.
	return errgo.Newf("controller %q already exists with a different UUID (old %s; new %s)", controllerName, oldCtl.ControllerUUID, ctl.ControllerUUID)
}

// ensureAccount ensures that the given named account exists in the given
// store under the given controller. creating one if necessary.
func ensureAccount(store jujuclient.ClientStore, controllerName string, acct jujuclient.AccountDetails) error {
	oldAcct, err := store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errgo.Mask(err)
	}
	if err != nil || oldAcct.User == acct.User {
		// The controller doesn't exist or it exists with the same UUID.
		// In both these cases, update its details which will create
		// it if needed.
		if err := store.UpdateAccount(controllerName, acct); err != nil {
			return errgo.Notef(err, "cannot update account in controller %q", controllerName)
		}
		return nil
	}
	// The account already exists with a different user name.
	// This is a problem. Return an error and get the user
	// to sort it out.
	return errgo.Newf("account in controller %q already exists with a different user name", controllerName)
}

const jemControllerPrefix = "jem-"

func jemControllerToLocalControllerName(p params.EntityPath) string {
	// Because we expect all controllers to be created under the
	// same user name, we'll treat the controller name as if it
	// were a global name space and ignore the user name.
	return jemControllerPrefix + string(p.Name)
}

// modelExists checks if the model with the given name exists.
// If controllerName is non-empty, it checks only in that controller;
// otherwise it checks all controllers.
// If a model is found, it returns the name of its controller
// otherwise it returns the empty string.
func modelExists(store jujuclient.ClientStore, modelName, controllerName string) (string, error) {
	var controllerNames []string
	if controllerName != "" {
		controllerNames = []string{controllerName}
	} else {
		// We don't know the controller name in advance, so
		// be conservative and check all jem-prefixed controllers
		// for the model name.
		ctls, err := store.AllControllers()
		if err != nil {
			return "", errgo.Notef(err, "cannot get local controllers")
		}
		for name := range ctls {
			if strings.HasPrefix(name, jemControllerPrefix) {
				controllerNames = append(controllerNames, name)
			}
		}
	}
	for _, controllerName := range controllerNames {
		_, err := store.ModelByName(controllerName, modelName)
		if err == nil {
			return controllerName, nil
		}
		if !errors.IsNotFound(err) {
			return "", errgo.Mask(err)
		}
	}
	return "", nil
}
