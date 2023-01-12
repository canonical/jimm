// Copyright 2023 Canonical Ltd.

package cmd

import (
	"encoding/json"
	"os"

	"github.com/juju/cmd/v3"
	jujucmdv3 "github.com/juju/cmd/v3"
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

	addRelationDoc = `
	add command adds relation to jimm.

	Example:
		jimmctl auth relation add <object> <relation> <target_object>
		jimmctl auth relation add -f <filename>

		-f		Read from a file where filename is the location of a JSON encoded file of the form:
		[
			{
				"object":"user:mike",
				"relation":"member",
				"target_object":"group:yellow"
			},
			{
				"object":"user:alice",
				"relation":"member",
				"target_object":"group:yellow"
			}
		]

		Certain constraints apply when creating a relation, namely:
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
)

// NewRelationCommand returns a command for relation management.
func NewRelationCommand() *jujucmdv3.SuperCommand {
	cmd := jujucmd.NewSuperCommand(jujucmdv3.SuperCommandParams{
		Name:    "relation",
		Doc:     relationDoc,
		Purpose: "relation management.",
	})
	cmd.Register(newRelationCommand())

	return cmd
}

// newRelationCommand returns a command to add a relation.
func newRelationCommand() cmd.Command {
	cmd := &addrelationCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addrelationCommand adds a relation.
type addrelationCommand struct {
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
func (c *addrelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "add",
		Purpose: "Add relation to jimm",
		Doc:     addRelationDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *addrelationCommand) Init(args []string) error {
	if c.filename != "" {
		return nil
	}
	switch len(args) {
	default:
		return errors.E("too many args")
	case 0:
		return errors.E("object")
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
func (c *addrelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatTabular,
	})
	f.StringVar(&c.filename, "filename", "", "file location of JSON encoded tuples")
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
func (c *addrelationCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	var params apiparams.AddRelationRequest
	if c.filename != "" {
		params.Tuples = append(params.Tuples, apiparams.RelationshipTuple{
			Object:       c.object,
			Relation:     c.relation,
			TargetObject: c.targetObject,
		})
	} else {
		params.Tuples, err = readTupleFile(c.filename)
		if err != nil {
			return nil
		}
	}

	client := api.NewClient(apiCaller)
	err = client.AddRelation(&params)
	if err != nil {
		return errors.E(err)
	}

	return nil
}
