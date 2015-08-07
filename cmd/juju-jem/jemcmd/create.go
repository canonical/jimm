// Copyright 2015 Canonical Ltd.

package jemcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	jujuconfig "github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/params"
)

type createCommand struct {
	commandBase

	srvPath    entityPathValue
	envPath    entityPathValue
	configFile string
	localName  string
}

var createDoc = `
The create command creates a new environment inside the specified state
server. Its argument specifies the JEM name of the new environment.
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
	f.Var(&c.srvPath, "state-server", "state server to create the environment in")
	f.Var(&c.srvPath, "s", "")
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
	client, err := c.newClient()
	if err != nil {
		return errgo.Mask(err)
	}
	defer client.Close()

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
	defaultsCtxt := providerDefaultsContext{
		envName: c.envPath.Name,
	}
	if err := setConfigDefaults(config, jesInfo, defaultsCtxt); err != nil {
		return errgo.Notef(err, "cannot add default values for configuration file")
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
				Name:        c.envPath.Name,
				Password:    password,
				StateServer: c.srvPath.EntityPath,
				Config:      config,
			},
		})
	})
}

func setConfigDefaults(config map[string]interface{}, jesInfo *params.JESResponse, ctxt providerDefaultsContext) error {
	// Authorized keys are special because the path is relative
	// to $HOME/.ssh by default.
	if _, ok := jesInfo.Schema["authorized-keys"]; ok && config["authorized-keys"] == nil {
		// Load authorized-keys-path into authorized-keys if necessary.
		path, _ := config["authorized-keys-path"].(string)
		keys, err := jujuconfig.ReadAuthorizedKeys(path)
		if err != nil {
			return errgo.Notef(err, "cannot read authorized keys")
		}
		config["authorized-keys"] = keys
		delete(config, "authorized-keys-path")
	}

	// Any string configuration attribute may be specified
	// with a -path attribute.
	for pathAttr, path := range config {
		if !strings.HasSuffix(pathAttr, "-path") {
			continue
		}
		attr := strings.TrimSuffix(pathAttr, "-path")
		field, ok := jesInfo.Schema[attr]
		if !ok || field.Type != environschema.Tstring {
			continue
		}
		pathStr, ok := path.(string)
		if !ok || pathStr == "" {
			// Probably just malformed - let the server deal with it.
			continue
		}
		delete(config, pathAttr)
		val, err := readFile(pathStr)
		if err != nil {
			return errgo.Notef(err, "cannot get value for %q", pathStr)
		}
		config[attr] = val
	}

	// Fill config attributes from appropriate environment variables
	for name, attr := range jesInfo.Schema {
		if config[name] != nil {
			continue
		}
		if attr.EnvVar != "" {
			// TODO it could be a problem that this potentially
			// enables a rogue JEM server to retrieve arbitrary
			// environment variables from a client. Implement
			// some kind of whitelisting scheme?
			if v := os.Getenv(attr.EnvVar); v != "" {
				config[name] = v
				continue
			}
		}
		if f := providerDefaults[jesInfo.ProviderType][name]; f != nil {
			v, err := f(ctxt)
			if err != nil {
				return errgo.Notef(err, "cannot make default value for %q", name)
			}
			config[name] = v
		}
	}
	return nil
}

// readFile reads an attribute from the given file path.
// If the path is relative, then it is treated as releative
// to $JUJU_HOME. Also, an initial "~" is expanded to $HOME.
func readFile(path string) (string, error) {
	// The logic here is largely copied from the
	// maybeReadAttrFromFile function in juju/environs/config.
	path, err := utils.NormalizePath(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(path) {
		if !osenv.IsJujuHomeSet() {
			return "", errgo.Newf("JUJU_HOME not set, not attempting to read file %q", path)
		}
		path = osenv.JujuHomePath(path)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errgo.Mask(err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("file %q is empty", path)
	}
	return string(data), nil
}

type providerDefaultsContext struct {
	envName params.Name
}

var providerDefaults = map[string]map[string]func(providerDefaultsContext) (interface{}, error){
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

func constValue(v interface{}) func(providerDefaultsContext) (interface{}, error) {
	return func(providerDefaultsContext) (interface{}, error) {
		return v, nil
	}
}

func uuidValue(providerDefaultsContext) (interface{}, error) {
	return utils.NewUUID()
}

func rawUUIDValue(providerDefaultsContext) (interface{}, error) {
	v, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	return fmt.Sprintf("%x", v.Raw()), nil
}

func localProviderNamespaceValue(ctxt providerDefaultsContext) (interface{}, error) {
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
