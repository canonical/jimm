// Copyright 2026 Canonical.

package cmd

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"sigs.k8s.io/yaml"

	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	addControllerProfileDoc = `
Adds a controller profile to JIMM.

The controller profile definition is read from a YAML file or from stdin.
`
	addControllerProfileExample = `
    juju jaas add-controller-profile my-profile --file ./profile.yaml
    cat profile.yaml | juju jaas add-controller-profile my-profile --file -
`

	updateControllerProfileDoc = `
Updates a saved controller profile in JIMM.

The controller profile definition is read from a YAML file or from stdin.
If the provided profile does not specify a version, the current version is
retrieved before saving.
`
	updateControllerProfileExample = `
    juju jaas update-controller-profile my-profile --file ./profile.yaml
    cat profile.yaml | juju jaas update-controller-profile my-profile --file -
`

	showControllerProfileDoc = `
Shows a saved controller profile from JIMM.
`
	showControllerProfileExample = `
    juju jaas show-controller-profile my-profile
    juju jaas show-controller-profile my-profile --format json
`

	listControllerProfilesDoc = `
Lists saved controller profiles in JIMM.
`
	listControllerProfilesExample = `
    juju jaas list-controller-profiles
    juju jaas list-controller-profiles --juju-version 3.6.4
`

	removeControllerProfileDoc = `
Removes a saved controller profile from JIMM.
`
	removeControllerProfileExample = `
    juju jaas remove-controller-profile my-profile
    juju jaas remove-controller-profile my-profile --force
`
)

// NewAddControllerProfileCommand returns a command to add a controller profile.
func NewAddControllerProfileCommand() cmd.Command {
	cmd := &addControllerProfileCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// addControllerProfileCommand adds a saved controller profile.
type addControllerProfileCommand struct {
	jaasCommandBase
	out cmd.Output

	file cmd.FileVar
	name string
}

// Info implements cmd.Command.
func (c *addControllerProfileCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-controller-profile",
		Args:     "<name>",
		Purpose:  "Add a saved controller profile to jimm.",
		Doc:      addControllerProfileDoc,
		Examples: addControllerProfileExample,
	})
}

// SetFlags implements cmd.Command.
func (c *addControllerProfileCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	c.file.StdinMarkers = stdinMarkers
	f.StringVar(&c.file.Path, "file", "", "Specify a file-path for the controller profile, use '-' to read from stdin.")
}

// Init implements cmd.Command.
func (c *addControllerProfileCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("controller profile name not specified")
	}
	c.name = args[0]
	if len(args) > 1 {
		return errors.New("too many args")
	}
	if c.file.Path == "" {
		return errors.New("controller profile file not specified")
	}
	return nil
}

// Run implements cmd.Command.
func (c *addControllerProfileCommand) Run(ctxt *cmd.Context) error {
	req, err := c.readSaveRequest(ctxt)
	if err != nil {
		return err
	}
	req.Version = 0

	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %w", err)
	}
	defer client.Close()

	resp, err := client.SaveControllerProfile(&req)
	if err != nil {
		return err
	}
	return c.out.Write(ctxt, resp)
}

func (c *addControllerProfileCommand) readSaveRequest(ctxt *cmd.Context) (apiparams.SaveControllerProfileRequest, error) {
	data, err := c.file.Read(ctxt)
	if err != nil {
		return apiparams.SaveControllerProfileRequest{}, err
	}
	var req apiparams.SaveControllerProfileRequest
	if err := yaml.Unmarshal(data, &req); err != nil {
		return apiparams.SaveControllerProfileRequest{}, err
	}
	if req.Name != "" && req.Name != c.name {
		return apiparams.SaveControllerProfileRequest{}, fmt.Errorf("provided controller profile name doesn't match, %s != %s", c.name, req.Name)
	}
	req.Name = c.name
	req.CreatedAt = ""
	req.UpdatedAt = ""
	return req, nil
}

// NewUpdateControllerProfileCommand returns a command to update a controller profile.
func NewUpdateControllerProfileCommand() cmd.Command {
	cmd := &updateControllerProfileCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// updateControllerProfileCommand updates a saved controller profile.
type updateControllerProfileCommand struct {
	addControllerProfileCommand
}

// Info implements cmd.Command.
func (c *updateControllerProfileCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-controller-profile",
		Args:     "<name>",
		Purpose:  "Update a saved controller profile in jimm.",
		Doc:      updateControllerProfileDoc,
		Examples: updateControllerProfileExample,
	})
}

