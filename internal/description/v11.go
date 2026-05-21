// Copyright 2025 Canonical.

package description

import (
	descriptionv11 "github.com/juju/description/v11"
	"github.com/juju/names/v5"
)

// migrationDescriptionV11 implements the Description interface.
type migrationDescriptionV11 struct {
	desc descriptionv11.Model
}

// Serialize serializes the model description.
func (md *migrationDescriptionV11) Serialize() ([]byte, error) {
	return descriptionv11.Serialize(md.desc)
}

// Owner returns the owner of the model.
func (md *migrationDescriptionV11) Owner() names.UserTag {
	return md.desc.Owner()
}

// SetOwner sets the owner of the model.
func (md *migrationDescriptionV11) SetOwner(owner names.UserTag) {
	md.desc.SetOwner(owner)
}

// Users returns the users with access to the model.
func (md *migrationDescriptionV11) Users() []User {
	v10Users := md.desc.Users()
	users := make([]User, len(v10Users))
	for i, u := range v10Users {
		users[i] = u
	}
	return users
}

// ClearUsers removes all users from the model description.
func (md *migrationDescriptionV11) ClearUsers() {
	md.desc.SetUsers(nil)
}

// CloudCredential returns the cloud credential used by the model.
func (md *migrationDescriptionV11) CloudCredential() CloudCredential {
	return md.desc.CloudCredential()
}

// CloudRegion returns the cloud region the model is deployed to.
func (md *migrationDescriptionV11) CloudRegion() string {
	return md.desc.CloudRegion()
}

// SetCloudCredential sets the cloud credential for the model.
func (md *migrationDescriptionV11) SetCloudCredential(args CloudCredentialArgs) {
	md.desc.SetCloudCredential(descriptionv11.CloudCredentialArgs{
		Owner:      args.Owner,
		Cloud:      args.Cloud,
		Name:       args.Name,
		AuthType:   args.AuthType,
		Attributes: args.Attributes,
	})
}

// Config returns the model configuration.
func (md *migrationDescriptionV11) Config() map[string]any {
	return md.desc.Config()
}

// Applications returns the applications in the model.
func (md *migrationDescriptionV11) Applications() []Application {
	v11Apps := md.desc.Applications()
	apps := make([]Application, len(v11Apps))
	for i, a := range v11Apps {
		apps[i] = &applicationV11{app: a}
	}
	return apps
}

// applicationV11 wraps an application description.
type applicationV11 struct {
	app descriptionv11.Application
}

// Name returns the application name.
func (a *applicationV11) Name() string {
	return a.app.Name()
}

// Offers returns the application offers.
func (a *applicationV11) Offers() []ApplicationOffer {
	v10Offers := a.app.Offers()
	offers := make([]ApplicationOffer, len(v10Offers))
	for i, o := range v10Offers {
		offers[i] = o
	}
	return offers
}
