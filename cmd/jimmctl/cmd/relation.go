// Copyright 2023 Canonical Ltd.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

const (
	// accessMessageFormat is an informative message sent back to the user denoting the access for a particular resource.
	// The final format string holds either an AccessResultAllowed or AccessResultDenied.
	accessMessageFormat = "access check for %s on resource %s with role %s is %s"
	accessResultAllowed = "allowed"
	accessResultDenied  = "not allowed"
	defaultPageSize     = 50
)

var (
	relationDoc = `
relation command enables relation management for jimm
`
	genericConstraintsDoc = `
The object and target object must be of the form <tag>-<objectname> or <tag>-<object-uuid>
E.g. "user-Alice" or "controller-MyController"

-f    Read from a file where filename is the location of a JSON encoded file of the form:
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

	user tag                = "user-<name>"
	group tag               = "group-<name>"
	controller tag          = "controller-<name>"
	model tag               = "model-<name>"
	application offer tag   = "offer-<name>"

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


Additionally, if the object is a group, a userset can be applied by adding #member as follows.
This will grant/revoke the relation to all users within TeamA:

	group-TeamA#member administrator controller-MyController
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

	checkRelationDoc = `
Verifies the access between resources.

Example:
jimmctl auth relation check user-alice@canonical.com administrator controller-aws-controller-1

Example:
	jimmctl auth relation check <object> <relation> <target_object>
	jimmctl auth relation check -f <filename>
	`

	listRelationsDoc = `
list relations known to jimm. Using the "target", "relation"
and "object" flags, only those relations matching the filter
will be returned.
Examples:
	jimmctl auth relation list
	returns the list of all relations

	jimmctl auth relation list --target <target_object>
	returns the list of relations, where target object
	matches the specified one

	jimmctl auth relation list --target <target_object>  --relation <relation>
	returns the list of relations, where target object
	and relation match the specified ones
`
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
	cmd.Register(newCheckRelationCommand())
	cmd.Register(newListRelationsCommand())

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
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
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
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
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

// checkRelationCommand holds the fields required to check a relation.
type checkRelationCommand struct {
	modelcmd.ControllerCommandBase
	out      cmd.Output
	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	tuple apiparams.RelationshipTuple
}

// accessResult holds the accessCheck result to be passed to a formatter
type accessResult struct {
	Msg     string                      `yaml:"result" json:"result"`
	Tuple   apiparams.RelationshipTuple `yaml:"tuple" json:"tuple"`
	Allowed bool                        `yaml:"allowed" json:"allowed"`
}

func (ar *accessResult) setMessage() *accessResult {
	t := ar.Tuple

	accessMsg := accessResultDenied
	if ar.Allowed {
		accessMsg = accessResultAllowed
	}
	ar.Msg = fmt.Sprintf(accessMessageFormat, t.Object, t.TargetObject, t.Relation, accessMsg)
	return ar
}

// newCheckRelationCommand
func newCheckRelationCommand() cmd.Command {
	cmd := &checkRelationCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// Info implements the cmd.Command interface.
func (c *checkRelationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "check",
		Purpose: "Check access to a resource.",
		Doc:     checkRelationDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *checkRelationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", map[string]cmd.Formatter{
		"smart": formatCheckRelationString,
		"json":  cmd.FormatJson,
		"yaml":  cmd.FormatYaml,
	})
}

// Init implements the cmd.Command interface.
func (c *checkRelationCommand) Init(args []string) error {
	err := verifyTupleArguments(args)
	if err != nil {
		return errors.E(err)
	}
	c.tuple = apiparams.RelationshipTuple{
		Object:       args[0],
		Relation:     args[1],
		TargetObject: args[2],
	}
	return nil
}

func formatCheckRelationString(writer io.Writer, value interface{}) error {
	accessResult, ok := value.(accessResult)
	if !ok {
		return errors.E("failed to parse access result")
	}
	writer.Write([]byte((&accessResult).setMessage().Msg))
	return nil
}

// Run implements Command.Run.
func (c *checkRelationCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}
	client := api.NewClient(apiCaller)

	resp, err := client.CheckRelation(&apiparams.CheckRelationRequest{
		Tuple: c.tuple,
	})
	if err != nil {
		return err
	}
	c.out.Write(ctxt, *(&accessResult{
		Tuple:   c.tuple,
		Allowed: resp.Allowed,
	}).setMessage())
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

// newListRelationsCommand returns a command to list relations.
func newListRelationsCommand() cmd.Command {
	cmd := &listRelationsCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// listRelationsCommand adds a relation.
type listRelationsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	tuple apiparams.RelationshipTuple
}

// Info implements the cmd.Command interface.
func (c *listRelationsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "list",
		Purpose: "List relations.",
		Doc:     listRelationsDoc,
	})
}

// Init implements the cmd.Command interface.
func (c *listRelationsCommand) Init(args []string) error {
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *listRelationsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatRelationsTabular,
	})
	f.StringVar(&c.tuple.Object, "object", "", "relation object")
	f.StringVar(&c.tuple.Relation, "relation", "", "relation name")
	f.StringVar(&c.tuple.TargetObject, "target", "", "relation target object")
}

// Run implements Command.Run.
func (c *listRelationsCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	params := apiparams.ListRelationshipTuplesRequest{
		Tuple:    c.tuple,
		PageSize: defaultPageSize,
	}
	result, err := fetchRelations(client, params)
	if err != nil {
		return errors.E(err)
	}

	// Ensure continutation token is empty so that we don't print it.
	result.ContinuationToken = ""
	err = c.out.Write(ctxt, result)
	if err != nil {
		return errors.E(err)
	}

	return nil
}

func fetchRelations(client *api.Client, params apiparams.ListRelationshipTuplesRequest) (*apiparams.ListRelationshipTuplesResponse, error) {
	tuples := make([]apiparams.RelationshipTuple, 0)
	for {
		response, err := client.ListRelationshipTuples(&params)
		if err != nil {
			return nil, errors.E(err)
		}
		tuples = append(tuples, response.Tuples...)

		if response.ContinuationToken == "" {
			return &apiparams.ListRelationshipTuplesResponse{Tuples: tuples, Errors: response.Errors}, nil
		}
		params.ContinuationToken = response.ContinuationToken
	}
}

func formatRelationsTabular(writer io.Writer, value interface{}) error {
	resp, ok := value.(*apiparams.ListRelationshipTuplesResponse)
	if !ok {
		return errors.E(fmt.Sprintf("expected value of type %T, got %T", resp, value))
	}

	table := uitable.New()
	table.MaxColWidth = 80
	table.Wrap = true

	table.AddRow("Object", "Relation", "Target Object")
	for _, tuple := range resp.Tuples {
		table.AddRow(tuple.Object, tuple.Relation, tuple.TargetObject)
	}
	fmt.Fprint(writer, table)

	if len(resp.Errors) != 0 {
		fmt.Fprintf(writer, "\n\n")
		fmt.Fprintln(writer, "Errors")
		for _, msg := range resp.Errors {
			fmt.Fprintln(writer, msg)
		}
	}
	return nil
}
