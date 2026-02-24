// Copyright 2026 Canonical.

package cmd

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/canonical/lxd/shared/logger"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/api"
	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"

	jimmapi "github.com/canonical/jimm/v3/pkg/api"
	jimmapiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type AddModelCloudAPI interface {
	AddCredential(tag string, credential jujucloud.Credential) error
	Cloud(names.CloudTag) (jujucloud.Cloud, error)
	UserCredentials(names.UserTag, names.CloudTag) ([]names.CloudCredentialTag, error)
}

// NewAddModelCommand returns a command to add a model.
func NewAddModelCommand() cmd.Command {
	command := &addModelCommand{
		jimmAPIFunc: func(root api.Connection) JIMMAPI {
			return jimmapi.NewClient(root)
		},
		cloudAPIFunc: func(root api.Connection) AddModelCloudAPI {
			return cloudapi.NewClient(root)
		},
	}
	command.CanClearCurrentModel = true
	return modelcmd.WrapController(command)
}

// addModelCommand calls the API to add a new model.
type addModelCommand struct {
	modelcmd.ControllerCommandBase
	jimmAPIFunc  func(api.Connection) JIMMAPI
	cloudAPIFunc func(api.Connection) AddModelCloudAPI

	modelOwner  string
	apiRoot     api.Connection
	jimmClient  JIMMAPI
	cloudClient AddModelCloudAPI

	Name             string
	Owner            string
	CredentialName   string
	CloudRegion      string
	Config           common.ConfigFlag
	noSwitch         bool
	targetController string
}

const addModelHelpDoc = `
Adds a model to a specific controller.

This command creates a new hosted model on the specified controller.
`

const addModelHelpExamples = `
    juju [jaas] add-model mymodel mycloud --target-controller jaas-controller-1
    juju [jaas] add-model mymodel us-east-1 --target-controller jaas-controller-1
	juju [jaas] add-model mymodel aws/us-east-1 --target-controller jaas-controller-2 --credential mycred
    juju [jaas] add-model mymodel --target-controller jaas-controller-3 --config key=value
`

// Info returns the command info.
func (c *addModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-model",
		Args:     "<model name> [cloud|region|(cloud/region)]",
		Purpose:  "Adds a model to a specific controller.",
		Doc:      strings.TrimSpace(addModelHelpDoc),
		Examples: addModelHelpExamples,
	})
}

// SetFlags sets the command flags.
func (c *addModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.StringVar(&c.Owner, "owner", "", "Specify the user who will own the model, if not the current user")
	f.StringVar(&c.CredentialName, "credential", "", "Specify the credential to be used by the model")
	f.Var(&c.Config, "config", "Specify the path to a YAML model configuration file or individual configuration options (`--config config.yaml [--config key=value ...]`)")
	f.BoolVar(&c.noSwitch, "no-switch", false, "Choose not to switch to the newly created model")
	f.StringVar(&c.targetController, "target-controller", "", "Target controller for the model")
}

// Init initializes the command with the given arguments.
func (c *addModelCommand) Init(args []string) error {
	if len(args) == 0 {
		return common.MissingModelNameError("add-model")
	}
	c.Name, args = args[0], args[1:]

	if len(args) > 0 {
		c.CloudRegion, args = args[0], args[1:]
	}

	if !names.IsValidModelName(c.Name) {
		return fmt.Errorf("%q is not a valid name: model names may only contain lowercase letters, digits and hyphens", c.Name)
	}

	if c.Owner != "" && !names.IsValidUser(c.Owner) {
		return fmt.Errorf("%q is not a valid user", c.Owner)
	}

	if c.targetController == "" {
		return fmt.Errorf("target controller not specified")
	}

	return cmd.CheckEmpty(args)
}

func (c *addModelCommand) newAPIRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewAPIRoot()
}

