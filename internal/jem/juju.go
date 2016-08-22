// Copyright 2016 Canonical Ltd.

package jem

import (
	"fmt"

	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
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
func (j *JEM) CreateModel(conn *apiconn.Conn, p CreateModelParams) (*mongodoc.Model, *jujuparams.ModelInfo, error) {
	if err := j.UpdateControllerCredential(conn, p.Path.User, p.Cloud, p.Credential); err != nil {
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
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
		return nil, nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}
	mmClient := modelmanager.NewClient(conn.Connection)
	m, err := mmClient.CreateModel(
		string(p.Path.Name),
		UserTag(p.Path.User).Id(),
		p.Region,
		CloudCredentialTag(p.Cloud, p.Path.User, p.Credential),
		p.Attributes,
	)
	if err != nil {
		// Remove the model that was created, because it's no longer valid.
		if err := j.DB.Models().RemoveId(modelDoc.Id); err != nil {
			logger.Errorf("cannot remove model from database after model creation error: %v", err)
		}
		return nil, nil, errgo.Notef(err, "cannot create model")
	}
	if err := mmClient.GrantModel(conn.Info.Tag.(names.UserTag).Id(), "admin", m.UUID); err != nil {
		// TODO (mhilton) destroy the model?
		return nil, nil, errgo.Notef(err, "cannot grant admin access")
	}
	// Now set the UUID to that of the actually created model.
	if err := j.DB.Models().UpdateId(modelDoc.Id, bson.D{{"$set", bson.D{{"uuid", m.UUID}}}}); err != nil {
		// TODO (mhilton) destroy the model?
		return nil, nil, errgo.Notef(err, "cannot update model UUID in database, leaked model %s", m.UUID)
	}
	modelDoc.UUID = m.UUID
	return modelDoc, &m, nil
}

// UpdateControllerCredential uploads the specified credential to conn.
func (j *JEM) UpdateControllerCredential(conn *apiconn.Conn, user params.User, cloud params.Cloud, name params.Name) error {
	userTag := UserTag(user)
	cloudTag := CloudTag(cloud)
	cloudCredentialTag := CloudCredentialTag(cloud, user, name)
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
	for _, credTag := range controllerCreds {
		if credTag == cloudCredentialTag {
			return nil
		}
	}

	err = cloudClient.UpdateCredential(
		cloudCredentialTag,
		jujucloud.NewCredential(jujucloud.AuthType(cred.Type), cred.Attributes),
	)
	if err != nil {
		return errgo.Notef(err, "cannot upload credentials")
	}
	return nil
}

// GrantModel grants the given access for the given user on the given model and updates the JEM database.
func (j *JEM) GrantModel(conn *apiconn.Conn, model *mongodoc.Model, user params.User, access string) error {
	client := modelmanager.NewClient(conn)
	if err := client.GrantModel(UserTag(user).Id(), access, model.UUID); err != nil {
		return errgo.Mask(err)
	}
	if err := j.Grant(j.DB.Models(), model.Path, user); err != nil {
		// TODO (mhilton) What should be done with the changes already made to the controller.
		return errgo.Mask(err)
	}
	return nil
}

// RevokeModel revokes the given access for the given user on the given model and updates the JEM database.
func (j *JEM) RevokeModel(conn *apiconn.Conn, model *mongodoc.Model, user params.User, access string) error {
	if err := j.Revoke(j.DB.Models(), model.Path, user); err != nil {
		return errgo.Mask(err)
	}
	client := modelmanager.NewClient(conn)
	if err := client.RevokeModel(UserTag(user).Id(), access, model.UUID); err != nil {
		// TODO (mhilton) What should be done with the changes already made to JEM.
		return errgo.Mask(err)
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

// CloudCredentialTag creates a juju cloud credential tag from the given
// cloud, user and name.
func CloudCredentialTag(cloud params.Cloud, user params.User, name params.Name) names.CloudCredentialTag {
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s@external/%s", cloud, user, name))
}
