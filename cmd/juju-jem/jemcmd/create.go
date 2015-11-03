// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	jujuconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type createCommand struct {
	commandBase

	srvPath       entityPathValue
	envPath       entityPathValue
	templatePaths entityPathsValue
	configFile    string
	localName     string
}

func newCreateCommand() cmd.Command {
	return envcmd.WrapBase(&createCommand{})
}

var createDoc = `
The create command creates a new environment inside the specified state
server. Its argument specifies the JEM name of the new environment.

When one or more templates paths are specified, the final configuration
is determined by starting with the first and adding attributes from
each one in turn, finally adding any attributes specified in
the configuration file specified by the --config flag.
`

func (c *createCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<user>/<envname>",
		Purpose: "Create a new environment in JEM",
		Doc:     createDoc,
	}
}

func (c *createCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.Var(&c.srvPath, "state-server", "")
	f.Var(&c.srvPath, "s", "state server to create the environment in")
	f.Var(&c.templatePaths, "template", "")
	f.Var(&c.templatePaths, "t", "comma-separated templates to use for config attributes")

	f.StringVar(&c.configFile, "config", "", "YAML config file containing environment configuration")
	f.StringVar(&c.localName, "local", "", "local name for environment (as used for juju switch). Defaults to <envname>")
}

func (c *createCommand) Init(args []string) error {
	if len(args) != 1 {
		return errgo.Newf("got %d arguments, want 1", len(args))
	}
	if err := c.envPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if c.srvPath.EntityPath == (params.EntityPath{}) {
		return errgo.Newf("state server must be specified")
	}
	if c.localName == "" {
		c.localName = string(c.envPath.Name)
	}
	return nil
}

func (c *createCommand) Run(ctxt *cmd.Context) error {
	client, err := c.newClient(ctxt)
	if err != nil {
		return errgo.Mask(err)
	}

	config := make(map[string]interface{})
	if c.configFile != "" {
		data, err := ioutil.ReadFile(c.configFile)
		if err != nil {
			return errgo.Notef(err, "cannot read configuration file")
		}
		if err := yaml.Unmarshal(data, &config); err != nil {
			return errgo.Notef(err, "cannot unmarshal %q", c.configFile)
		}
	}
	jesInfo, err := client.GetJES(&params.GetJES{
		EntityPath: c.srvPath.EntityPath,
	})
	if err != nil {
		return errgo.Notef(err, "cannot get JES info")
	}
	defaultsCtxt := schemaContext{
		envName:      c.envPath.Name,
		providerType: jesInfo.ProviderType,
		knownAttrs:   config,
	}
	config, err = defaultsCtxt.generateConfig(jesInfo.Schema)
	if err != nil {
		return errgo.Notef(err, "invalid configuration")
	}
	// Generate a random password for the user.
	// TODO potentially allow the password to be specified in
	// the config file or as a command line flag or interactively?
	password, err := utils.RandomPassword()
	if err != nil {
		return errgo.Notef(err, "cannot generate password")
	}
	return writeEnvironment(c.localName, func() (*params.EnvironmentResponse, error) {
		return client.NewEnvironment(&params.NewEnvironment{
			User: c.envPath.User,
			Info: params.NewEnvironmentInfo{
				Name:          c.envPath.Name,
				Password:      password,
				StateServer:   c.srvPath.EntityPath,
				Config:        config,
				TemplatePaths: c.templatePaths.paths,
			},
		})
	})
}

type schemaContext struct {
	envName      params.Name
	providerType string
	knownAttrs   map[string]interface{}
}

func (ctxt schemaContext) generateConfig(schema environschema.Fields) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	for name, attr := range schema {
		checker, err := attr.Checker()
		if err != nil {
			return nil, errgo.Notef(err, "invalid attribute %q", name)
		}
		val, err := ctxt.getDefault(form.NamedAttr{
			Name: name,
			Attr: attr,
		}, checker)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		if val == nil && attr.Mandatory {
			return nil, errgo.Newf("no value found for mandatory attribute %q", name)
		}
		if val != nil {
			config[name] = val
		}
	}
	return config, nil
}

// getDefault gets the default value for the given attribute.
// If there is no default value found, it returns (nil, nil)
func (ctxt schemaContext) getDefault(attr form.NamedAttr, checker schema.Checker) (val interface{}, err error) {
	val, from, err := ctxt.getDefault1(attr, checker)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get value for %q", attr.Name)
	}
	if val == nil {
		return nil, nil
	}
	val, err = checker.Coerce(val, nil)
	if err != nil {
		return nil, errgo.Notef(err, "bad value for %q in %s", attr.Name, from)
	}
	return val, nil
}

