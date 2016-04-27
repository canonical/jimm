package params

import (
	"bytes"
	"fmt"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
)

// SetControllerPerm holds the parameters for setting the ACL
// on a controller.
type SetControllerPerm struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetControllerPerm holds the parameters for getting the ACL
// of a controller.
type GetControllerPerm struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name/perm"`
	EntityPath
}

// SetModelPerm holds the parameters for setting the ACL
// on an model.
type SetModelPerm struct {
	httprequest.Route `httprequest:"PUT /v2/model/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetModelPerm holds the parameters for getting the ACL
// of an model.
type GetModelPerm struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name/perm"`
	EntityPath
}

// SetTemplatePerm holds the parameters for setting the ACL
// on a template.
type SetTemplatePerm struct {
	httprequest.Route `httprequest:"PUT /v2/template/:User/:Name/perm"`
	EntityPath
	ACL ACL `httprequest:",body"`
}

// GetTemplatePerm holds the parameters for getting the ACL
// on a template.
type GetTemplatePerm struct {
	httprequest.Route `httprequest:"GET /v2/template/:User/:Name/perm"`
	EntityPath
}

// ACL holds an access control list for an entity.
type ACL struct {
	// Read holds users and groups that are allowed to read the
	// entity.
	Read []string `json:"read"`
}

// AddController holds the parameters for adding a new controller.
type AddController struct {
	httprequest.Route `httprequest:"PUT /v2/controller/:User/:Name"`
	EntityPath
	Info ControllerInfo `httprequest:",body"`
}

// DeleteController holds the parameters for removing the Controller.
type DeleteController struct {
	httprequest.Route `httprequest:"DELETE /v2/controller/:User/:Name"`
	EntityPath
}

type GetControllerLocations struct {
	httprequest.Route `httprequest:"GET /v2/location/:Attr"`
	Attr              string `httprequest:",path"`

	// Location constrains the controllers that the
	// set of returned values will be returned from
	// to those with matching location attributes.
	// Note that the values in this should be passed in the
	// URL query parameters.
	Location map[string]string
}

type ControllerLocationsResponse struct {
	Values []string
}

// ControllerInfo holds information specifying how
// to connect to an existing controller.
type ControllerInfo struct {
	// HostPorts holds host/port pairs (in host:port form)
	// of the controller API endpoints.
	HostPorts []string `json:"host-ports"`

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// User holds the name of user to use when
	// connecting to the controller.
	User string `json:"user"`

	// Password holds the password for the user.
	Password string `json:"password"`

	// ControllerUUID holds the UUID of the admin model
	// of the controller.
	ControllerUUID string `json:"controller-uuid"`

	// Location holds location attributes to be associated with the controller.
	Location map[string]string
}

// EntityPath holds the path parameters for specifying
// an entity in the API. It can also be used as a value
// in its own right, because it implements TextMarshaler
// and TextUnmarshaler.
type EntityPath struct {
	User User `httprequest:",path"`
	Name Name `httprequest:",path"`
}

func (p EntityPath) String() string {
	return fmt.Sprintf("%s/%s", p.User, p.Name)
}

var slash = []byte("/")

