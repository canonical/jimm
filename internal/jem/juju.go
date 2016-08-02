// Copyright 2016 Canonical Ltd.

package jem

import (
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujucloud "github.com/juju/juju/cloud"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jem/internal/apiconn"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

// CreateModelParams specifies the parameters needed to create a new
// model using CreateModel.
type CreateModelParams struct {
	// Path contains the path of the new model.
	Path params.EntityPath

	// ControllerPath contains the path of the owning
	// controller.
	ControllerPath params.EntityPath

	// Credential contains the name of the credential to use to
	// create the model.
	Credential params.Name

	// Cloud contains the name of the cloud in which the
	// model will be created.
	Cloud params.Cloud

	// Region contains the name of the region in which the model will
	// be created. This may be empty if the cloud does not support
	// regions.
	Region string

	// Attributes contains the attributes to assign to the new model.
	Attributes map[string]interface{}
}

// CreateModel creates a new model as specified by p using conn.
func (j *JEM) CreateModel(conn *apiconn.Conn, p CreateModelParams) (*mongodoc.Model, error) {
	if err := j.UpdateControllerCredential(conn, p.Path.User, p.Cloud, p.Credential); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// an model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:       p.Path,
		Controller: p.ControllerPath,
	}
	if err := j.AddModel(modelDoc); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	mmClient := modelmanager.NewClient(conn.Connection)
	m, err := mmClient.CreateModel(
		string(p.Path.Name),
		UserTag(p.Path.User).Id(),
		p.Region,
		string(p.Credential),
		p.Attributes,
	)
	if err != nil {
		// Remove the model that was created, because it's no longer valid.
		if err := j.DB.Models().RemoveId(modelDoc.Id); err != nil {
			logger.Errorf("cannot remove model from database after model creation error: %v", err)
		}
		return nil, errgo.Notef(err, "cannot create model")
	}
	if err := mmClient.GrantModel(conn.Info.Tag.(names.UserTag).Id(), "admin", m.UUID); err != nil {
		// TODO (mhilton) destroy the model?
		return nil, errgo.Notef(err, "cannot grant admin access")
	}
	// Now set the UUID to that of the actually created model.
	if err := j.DB.Models().UpdateId(modelDoc.Id, bson.D{{"$set", bson.D{{"uuid", m.UUID}}}}); err != nil {
		// TODO (mhilton) destroy the model?
		return nil, errgo.Notef(err, "cannot update model UUID in database, leaked model %s", m.UUID)
	}
	modelDoc.UUID = m.UUID
	return modelDoc, nil
}

// UpdateControllerCredential uploads the specified credential to conn.
func (j *JEM) UpdateControllerCredential(conn *apiconn.Conn, user params.User, cloud params.Cloud, name params.Name) error {
	userTag := UserTag(user)
	cloudTag := CloudTag(cloud)
	cred, err := j.Credential(user, cloud, name)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	cloudClient := cloudapi.NewClient(conn)

	// Currently juju does not support updating existing credentials,
	// bug 1608421. For now we will only upload them if they don't
	// exist.
	controllerCreds, err := cloudClient.Credentials(userTag, cloudTag)
	if err != nil {
		return errgo.Mask(err)
	}
	for k := range controllerCreds {
		if k == string(name) {
			return nil
		}
	}

	err = cloudClient.UpdateCredentials(userTag, cloudTag, map[string]jujucloud.Credential{
		string(name): jujucloud.NewCredential(jujucloud.AuthType(cred.Type), cred.Attributes),
	})
	if err != nil {
		return errgo.Notef(err, "cannot upload credentials")
	}
	return nil
}

// UserTag creates a juju user tag from a params.User
func UserTag(u params.User) names.UserTag {
	return names.NewUserTag(string(u) + "@external")
}

// CloudTag creates a juju cloud tag from a params.Cloud
func CloudTag(c params.Cloud) names.CloudTag {
	return names.NewCloudTag(string(c))
}