// Run implements cmd.Command.
func (c *updateControllerProfileCommand) Run(ctxt *cmd.Context) error {
	req, err := c.readSaveRequest(ctxt)
	if err != nil {
		return err
	}

	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %w", err)
	}
	defer client.Close()

	if req.Version == 0 {
		current, err := client.GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: req.Name})
		if err != nil {
			return err
		}
		req.Version = current.Version
	}

	resp, err := client.SaveControllerProfile(&req)
	if err != nil {
		return err
	}
	return c.out.Write(ctxt, resp)
}

// NewShowControllerProfileCommand returns a command to show a controller profile.
func NewShowControllerProfileCommand() cmd.Command {
	cmd := &showControllerProfileCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// showControllerProfileCommand shows a saved controller profile.
type showControllerProfileCommand struct {
	jaasCommandBase
	out cmd.Output

	name string
}

// Info implements cmd.Command.
func (c *showControllerProfileCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "show-controller-profile",
		Args:     "<name>",
		Purpose:  "Show a saved controller profile from jimm.",
		Doc:      showControllerProfileDoc,
		Examples: showControllerProfileExample,
	})
}

// SetFlags implements cmd.Command.
func (c *showControllerProfileCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements cmd.Command.
func (c *showControllerProfileCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("controller profile name not specified")
	}
	c.name = args[0]
	if len(args) > 1 {
		return errors.New("too many args")
	}
	return nil
}

// Run implements cmd.Command.
func (c *showControllerProfileCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %w", err)
	}
	defer client.Close()

	resp, err := client.GetControllerProfile(&apiparams.GetControllerProfileRequest{Name: c.name})
	if err != nil {
		return err
	}
	return c.out.Write(ctxt, resp)
}

// NewListControllerProfilesCommand returns a command to list controller profiles.
func NewListControllerProfilesCommand() cmd.Command {
	cmd := &listControllerProfilesCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// listControllerProfilesCommand lists saved controller profiles.
type listControllerProfilesCommand struct {
	jaasCommandBase
	out cmd.Output

	jujuVersion string
}

// Info implements cmd.Command.
func (c *listControllerProfilesCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list-controller-profiles",
		Purpose:  "List saved controller profiles in jimm.",
		Doc:      listControllerProfilesDoc,
		Examples: listControllerProfilesExample,
	})
}

// SetFlags implements cmd.Command.
func (c *listControllerProfilesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.jujuVersion, "juju-version", "", "Only return profiles compatible with the specified Juju version.")
}

// Init implements cmd.Command.
func (c *listControllerProfilesCommand) Init(args []string) error {
	if len(args) > 0 {
		return errors.New("too many args")
	}
	return nil
}

// Run implements cmd.Command.
func (c *listControllerProfilesCommand) Run(ctxt *cmd.Context) error {
	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %w", err)
	}
	defer client.Close()

	profiles, err := client.ListControllerProfiles(&apiparams.ListControllerProfilesRequest{
		JujuVersion: c.jujuVersion,
	})
	if err != nil {
		return err
	}
	return c.out.Write(ctxt, profiles)
}

// NewRemoveControllerProfileCommand returns a command to remove a controller profile.
func NewRemoveControllerProfileCommand() cmd.Command {
	cmd := &removeControllerProfileCommand{}
	cmd.SetClientStore(jujuclient.NewFileClientStore())

	return modelcmd.WrapBase(cmd)
}

// removeControllerProfileCommand removes a saved controller profile.
type removeControllerProfileCommand struct {
	jaasCommandBase

	name  string
	force bool
}

// Info implements cmd.Command.
func (c *removeControllerProfileCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-controller-profile",
		Args:     "<name>",
		Purpose:  "Remove a saved controller profile from jimm.",
		Doc:      removeControllerProfileDoc,
		Examples: removeControllerProfileExample,
	})
}

// SetFlags implements cmd.Command.
func (c *removeControllerProfileCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "delete controller profile without prompt")
}

// Init implements cmd.Command.
func (c *removeControllerProfileCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("controller profile name not specified")
	}
	c.name = args[0]
	if len(args) > 1 {
		return errors.New("too many args")
	}
	return nil
}

// Run implements cmd.Command.
func (c *removeControllerProfileCommand) Run(ctxt *cmd.Context) error {
	if !c.force {
		reader := bufio.NewReader(ctxt.Stdin)
		_, err := fmt.Fprintf(ctxt.Stdout, "Confirm you would like to delete controller profile %q (y/N): ", c.name)
		if err != nil {
			return err
		}
		text, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read from input: %w", err)
		}
		text = strings.TrimSpace(text)
		if text != "y" && text != "Y" {
			return nil
		}
	}

	client, err := c.getJIMMAPI()
	if err != nil {
		return fmt.Errorf("could not create JIMM client: %w", err)
	}
	defer client.Close()

	return client.RemoveControllerProfile(&apiparams.RemoveControllerProfileRequest{Name: c.name})
}