// Run executes the add-model command.
//
//nolint:gocognit
func (c *addModelCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return err
	}

	root, err := c.newAPIRoot()
	if err != nil {
		return fmt.Errorf("opening API connection: %w", err)
	}
	defer root.Close()

	c.jimmClient = c.jimmAPIFunc(root)
	c.cloudClient = c.cloudAPIFunc(root)

	store := c.ClientStore()
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil {
		return err
	}

	c.modelOwner = accountDetails.User
	if c.Owner != "" {
		c.modelOwner = c.Owner
	}

	var cloudTag names.CloudTag
	var cloud jujucloud.Cloud
	var cloudRegion string
	if c.CloudRegion != "" {
		cloudTag, cloud, cloudRegion, err = c.getCloudRegion()
		if err != nil {
			logger.Errorf("%v", err)
			ctx.Infof("Use 'juju clouds' to see a list of all available clouds or 'juju add-cloud' to a add one.")
			return cmd.ErrSilent
		}
	} else {
		if cloudTag, cloud, err = c.getCloudsModelOwnerCanAccess(); err != nil {
			return errors.Trace(err)
		}
	}

	// Find a local credential to use with the new model.
	// If credential was found on the controller, it will be nil in return.
	credential, credentialTag, credentialRegion, err := c.findCredential(ctx, &findCredentialParams{
		cloudTag:    cloudTag,
		cloudRegion: cloudRegion,
		cloud:       cloud,
		modelOwner:  c.modelOwner,
	})
	if err != nil {
		logger.Errorf("%v", err)
		ctx.Infof("Use \n* 'juju add-credential -c' to upload a credential to a controller or\n" +
			"* 'juju autoload-credentials' to add credentials from local files or\n" +
			"* 'juju add-model --credential' to use a local credential.\n" +
			"Use 'juju credentials' to list all available credentials.\n")
		return cmd.ErrSilent
	}

	// If the user has not specified an explicit cloud region,
	// use any default region from the credential.
	if cloudRegion == "" {
		cloudRegion = credentialRegion
	}

	// Upload the credential if it was explicitly set and we have found it locally.
	if c.CredentialName != "" && credential != nil {
		ctx.Infof("Uploading credential '%s' to controller", credentialTag.Id())
		if err := c.cloudClient.AddCredential(credentialTag.String(), *credential); err != nil {
			ctx.Infof("Failed to upload credential: %v", err)
			return cmd.ErrSilent
		}
	}

	modelConfig, err := c.getConfigValues(ctx)
	if err != nil {
		return err
	}

	// Here is the main difference between the juju add-model command
	// and the jimm add-model command where we call the JIMM facade
	// method to add model to a specific controller.
	req := &jimmapiparams.AddModelToControllerRequest{
		ModelCreateArgs: params.ModelCreateArgs{
			Name:               c.Name,
			OwnerTag:           names.NewUserTag(c.modelOwner).String(),
			CloudTag:           cloudTag.String(),
			CloudRegion:        cloudRegion,
			CloudCredentialTag: credentialTag.String(),
			Config:             modelConfig,
		},
		ControllerName: c.targetController,
	}

	model, err := c.jimmClient.AddModelToController(req)
	if err != nil {
		if strings.HasPrefix(errors.Cause(err).Error(), "getting credential") {
			err = errors.NewNotFound(nil,
				fmt.Sprintf("%v\nSee `juju add-credential %s --help` for instructions", err, cloudTag.Id()))
			return errors.Trace(err)
		}
		err = params.TranslateWellKnownError(err)
		switch {
		case errors.Is(err, errors.Unauthorized):
			common.PermissionsMessage(ctx.Stderr, "add a model")
		case errors.Is(err, errors.NotValid) && cloud.Type == "kubernetes":
			// Workaround for https://bugs.launchpad.net/juju/+bug/1994454
			return fmt.Errorf("cannot create model %[1]q: a namespace called %[1]q already exists on this k8s cluster. Please pick a different model name", c.Name)
		}
		return err
	}

	messageFormat := "Added '%s' model"
	messageArgs := []interface{}{c.Name}

	details := jujuclient.ModelDetails{
		ModelUUID: model.UUID,
		ModelType: coremodel.ModelType(model.Type),
	}

	if c.modelOwner == accountDetails.User {
		modelName := fmt.Sprintf("%s/%s", c.modelOwner, model.Name)

		if err := store.UpdateModel(controllerName, modelName, details); err != nil {
			return err
		}
		if !c.noSwitch {
			if err := store.SetCurrentModel(controllerName, modelName); err != nil {
				return err
			}
		}
	}

	if c.CloudRegion != "" || model.CloudRegion != "" {
		// The user explicitly requested a cloud/region,
		// or the cloud supports multiple regions. Whichever
		// the case, tell the user which cloud/region the
		// model was deployed to.
		cloudRegion := model.CloudTag
		if model.CloudRegion != "" {
			cloudRegion += "/" + model.CloudRegion
		}
		messageFormat += " on %s"
		messageArgs = append(messageArgs, cloudRegion)
	}
	if model.CloudCredentialTag != "" {
		tag, err := names.ParseCloudCredentialTag(model.CloudCredentialTag)
		if err != nil {
			return errors.NotValidf("cloud credential tag %q", model.CloudCredentialTag)
		}
		credentialName := tag.Name()
		if tag.Owner().Id() != c.modelOwner {
			credentialName = fmt.Sprintf("%s/%s", tag.Owner().Id(), credentialName)
		}
		messageFormat += " with credential '%s'"
		messageArgs = append(messageArgs, credentialName)
	}

	messageFormat += fmt.Sprintf(" for user '%s'", names.NewUserTag(c.modelOwner).Name())

	// "Added '<model>' model [on <cloud>/<region>] [with credential '<credential>'] for user '<user namePart>'"
	ctx.Infof(messageFormat, messageArgs...)

	if _, ok := modelConfig[config.AuthorizedKeysKey]; !ok {
		// It is not an error to have no authorized-keys when adding a
		// model, though this should never happen since we generate
		// juju-specific SSH keys.
		ctx.Infof(`
No SSH authorized-keys were found. You must use "juju add-ssh-key"
before "juju ssh", "juju scp", or "juju debug-hooks" will work.`)
	}

	return nil
}

