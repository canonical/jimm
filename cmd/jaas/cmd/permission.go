// Copyright 2025 Canonical.

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

	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/pkg/api"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

const (
	// accessMessageFormat is an informative message sent back to the user denoting the access for a particular resource.
	// The final format string holds either an AccessResultAllowed or AccessResultDenied.
	accessMessageFormat = "access check for %s on resource %s with role %s is %s"
	accessResultAllowed = "allowed"
	accessResultDenied  = "not allowed"
	defaultPageSize     = 50
)

const (
	genericConstraintsDoc = `
This command works at a low-level and commands like 'juju grant'
should be preferred in most cases.

Permissions in JIMM consist of an object, a relation and a target object.
These are used to define access control between resources.

The object and target object must be of the form <tag>-<objectname> or <tag>-<object-uuid>
E.g. "user-Alice" or "controller-MyController"

Certain reserved tags exist to denote specific resource types:
- The user-everyone@external tag represents all users.
- The controller-jimm tag represents the JIMM controller itself.

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

Certain constraints apply when creating/removing permissions, namely:
Resources may be one of:

    user tag                = "user-<name>"
    group tag               = "group-<name>"
	role tag 			    = "role-<name>"
    controller tag          = "controller-<name>"
    model tag               = "model-<name>"
	cloud tag			    = "cloud-<name>"
    application-offer tag   = "applicationoffer-<name>"

If target_object is a group, the relation can only be:

    member

If target_object is a role, the relation can only be:

	assignee

If target_object is a controller, the relation can be one of:

    audit_log_viewer (only relevent for the JIMM controller)
	can_addmodel
    administrator

If target_object is a model, the relation can be one of:

    reader
    writer
    administrator

If target_object is a cloud, the relation can be one of:

	administrator
	can_addmodel

If target_object is an application offer, the relation can be one of:

    reader
    consumer
    administrator

If the object is a group, a userset must be applied by adding #member as follows.
This will grant/revoke access to all users within TeamA:

    group-TeamA#member administrator controller-MyController

Similarly if the object is a role, a userset must be applied by adding #member as follows.

	role-Auditor#assignee audit_log_viewer controller-MyController
`

	addPermissionDoc = `
Grants access to a resource.
` + genericConstraintsDoc

	addRelationExample = `
    juju add-permission user-alice@canonical.com member group-mygroup
    juju add-permission group-MyTeam#member admin model-mymodel
    juju add-permission -f /path/to/file.yaml
`

	removePermissionDoc = `
Revokes access to a resource.
` + genericConstraintsDoc

	removePermissionExample = `
    juju remove-permission user-alice@canonical.com member group-mygroup
    juju remove-permission group-MyTeam#member admin model-mymodel
    juju remove-permission -f /path/to/file.yaml
`

	checkPermissionDoc = `
Verifies access to a resource.
`
	checkPermissionExample = `
    juju check-permission user-alice@canonical.com administrator controller-aws-controller-1
`

	listPermissionsDoc = `
List permissions known to JIMM. Using the "target", "relation" and "object" flags, 
only those permissions matching the filter will be returned.
`

	listPermissionsExample = `
List all permissions

    juju list-permissions

List permissions where the target object match

    juju list-permissions --target model-mymodel

List permissions where the target object and relation match

    juju list-permissions --target model-mymodel  --relation admin
`
)

// NewAddPermissionCommand returns a command to grant access.
func NewAddPermissionCommand() cmd.Command {
	cmd := &addPermission{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// addPermission adds permission.
type addPermission struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	object       string
	relation     string
	targetObject string

	filename string // optional
}

// Info implements the cmd.Command interface.
func (c *addPermission) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "add-permission",
		Args:     "<object> <relation> <target_object>",
		Purpose:  "Add relation to JIMM.",
		Doc:      addPermissionDoc,
		Examples: addRelationExample,
	})
}

// Init implements the cmd.Command interface.
func (c *addPermission) Init(args []string) error {
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
func (c *addPermission) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.filename, "f", "", "file location of JSON encoded tuples")
}

