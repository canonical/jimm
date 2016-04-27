// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/errgo.v1"
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

The location of the JEM server can be specified
as an environment variable:

	JUJU_JEM=<JEM server URL>

or as a command line flag:

	--jem-url <JEM server URL>

The latter takes precedence over the former.
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
	supercmd.Register(newAddControllerCommand())
	supercmd.Register(newCreateCommand())
	supercmd.Register(newCreateTemplateCommand())
	supercmd.Register(newGetCommand())
	supercmd.Register(newGenerateCommand())
	supercmd.Register(newGrantCommand())
	supercmd.Register(newListCommand())
	supercmd.Register(newListServersCommand())
	supercmd.Register(newLocationsCommand())
	supercmd.Register(newListTemplatesCommand())
	supercmd.Register(newRemoveCommand())
	supercmd.Register(newRevokeCommand())

	return supercmd
}

// commandBase holds the basis for JEM commands.
type commandBase struct {
	modelcmd.JujuCommandBase
	jemURL string
}

func (c *commandBase) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.jemURL, "jem-url", "", "URL of JEM server (defaults to $JUJU_JEM)")
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
	return jemclient.New(jemclient.NewParams{
		BaseURL: c.serverURL(),
		Client:  bakeryClient,
	}), nil
}

const jemServerURL = "https://api.jujucharms.com/jem"

// serverURL returns the JEM server URL.
// The returned value can be overridden by setting the JUJU_JEM
// model variable.
func (c *commandBase) serverURL() string {
	if c.jemURL != "" {
		return c.jemURL
	}
	if url := os.Getenv("JUJU_JEM"); url != "" {
		return url
	}
	return jemServerURL
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

// writeModel runs the given getEnv function and writes the result
// into the local controller/account/model cache
// using the given local model name and controller
// name. The controller name may be empty if unknown,
// in which case a new controller will be created
// when necessary.
//
// It returns the a string suitable for passing to "juju switch"
// to change to the new model.
//
// TODO(rog) re-use an old controller even if it does not fit
// the jem naming convention.
func writeModel(localModelName, localControllerName string, getEnv func() (*params.ModelResponse, error)) (string, error) {
	store := jujuclient.NewFileClientStore()

	if localControllerName != "" {
		ok, err := modelExists(store, localModelName, localControllerName)
		if err != nil {
			return "", errgo.Notef(err, "cannot check whether model exists")
		}
		if ok {
			return "", errgo.Notef(err, "local model %q already exists in controller %q", localModelName, localControllerName)
		}
	}

	resp, err := getEnv()
	if err != nil {
		return "", errgo.Mask(err)
	}

	// First try to connect to the model to ensure
	// that the response is somewhat sane.
	apiInfo := &api.Info{
		Tag:      names.NewUserTag(resp.User),
		Password: resp.Password,
		Addrs:    resp.HostPorts,
		CACert:   resp.CACert,
		ModelTag: names.NewModelTag(resp.UUID),
	}
	st, err := api.Open(apiInfo, api.DialOpts{})
	if err != nil {
		return "", errgo.Notef(err, "cannot open model")
	}
	st.Close()

	ctlName := jemControllerToLocalControllerName(resp.ControllerPath)
	if localControllerName == "" {
		localControllerName = ctlName
	} else if localControllerName != ctlName {
		return "", errgo.Newf("controller path %q in model response does not match expected controller %q", ctlName, localControllerName)
	}

	if err := ensureController(store, localControllerName, jujuclient.ControllerDetails{
		UnresolvedAPIEndpoints: resp.HostPorts,
		// We set APIEndpoints as well as UnresolvedAPIEndpoints because
		// it seems the the juju API connection code ignores UnresolvedAPIEndpoints.
		// See https://bugs.launchpad.net/juju-core/+bug/1566893.
		APIEndpoints:   resp.HostPorts,
		ControllerUUID: resp.ControllerUUID,
		CACert:         resp.CACert,
	}); err != nil {
		return "", errgo.Mask(err)
	}

	localAcctName := resp.User + "@local"
	// Now we've ensured that the controller exists, ensure
	// that the user account also exists.
	// TODO is every possible Juju user name also a valid
	// account name?
	if err := ensureAccount(store, localControllerName, localAcctName, jujuclient.AccountDetails{
		User:     localAcctName,
		Password: resp.Password,
	}); err != nil {
		return "", errgo.Mask(err)
	}

	if err := store.SetCurrentAccount(localControllerName, localAcctName); err != nil {
		return "", errgo.Notef(err, "cannot set %q to current user account", localAcctName)
	}

	// Check again that the model doesn't exist locally
	// before overwriting it. If we're creating a new model
	// this shouldn't happen because we pass in the controller
	// name and check at the start, but when getting an existing
	// model we haven't been able to check until now (but in
	// that case it doesn't matter that we return an error
	// because getting a model is idempotent).
	ok, err := modelExists(store, localModelName, localControllerName)
	if err != nil {
		return "", errgo.Notef(err, "cannot check whether model exists")
	}
	if ok {
		return "", errgo.Notef(err, "local model %q already exists in controller %q", localModelName, localControllerName)
	}

	if err := store.UpdateModel(localControllerName, localAcctName, localModelName, jujuclient.ModelDetails{
		ModelUUID: resp.UUID,
	}); err != nil {
		return "", errgo.Notef(err, "cannot update model %q", localModelName)
	}
	return localControllerName + ":" + localModelName, nil
}

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

// ensureAccount ensures that the given named account exists in
// the given store with the given name under the given controller.
// creating one if necessary.
func ensureAccount(store jujuclient.ClientStore, controllerName, acctName string, acct jujuclient.AccountDetails) error {
	oldAcct, err := store.AccountByName(controllerName, acctName)
	if err != nil && !errors.IsNotFound(err) {
		return errgo.Mask(err)
	}
	if err != nil || oldAcct.User == acct.User {
		// The controller doesn't exist or it exists with the same UUID.
		// In both these cases, update its details which will create
		// it if needed.
		if err := store.UpdateAccount(controllerName, acctName, acct); err != nil {
			return errgo.Notef(err, "cannot update account %q in controller %q", acctName, controllerName)
		}
		return nil
	}
	// The account already exists with a different user name.
	// This is a problem. Return an error and get the user
	// to sort it out.
	return errgo.Newf("account %q already exists with a different user name", acctName)
}

func jemControllerToLocalControllerName(p params.EntityPath) string {
	// Because we expect all controllers to be created under the
	// same user name, we'll treat the controller name as if it
	// were a global name space and ignore the user name.
	return "jem-" + string(p.Name)
}

func modelExists(store jujuclient.ClientStore, modelName, controllerName string) (bool, error) {
	accts, err := store.AllAccounts(controllerName)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errgo.Mask(err)
	}
	// Check that none of the existing accounts holds a model
	// with the desired name. This is somewhat more
	// conservative than necessary, but we can't pre-guess
	// the user that's going to be used.
	for acctName := range accts {
		_, err := store.ModelByName(controllerName, acctName, modelName)
		if err == nil {
			return true, nil
		}
		if !errors.IsNotFound(err) {
			return false, errgo.Mask(err)
		}
	}
	return false, nil
}