func (c *addModelCommand) getCloudRegion() (cloudTag names.CloudTag, cloud jujucloud.Cloud, cloudRegion string, err error) {
	fail := func(err error) (names.CloudTag, jujucloud.Cloud, string, error) {
		return names.CloudTag{}, jujucloud.Cloud{}, "", err
	}

	var cloudName string
	sep := strings.IndexRune(c.CloudRegion, '/')
	if sep >= 0 {
		// User specified "cloud/region".
		cloudName, cloudRegion = c.CloudRegion[:sep], c.CloudRegion[sep+1:]
		if !names.IsValidCloud(cloudName) {
			return fail(fmt.Errorf("invalid cloud name %q", cloudName))
		}
		cloudTag = names.NewCloudTag(cloudName)
		if cloud, err = c.cloudClient.Cloud(cloudTag); err != nil {
			return fail(err)
		}
	} else {
		// User specified "cloud" or "region". We'll try first
		// for cloud (check if it's a valid cloud name, and
		// whether there is a cloud by that name), and then
		// as a region within the default cloud.
		if names.IsValidCloud(c.CloudRegion) {
			cloudName = c.CloudRegion
		} else {
			cloudRegion = c.CloudRegion
		}
		if cloudName != "" {
			cloudTag = names.NewCloudTag(cloudName)
			cloud, err = c.cloudClient.Cloud(cloudTag)
			if params.IsCodeNotFound(err) {
				// No such cloud with the specified name,
				// so we'll try the name as a region in
				// the default cloud.
				cloudRegion, cloudName = cloudName, ""
			} else if err != nil {
				return fail(err)
			}
		}

		if cloudName == "" {
			cloudTag, cloud, err = c.getCloudsModelOwnerCanAccess()
			if err != nil {
				return fail(errors.Trace(err))
			}
		}
	}
	if cloudRegion != "" {
		// A region has been specified, make sure it exists.
		if _, err := jujucloud.RegionByName(cloud.Regions, cloudRegion); err != nil {
			if cloudRegion == c.CloudRegion {
				// The string is not in the format cloud/region,
				// so we should tell that the user that it is
				// neither a cloud nor a region in the
				// controller's cloud.
				clouds, err := c.jimmClient.ListUserClouds(&jimmapiparams.ListUserCloudsRequest{
					UserTag: names.NewUserTag(c.modelOwner).String(),
				})
				if err != nil {
					return fail(fmt.Errorf("querying supported clouds: %w", err))
				}
				return fail(unsupportedCloudOrRegionError(clouds, c.CloudRegion))
			}
			return fail(err)
		}
	}
	return cloudTag, cloud, cloudRegion, nil
}

