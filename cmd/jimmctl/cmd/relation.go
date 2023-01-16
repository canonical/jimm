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
	-f		Read from a file where filename is the location of a JSON encoded file of the form:
		[
			{
				"object":"user:user-mike",
				"relation":"member",
				"target_object":"group:group-yellow"
			},
			{
				"object":"user:user-alice",
				"relation":"member",
				"target_object":"group:group-yellow"
			}
		]

		Certain constraints apply when creating/removing a relation, namely:
		Object may be one of:

		user tag
		group tag
		controller tag
		model tag
		application offer tag

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
	`

	addRelationDoc = `
	add command adds relation to jimm.

	Example:
		jimmctl auth relation add <object> <relation> <target_object>
		jimmctl auth relation add -f <filename>
	` + genericConstraintsDoc

	removeRelationDoc = `
	remove command removes a relation from jimm.

	Example:
		jimmctl auth relation remove <object> <relation> <target_object>
		jimmctl auth relation remove -f <filename>
	` + genericConstraintsDoc
)

// NewRelationCommand returns a command for relation management.
func NewRelationCommand() *cmd.SuperCommand {
	cmd := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:    "relation",
		Doc:     relationDoc,
		Purpose: "relation management.",
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

	//Naming follows OpenFGA convention
	object       string //object
	relation     string
	targetObject string //target_object

	filename string //optional
}

// Info implements the cmd.Command interface.
func (c *addRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add",
		Purpose: "Add relation to jimm",
		Doc:     addRelationDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *addRelationCommand) Init(args []string) error {
	if c.filename != "" {
		return nil
	}
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
	c.object, c.relation, c.targetObject, args = args[0], args[1], args[2], args[3:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
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

	//Naming follows OpenFGA convention
	object       string //object
	relation     string
	targetObject string //target_object

	filename string //optional
}

// Info implements the cmd.Command interface.
func (c *removeRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove",
		Purpose: "Remove relation to jimm",
		Doc:     removeRelationDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *removeRelationCommand) Init(args []string) error {
	if c.filename != "" {
		return nil
	}
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
	c.object, c.relation, c.targetObject, args = args[0], args[1], args[2], args[3:]
	if len(args) > 0 {
		return errors.E("too many args")
	}
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
