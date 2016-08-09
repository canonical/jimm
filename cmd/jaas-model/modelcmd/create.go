// Copyright 2015-2016 Canonical Ltd.

package modelcmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"github.com/juju/utils/keyvalues"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/environschema.v1/form"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

type createCommand struct {
	commandBase

	ctlPath        entityPathValue
	modelPath      entityPathValue
	configFile     string
	localName      string
	location       map[string]string
	credentialName string
}

func newCreateCommand() cmd.Command {
	return modelcmd.WrapBase(&createCommand{})
}

var createDoc = `
The create command creates a new model inside the specified controller.
Its argument specifies the server name of the new model.

Any provided key-value arguments are used to select the location
of the controller that will run the model. For example:

	jaas model create me/mymodel cloud=aws region=us-east-1
`

func (c *createCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<user>/<modelname> [<key>=<value> ...]",
		Purpose: "Create a new model on managing server ",
		Doc:     createDoc,
	}
}

func (c *createCommand) SetFlags(f *gnuflag.FlagSet) {
	c.commandBase.SetFlags(f)
	f.Var(&c.ctlPath, "controller", "")
	f.Var(&c.ctlPath, "c", "controller to create the model in")
	f.StringVar(&c.configFile, "config", "", "YAML config file containing model configuration")
	f.StringVar(&c.localName, "local", "", "local name for model (as used for juju switch). Defaults to <modelname>")
	f.StringVar(&c.credentialName, "credential", "", "name of the credential to use to create the model")
}

func (c *createCommand) Init(args []string) error {
	if len(args) < 1 {
		return errgo.Newf("missing model name argument")
	}
	if err := c.modelPath.Set(args[0]); err != nil {
		return errgo.Mask(err)
	}
	if !c.ctlPath.EntityPath.IsZero() && len(args) > 1 {
		return errgo.Newf("cannot specify explicit controller name with location")
	}
	if c.localName == "" {
		c.localName = string(c.modelPath.Name)
	}
	attrs, err := keyvalues.Parse(args[1:], false)
	if err != nil {
		return errgo.Mask(err)
	}
	c.location = attrs
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
	providerType, schema, err := c.providerSchema(client)
	if err != nil {
		return errgo.Mask(err)
	}
	defaultsCtxt := schemaContext{
		modelName:    c.modelPath.Name,
		providerType: providerType,
		knownAttrs:   config,
		cmdContext:   ctxt,
	}
	config, err = defaultsCtxt.generateConfig(schema)
	if err != nil {
		return errgo.Notef(err, "invalid configuration")
	}
	var localCtlName string
	if !c.ctlPath.IsZero() {
		localCtlName = jemControllerToLocalControllerName(c.ctlPath.EntityPath)
	}
	switchName, err := writeModel(c.localName, localCtlName, func() (*params.ModelResponse, error) {
		var ctlPath *params.EntityPath
		if !c.ctlPath.EntityPath.IsZero() {
			ctlPath = &c.ctlPath.EntityPath
		}
		return client.NewModel(&params.NewModel{
			User: c.modelPath.User,
			Info: params.NewModelInfo{
				Name:       c.modelPath.Name,
				Controller: ctlPath,
				Credential: params.Name(c.credentialName),
				Config:     config,
				Location:   c.location,
			},
		})
	})
	if err != nil {
		return errgo.Mask(err)
	}
	ctxt.Infof("%s", switchName)
	return nil
}

// providerSchema returns the provider type and schema for the new model.
func (c *createCommand) providerSchema(client *jemclient.Client) (string, environschema.Fields, error) {
	if c.ctlPath.IsZero() {
		// No controller specified - get the schema from the location.
		info, err := client.GetSchema(&params.GetSchema{
			Location: c.location,
		})
		if err != nil {
			return "", nil, errgo.Notef(err, "cannot get schema info")
		}
		return info.ProviderType, info.Schema, nil
	}

	info, err := client.GetController(&params.GetController{
		EntityPath: c.ctlPath.EntityPath,
	})
	if err != nil {
		return "", nil, errgo.Notef(err, "cannot get Controller info")
	}
	return info.ProviderType, info.Schema, nil
}

type schemaContext struct {
	modelName    params.Name
	providerType string
	knownAttrs   map[string]interface{}
	cmdContext   *cmd.Context
}

func (ctxt schemaContext) generateConfig(schema environschema.Fields) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	for name, attr := range schema {
		val, err := ctxt.getVal(form.NamedAttr{
			Name: name,
			Attr: attr,
		})
		if err != nil {
			return nil, errgo.Mask(err)
		}
		if val != nil {
			config[name] = val
		}
	}
	return config, nil
}

// getVal gets the value for the given attribute.
// If there is no known value or default value found, it returns (nil, nil).
func (ctxt schemaContext) getVal(attr form.NamedAttr) (val interface{}, err error) {
	checker, err := attr.Checker()
	if err != nil {
		return nil, errgo.Notef(err, "invalid attribute %q", attr.Name)
	}
	val, from, err := ctxt.getVal1(attr, checker)
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

// getVal1 is the internal version of getVal. It does
// not coerce the returned value through the checker,
// and it also returns the source of the value so that
// that can be included in an error message if the value
// is erroneous.
func (ctxt schemaContext) getVal1(attr form.NamedAttr, checker schema.Checker) (val interface{}, from string, err error) {
	if val, ok := ctxt.knownAttrs[attr.Name]; ok {
		return val, "attributes", nil
	}
	if attr.Name == "authorized-keys" {
		path, _ := ctxt.knownAttrs["authorized-keys-path"].(string)
		keys, err := common.ReadAuthorizedKeys(ctxt.cmdContext, path)
		if err != nil {
			if errgo.Cause(err) == common.ErrNoAuthorizedKeys {
				return nil, "", nil
			}
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
	// enables a rogue JEM controller to retrieve arbitrary
	// model variables from a client. Implement
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
		if !osenv.IsJujuXDGDataHomeSet() {
			return "", "", errgo.Newf("JUJU_HOME not set, not attempting to read file %q", finalPath)
		}
		finalPath = osenv.JujuXDGDataHomePath(finalPath)
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
	if ctxt.modelName == "" {
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
	return fmt.Sprintf("%s-%s", username, ctxt.modelName), nil
}