func unsupportedCloudOrRegionError(clouds map[names.CloudTag]jujucloud.Cloud, cloudRegion string) (err error) {
	cloudNames := make([]string, 0, len(clouds))
	for tag := range clouds {
		cloudNames = append(cloudNames, tag.Id())
	}
	sort.Strings(cloudNames)

	var buf bytes.Buffer
	tw := output.TabWriter(&buf)
	fmt.Fprintln(tw, "Cloud\tRegions")
	for _, cloudName := range cloudNames {
		cloud := clouds[names.NewCloudTag(cloudName)]
		regionNames := make([]string, len(cloud.Regions))
		for i, region := range cloud.Regions {
			regionNames[i] = region.Name
		}
		fmt.Fprintf(tw, "%s\t%s\n", cloudName, strings.Join(regionNames, ", "))
	}
	tw.Flush()

	var prefix string
	switch len(clouds) {
	case 0:
		return fmt.Errorf(`
you do not have add-model access to any clouds on this controller.
Please ask the controller administrator to grant you add-model permission
for a particular cloud to which you want to add a model`)
	case 1:
		for cloudTag := range clouds {
			prefix = fmt.Sprintf(`
%q is neither a cloud supported by this controller,
nor a region in the controller's default cloud %q.`[1:],
				cloudRegion, cloudTag.Id())
		}
	default:
		prefix = `
this controller manages more than one cloud.
Please specify which cloud/region to use:

    juju add-model [options] <model-name> cloud[/region]
`[1:]
	}
	return fmt.Errorf("%s\nThe clouds/regions supported by this controller are:\n\n%s", prefix, buf.String())
}

var errAmbiguousDetectedCredential = fmt.Errorf(`
more than one credential detected. Add all detected credentials
to the client with:

    juju autoload-credentials

and then run the add-model command again with the --credential option`,
)

var errAmbiguousCredential = fmt.Errorf(`
more than one credential is available. List credentials with:

    juju credentials

and then run the add-model command again with the --credential option`,
)

type findCredentialParams struct {
	cloudTag    names.CloudTag
	cloud       jujucloud.Cloud
	cloudRegion string
	modelOwner  string
}

// findCredential finds a suitable credential to use for the new model.
// The credential will first be searched for locally and then on the
// controller. If a credential is found locally then it's value will be
// returned as the first return value. If it is found on the controller
// this will be nil as there is no need to upload it in that case.
func (c *addModelCommand) findCredential(ctx *cmd.Context, p *findCredentialParams) (_ *jujucloud.Credential, _ names.CloudCredentialTag, cloudRegion string, _ error) {
	if c.CredentialName == "" {
		return c.findUnspecifiedCredential(ctx, p)
	}
	return c.findSpecifiedCredential(ctx, p)
}

func (c *addModelCommand) findUnspecifiedCredential(ctx *cmd.Context, p *findCredentialParams) (_ *jujucloud.Credential, _ names.CloudCredentialTag, cloudRegion string, _ error) {
	fail := func(err error) (*jujucloud.Credential, names.CloudCredentialTag, string, error) {
		return nil, names.CloudCredentialTag{}, "", err
	}
	// If the user has not specified a credential, and the cloud advertises
	// itself as supporting the "empty" auth-type, then return immediately.
	for _, authType := range p.cloud.AuthTypes {
		if authType == jujucloud.EmptyAuthType {
			return nil, names.CloudCredentialTag{}, p.cloudRegion, nil
		}
	}

	if p.cloudTag.Id() == "" {
		return nil, names.CloudCredentialTag{}, p.cloudRegion, nil
	}

	// No credential has been specified, so see if there is one already on the controller we can use.
	modelOwnerTag := names.NewUserTag(p.modelOwner)
	credentialTags, err := c.cloudClient.UserCredentials(modelOwnerTag, p.cloudTag)
	if err != nil {
		return nil, names.CloudCredentialTag{}, p.cloudRegion, err
	}
	var credentialTag names.CloudCredentialTag
	if len(credentialTags) == 1 {
		credentialTag = credentialTags[0]
	}

	if (credentialTag != names.CloudCredentialTag{}) {
		// If the controller already has a credential, see if
		// there is a local version that has an associated
		// region.
		credential, _, cloudRegion, err := c.findLocalCredential(ctx, p, credentialTag.Name())
		if errors.Is(err, errors.NotFound) {
			// No local credential; use the region
			// specified by the user, if any.
			cloudRegion = p.cloudRegion
		} else if err != nil {
			return fail(err)
		}
		// If there is a credential in the controller use it even if we don't have a local version.
		return credential, credentialTag, cloudRegion, nil
	}
	// There is not a default credential on the controller (either
	// there are no credentials, or there is more than one). Look for
	// a local credential we might use.
	credential, credentialName, cloudRegion, err := c.findLocalCredential(ctx, p, "")
	if err != nil {
		return fail(err)
	}
	// We've got a local credential to use.
	credentialTag, err = common.ResolveCloudCredentialTag(
		modelOwnerTag, p.cloudTag, credentialName,
	)
	if err != nil {
		return fail(err)
	}
	return credential, credentialTag, cloudRegion, nil
}

