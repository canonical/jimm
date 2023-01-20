// Copyright 2023 Canonical Ltd.

package cmd

import (
	"encoding/json"
	"os"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/CanonicalLtd/jimm/api"
	apiparams "github.com/CanonicalLtd/jimm/api/params"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

var (
	relationDoc = `
relation command enables relation management for jimm
`
	genericConstraintsDoc = `
The object and target object must be of the form <tag>-<objectname> or <tag>-<object-uuid>
E.g. "user-Alice" or "controller-MyController"

-f		Read from a file where filename is the location of a JSON encoded file of the form:
	[
		{
			"object":"user-mike",
			"relation":"member",
			"target_object":"group-yellow"
		},
		{
			"object":"user-alice",
			"relation":"member",
			"target_object":"group-yellow"
		}
	]

Certain constraints apply when creating/removing a relation, namely:
Object may be one of:

	user tag 				= "user-<name>"
	group tag 				= "group-<name>"
	controller tag 			= "controller-<name>"
	model tag 				= "model-<name>"
	application offer tag 	= "offer-<name>"

If target_object is a group, the relation can only be:

	member

If target_object is a controller, the relation can be one of:

	loginer
	administrator

If target_object is a model, the relation can be one of:

	reader
	writer
	administrator

If target_object is an application offer, the relation can be one of:

	reader
	consumer
	administrator 


Additionally, if the object is a group, a userset can be applied by adding #member as follows:

	group-TeamA#member administrator controller-MyController

This will grant/revoke access from users that are members of TeamA as administrators of MyController.
`

	addRelationDoc = `
add command adds relation to jimm.

Example:
	jimmctl auth relation add <object> <relation> <target_object>
	jimmctl auth relation add -f <filename>
` + genericConstraintsDoc +
		`
Examples:
jimmctl auth relation add user-Alice member group-MyGroup
jimmctl auth relation add group-MyTeam#member loginer controller-MyController
`

	removeRelationDoc = `
remove command removes a relation from jimm.

Example:
	jimmctl auth relation remove <object> <relation> <target_object>
	jimmctl auth relation remove -f <filename>
	` + genericConstraintsDoc +
		`
Examples:
jimmctl auth relation remove user-Alice member group-MyGroup
jimmctl auth relation remove group-MyTeam#member loginer controller-MyController`
)

// NewRelationCommand returns a command for relation management.
func NewRelationCommand() *cmd.SuperCommand {
	cmd := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "relation",
		Doc:     relationDoc,
		Purpose: "Relation management.",
	})
	cmd.Register(newAddRelationCommand())
	cmd.Register(newRemoveRelationCommand())

	return cmd
}

// newAddRelationCommand returns a command to add a relation.
func newAddRelationCommand() cmd.Command {
	cmd := &addRelationCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addRelationCommand adds a relation.
type addRelationCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	object       string
	relation     string
	targetObject string

	filename string //optional
}

// Info implements the cmd.Command interface.
func (c *addRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add",
		Purpose: "Add relation to jimm.",
		Doc:     addRelationDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *addRelationCommand) Init(args []string) error {
	if c.filename != "" {
		return nil
	}
	err := verifyTupleArguments(args)
	if err != nil {
		return errors.E(err)
	}
	c.object, c.relation, c.targetObject = args[0], args[1], args[2]
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *addRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabular,
	})
	f.StringVar(&c.filename, "f", "", "file location of JSON encoded tuples")
}

// Run implements Command.Run.
func (c *addRelationCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	var params apiparams.AddRelationRequest
	if c.filename == "" {
		params.Tuples = append(params.Tuples, apiparams.RelationshipTuple{
			Object:       c.object,
			Relation:     c.relation,
			TargetObject: c.targetObject,
		})
	} else {
		params.Tuples, err = readTupleFile(c.filename)
		if err != nil {
			return err
		}
	}

	client := api.NewClient(apiCaller)
	err = client.AddRelation(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// newRemoveRelationCommand returns a command to remove a relation.
func newRemoveRelationCommand() cmd.Command {
	cmd := &removeRelationCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// removeRelationCommand removes a relation.
type removeRelationCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	object       string
	relation     string
	targetObject string

	filename string //optional
}

// Info implements the cmd.Command interface.
func (c *removeRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove",
		Purpose: "Remove relation from jimm.",
		Doc:     removeRelationDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *removeRelationCommand) Init(args []string) error {
	if c.filename != "" {
		return nil
	}
	err := verifyTupleArguments(args)
	if err != nil {
		return errors.E(err)
	}
	c.object, c.relation, c.targetObject = args[0], args[1], args[2]
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *removeRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabular,
	})
	f.StringVar(&c.filename, "f", "", "file location of JSON encoded tuples")
}

// Run implements Command.Run.
func (c *removeRelationCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	var params apiparams.RemoveRelationRequest
	if c.filename == "" {
		params.Tuples = append(params.Tuples, apiparams.RelationshipTuple{
			Object:       c.object,
			Relation:     c.relation,
			TargetObject: c.targetObject,
		})
	} else {
		params.Tuples, err = readTupleFile(c.filename)
		if err != nil {
			return err
		}
	}

	client := api.NewClient(apiCaller)
	err = client.RemoveRelation(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

// readTupleFile reads a file with filename as provided by the user and attempts to
// unmarshal the JSON into a list of relationship tuples.
func readTupleFile(filename string) ([]apiparams.RelationshipTuple, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var res []apiparams.RelationshipTuple
	err = json.Unmarshal(content, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// verifyTupleArguments is used across relation commands to verify the number of arguments.
func verifyTupleArguments(args []string) error {
	switch len(args) {
	default:
		return errors.E("too many args")
	case 0:
		return errors.E("object not specified")
	case 1:
		return errors.E("relation not specified")
	case 2:
		return errors.E("target object not specified")
	case 3:
	}
	return nil
}