// getDefault1 is the internal version of getDefault. It does
// not coerce the returned value through the checker,
// and it also returns the source of the value so that
// that can be included in an error message if the value
// is erroneous.
func (ctxt schemaContext) getDefault1(attr form.NamedAttr, checker schema.Checker) (val interface{}, from string, err error) {
	if val, ok := ctxt.knownAttrs[attr.Name]; ok {
		return val, "attributes", nil
	}
	if attr.Name == "authorized-keys" {
		path, _ := ctxt.knownAttrs["authorized-keys-path"].(string)
		keys, err := jujuconfig.ReadAuthorizedKeys(path)
		if err != nil {
			return nil, "", errgo.Notef(err, "cannot read authorized keys")
		}
		display := path
		if display == "" {
			display = "default authorized keys paths"
		}
		return keys, "authorized keys file", nil
	}
	path, _ := ctxt.knownAttrs[attr.Name+"-path"].(string)
	if path != "" && attr.Type == environschema.Tstring {
		// Any string configuration attribute may be specified
		// with a -path attribute.
		val, path, err := readFile(path)
		if err != nil {
			return nil, path, errgo.Mask(err)
		}
		return val, path, nil
	}
	// TODO it could be a problem that this potentially
	// enables a rogue JEM server to retrieve arbitrary
	// environment variables from a client. Implement
	// some kind of whitelisting scheme?
	val, _, err = form.DefaultFromEnv(attr, checker)
	if err != nil {
		return val, "", errgo.Mask(err)
	}
	if val != nil {
		return val, "", nil
	}
	f := providerDefaults[ctxt.providerType][attr.Name]
	if f == nil {
		logger.Infof("none found")
		return nil, "", nil
	}
	v, err := f(ctxt)
	if err != nil {
		return nil, "", errgo.Mask(err)
	}
	return v, "provider defaults", nil
}

// readFile reads an attribute from the given file path.
// If the path is relative, then it is treated as releative
// to $JUJU_HOME. Also, an initial "~" is expanded to $HOME.
func readFile(path string) (val string, finalPath string, err error) {
	// The logic here is largely copied from the
	// maybeReadAttrFromFile function in juju/environs/config.
	finalPath, err = utils.NormalizePath(path)
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(finalPath) {
		if !osenv.IsJujuHomeSet() {
			return "", "", errgo.Newf("JUJU_HOME not set, not attempting to read file %q", finalPath)
		}
		finalPath = osenv.JujuHomePath(finalPath)
	}
	data, err := ioutil.ReadFile(finalPath)
	if err != nil {
		return "", "", errgo.Mask(err)
	}
	if len(data) == 0 {
		return "", "", fmt.Errorf("file %q is empty", finalPath)
	}
	return string(data), finalPath, nil
}

var providerDefaults = map[string]map[string]func(schemaContext) (interface{}, error){
	"azure": {
		"availability-sets-enabled": constValue(true),
	},
	"ec2": {
		"control-bucket": rawUUIDValue,
	},
	"joyent": {
		"control-dir": uuidValue,
	},
	"local": {
		"namespace": localProviderNamespaceValue,
		"proxy-ssh": constValue(false),
	},
	"maas": {
		"maas-agent-name": uuidValue,
	},
	"openstack": {
		"control-bucket": rawUUIDValue,
	},
}

func constValue(v interface{}) func(schemaContext) (interface{}, error) {
	return func(schemaContext) (interface{}, error) {
		return v, nil
	}
}

func uuidValue(schemaContext) (interface{}, error) {
	return utils.NewUUID()
}

func rawUUIDValue(schemaContext) (interface{}, error) {
	v, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("%x", v.Raw()), nil
}

func localProviderNamespaceValue(ctxt schemaContext) (interface{}, error) {
	if ctxt.envName == "" {
		return nil, errgo.Newf("no default value for local provider namespace attribute")
	}
	username := os.Getenv("USER")
	if username == "" {
		u, err := user.Current()
		if err != nil {
			return nil, errgo.Notef(err, "failed to determine username for namespace")
		}
		username = u.Username
	}
	return fmt.Sprintf("%s-%s", username, ctxt.envName), nil
}