func (c *addModelCommand) findSpecifiedCredential(ctx *cmd.Context, p *findCredentialParams) (_ *jujucloud.Credential, _ names.CloudCredentialTag, cloudRegion string, _ error) {
	fail := func(err error) (*jujucloud.Credential, names.CloudCredentialTag, string, error) {
		return nil, names.CloudCredentialTag{}, "", err
	}
	// Look for a local credential with the specified name
	credential, credentialName, cloudRegion, err := c.findLocalCredential(ctx, p, c.CredentialName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return fail(err)
	}
	if credential != nil {
		// We found a local credential with the specified name.
		modelOwnerTag := names.NewUserTag(p.modelOwner)
		credentialTag, err := common.ResolveCloudCredentialTag(
			modelOwnerTag, p.cloudTag, credentialName,
		)
		if err != nil {
			return fail(err)
		}
		return credential, credentialTag, cloudRegion, nil
	}

	// There was no local credential with that name, check the controller
	modelOwnerTag := names.NewUserTag(p.modelOwner)
	credentialTags, err := c.cloudClient.UserCredentials(modelOwnerTag, p.cloudTag)
	if err != nil {
		return fail(err)
	}
	credentialTag, err := common.ResolveCloudCredentialTag(
		modelOwnerTag, p.cloudTag, c.CredentialName,
	)
	if err != nil {
		return fail(err)
	}
	credentialId := credentialTag.Id()
	for _, tag := range credentialTags {
		if tag.Id() != credentialId {
			continue
		}
		ctx.Infof("Using credential '%s' cached in controller", c.CredentialName)
		return nil, credentialTag, "", nil
	}
	// Cannot find a credential with the correct name
	return fail(fmt.Errorf("credential '%s' not found", c.CredentialName))
}

func (c *addModelCommand) findLocalCredential(ctx *cmd.Context, p *findCredentialParams, name string) (_ *jujucloud.Credential, credentialName, cloudRegion string, _ error) {
	fail := func(err error) (*jujucloud.Credential, string, string, error) {
		return nil, "", "", err
	}
	provider, err := environs.GlobalProviderRegistry().Provider(p.cloud.Type)
	if err != nil {
		return fail(err)
	}
	credential, credentialName, cloudRegion, _, err := common.GetOrDetectCredential(
		ctx, c.ClientStore(), provider, modelcmd.GetCredentialsParams{
			Cloud:          p.cloud,
			CloudRegion:    p.cloudRegion,
			CredentialName: name,
		},
	)
	if err == nil {
		return credential, credentialName, cloudRegion, nil
	}
	switch errors.Cause(err) {
	case modelcmd.ErrMultipleCredentials:
		return fail(errAmbiguousCredential)
	case common.ErrMultipleDetectedCredentials:
		return fail(errAmbiguousDetectedCredential)
	}
	return fail(err)
}

func (c *addModelCommand) getConfigValues(ctx *cmd.Context) (map[string]interface{}, error) {
	configValues, err := c.Config.ReadAttrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}
	coercedValues, err := common.ConformYAML(configValues)
	if err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}
	attrs, ok := coercedValues.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("params must contain a YAML map with string keys")
	}
	if err := common.FinalizeAuthorizedKeys(ctx, attrs); err != nil {
		if errors.Cause(err) != common.ErrNoAuthorizedKeys {
			return nil, err
		}
	}
	if _, ok := attrs[config.DefaultSeriesKey]; ok {
		if _, ok := attrs[config.DefaultBaseKey]; ok {
			return nil, fmt.Errorf("cannot specify both default-series and default-base")
		}
	}
	return attrs, nil
}

func (c *addModelCommand) getCloudsModelOwnerCanAccess() (names.CloudTag, jujucloud.Cloud, error) {
	clouds, err := c.jimmClient.ListUserClouds(&jimmapiparams.ListUserCloudsRequest{
		UserTag: names.NewUserTag(c.modelOwner).String(),
	})
	if err != nil {
		return names.CloudTag{}, jujucloud.Cloud{}, errors.Trace(err)
	}
	if len(clouds) != 1 {
		return names.CloudTag{}, jujucloud.Cloud{}, unsupportedCloudOrRegionError(clouds, "")
	}
	for cloudTag, cloud := range clouds {
		return cloudTag, cloud, nil
	}
	panic("unreachable")
}