func (p *EntityPath) UnmarshalText(data []byte) error {
	parts := bytes.Split(data, slash)
	if len(parts) != 2 {
		return errgo.New("wrong number of parts in entity path")
	}
	if err := p.User.UnmarshalText(parts[0]); err != nil {
		return errgo.Mask(err)
	}
	if err := p.Name.UnmarshalText(parts[1]); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (p EntityPath) MarshalText() ([]byte, error) {
	data := make([]byte, 0, len(p.User)+1+len(p.Name))
	data = append(data, p.User...)
	data = append(data, '/')
	data = append(data, p.Name...)
	return data, nil
}

// GetModel holds parameters for retrieving
// an model.
type GetModel struct {
	httprequest.Route `httprequest:"GET /v2/model/:User/:Name"`
	EntityPath
}

// DeleteModel holds parameters for deletion of
// an model.
type DeleteModel struct {
	httprequest.Route `httprequest:"DELETE /v2/model/:User/:Name"`
	EntityPath
}

// NewModel holds parameters for creating a new model.
type NewModel struct {
	httprequest.Route `httprequest:"POST /v2/model/:User"`

	// User holds the User element from the URL path.
	User User `httprequest:",path"`

	// Info holds the information required to create
	// the model.
	Info NewModelInfo `httprequest:",body"`
}

// ListModels holds parameters for listing
// current models.
type ListModels struct {
	httprequest.Route `httprequest:"GET /v2/model"`

	// TODO add parameters for restricting results.
}

// ListModelsResponse holds a list of controllers as returned
// by ListModels.
type ListModelsResponse struct {
	Models []ModelResponse `json:"models"`
}

// ListController holds parameters for listing all current controllers.
type ListController struct {
	httprequest.Route `httprequest:"GET /v2/controller"`

	// TODO add parameters for restricting results.
}

// ListControllerResponse holds a list of controllers as returned
// by ListController.
type ListControllerResponse struct {
	// TODO factor out common items in the templates
	// into a separate field.
	Controllers []ControllerResponse `json:"controllers"`
}

// ListTemplate holds parameters for listing all current templates.
type ListTemplates struct {
	httprequest.Route `httprequest:"GET /v2/template"`

	// TODO add parameters for restricting results.
}

// ListTemplatesResponse holds a list of templates as returned
// by ListTemplates.
type ListTemplatesResponse struct {
	Templates []TemplateResponse `json:"templates"`
}

// TemplateResponse holds information on a template
type TemplateResponse struct {
	// Path holds the path of the template.
	Path EntityPath `json:"path"`

	// Schema holds the controller schema that was used
	// to create the template.
	Schema environschema.Fields `json:"schema"`

	// Config holds the template's attributes, with all secret attributes
	// replaced with their zero value.
	Config map[string]interface{} `json:"config"`
}

// GetTemplate holds parameters for retrieving information on a template.
type GetTemplate struct {
	httprequest.Route `httprequest:"GET /v2/template/:User/:Name"`
	EntityPath
}

// DeleteTemplate holds parameters for deletion of a template.
type DeleteTemplate struct {
	httprequest.Route `httprequest:"DELETE /v2/template/:User/:Name"`
	EntityPath
}

// ControllerResponse holds information on a given Controller.
// Each Controller is also associated with an model
// at /v2/model/:User/:Name where User and Name
// are the same as that of the Controller's path.
type ControllerResponse struct {
	// Path holds the path of the controller.
	Path EntityPath `json:"path"`

	// ProviderType holds the kind of provider used
	// by the Controller.
	ProviderType string `json:"provider-type,omitempty"`

	// Schema holds the fields required to start
	// a new model using the Controller.
	Schema environschema.Fields `json:"schema,omitempty"`

	// Location holds location attributes to be associated with the controller.
	Location map[string]string
}

// GetController holds parameters for retrieving information on a Controller.
type GetController struct {
	httprequest.Route `httprequest:"GET /v2/controller/:User/:Name"`
	EntityPath
}

// NewModelInfo holds the JSON body parameters
// for a NewModel request.
type NewModelInfo struct {
	// Name holds the name to give to the new model
	// within its user name space.
	Name Name `json:"name"`

	// Controller holds the path to the controller entity
	// to use to start the model.
	// This is optional and may not be available to all users.
	Controller *EntityPath `json:"controller,omitempty"`

	// Location holds location attributes that narrow down the range
	// of possible controllers to be used for the model.
	Location map[string]string

	// TemplatePaths optionally holds a sequence of templates to use
	// to create the base configuration entry on top of which Config is applied.
	// Each path must refer to an entry in /template, and overrides attributes in the one before it,
	// followed finally by Config itself. The resulting configuration
	// is checked for compatibility with the schema of the above controller.
	TemplatePaths []EntityPath `json:"templates,omitempty"`

	// Config holds the configuration attributes to use to create the new model.
	// It is applied on top of the above templates.
	Config map[string]interface{} `json:"config"`
}

// ModelResponse holds the response body from
// a NewModel call.
type ModelResponse struct {
	// Path holds the path of the model.
	Path EntityPath `json:"path"`

	// User holds the admin user name associated
	// with the model.
	User string `json:"user"`

	// Password holds the admin password associated with the
	// model. If it is empty, macaroon authentication should
	// be used to connect to the model (only possible when
	// macaroon authentication is implemented by Juju).
	Password string `json:"password"`

	// UUID holds the UUID of the model.
	UUID string `json:"uuid"`

	// ControllerPath holds the path of the controller holding this model.
	ControllerPath EntityPath `json:"controller-path"`

	// ControllerUUID holds the UUID of the controller's admin UUID.
	ControllerUUID string `json:"controller-uuid"`

	// CACert holds the CA certificate that will be used
	// to validate the controller's certificate, in PEM format.
	CACert string `json:"ca-cert"`

	// HostPorts holds host/port pairs (in host:port form)
	// of the controller API endpoints.
	HostPorts []string `json:"host-ports"`
}

// AddTemplate holds parameters for adding a template.
type AddTemplate struct {
	httprequest.Route `httprequest:"PUT /v2/template/:User/:Name"`
	EntityPath

	Info AddTemplateInfo `httprequest:",body"`
}

// AddTemplateInfo holds information on a template to
// be added.
type AddTemplateInfo struct {
	// Controller holds the name of a controller to use
	// as the base schema for the template. The Config attributes
	// below will be checked against the schema of this controller.
	Controller EntityPath `json:"controller"`

	// Config holds the template's configuration attributes.
	Config map[string]interface{} `json:"config"`
}

// WhoAmI holds parameters for requesting the current user name.
type WhoAmI struct {
	httprequest.Route `httprequest:"GET /v2/whoami"`
}

// WhoAmIResponse holds information on the currently
// authenticated user.
type WhoAmIResponse struct {
	User string `json:"user"`
}