// Run implements Command.Run.
func (c *addPermission) Run(ctxt *cmd.Context) error {
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

// NewRemovePermissionCommand returns a command to remove access.
func NewRemovePermissionCommand() cmd.Command {
	cmd := &removePermissionCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// removePermissionCommand revokes access.
type removePermissionCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	object       string
	relation     string
	targetObject string

	filename string // optional
}

// Info implements the cmd.Command interface.
func (c *removePermissionCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-permission",
		Args:     "<object> <relation> <target_object>",
		Purpose:  "Remove relation from JIMM.",
		Doc:      removePermissionDoc,
		Examples: removePermissionExample,
	})
}

// Init implements the cmd.Command interface.
func (c *removePermissionCommand) Init(args []string) error {
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
func (c *removePermissionCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
	f.StringVar(&c.filename, "f", "", "file location of JSON encoded tuples")
}

// Run implements Command.Run.
func (c *removePermissionCommand) Run(ctxt *cmd.Context) error {
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

// checkPermissionCommand holds the fields required to check for access.
type checkPermissionCommand struct {
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

// NewCheckPermissionCommand
func NewCheckPermissionCommand() cmd.Command {
	cmd := &checkPermissionCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// Info implements the cmd.Command interface.
func (c *checkPermissionCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "check-permission",
		Args:     "<object> <relation> <target_object>",
		Purpose:  "Check access to a resource.",
		Doc:      checkPermissionDoc,
		Examples: checkPermissionExample,
	})
}

// SetFlags implements Command.SetFlags.
func (c *checkPermissionCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", map[string]cmd.Formatter{
		"smart": formatCheckRelationString,
		"json":  cmd.FormatJson,
		"yaml":  cmd.FormatYaml,
	})
}

// Init implements the cmd.Command interface.
func (c *checkPermissionCommand) Init(args []string) error {
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
	_, err := writer.Write([]byte((&accessResult).setMessage().Msg))
	if err != nil {
		return errors.E("failed to write access result", err)
	}
	return nil
}

// Run implements Command.Run.
func (c *checkPermissionCommand) Run(ctxt *cmd.Context) error {
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
	err = c.out.Write(ctxt, *(&accessResult{
		Tuple:   c.tuple,
		Allowed: resp.Allowed,
	}).setMessage())
	if err != nil {
		return err
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

// verifyTupleArguments is used across permission commands to verify the number of arguments.
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

// NewListPermissionsCommand returns a command to list permissions.
func NewListPermissionsCommand() cmd.Command {
	cmd := &listPermissionsCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// listPermissionsCommand lists permissions.
type listPermissionsCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store    jujuclient.ClientStore
	dialOpts *jujuapi.DialOpts

	tuple        apiparams.RelationshipTuple
	resolveUUIDs bool
}

// Info implements the cmd.Command interface.
func (c *listPermissionsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "list-permissions",
		Purpose:  "List relations.",
		Doc:      listPermissionsDoc,
		Examples: listPermissionsExample,
		Aliases:  []string{"permissions"},
	})
}

// Init implements the cmd.Command interface.
func (c *listPermissionsCommand) Init(args []string) error {
	if len(args) > 0 {
		return errors.E("too many args")
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *listPermissionsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatRelationsTabular,
	})
	f.StringVar(&c.tuple.Object, "object", "", "relation object")
	f.StringVar(&c.tuple.Relation, "relation", "", "relation name")
	f.StringVar(&c.tuple.TargetObject, "target", "", "relation target object")
	f.BoolVar(&c.resolveUUIDs, "resolve", true, "resolves UUIDs to human readable tags")
}

// Run implements Command.Run.
func (c *listPermissionsCommand) Run(ctxt *cmd.Context) error {
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
		Tuple:        c.tuple,
		PageSize:     defaultPageSize,
		ResolveUUIDs: c.resolveUUIDs,
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
			return nil, errors.E(fmt.Sprintf("failed to fetch list of relationship tuples: %s", err.Error()))
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
