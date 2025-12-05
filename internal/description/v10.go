// Copyright 2025 Canonical.

package description

import (
	descriptionv10 "github.com/juju/description/v10"
	"github.com/juju/names/v5"
)

// migrationDescriptionV10 implements the Description interface.
type migrationDescriptionV10 struct {
	desc descriptionv10.Model
}

// Serialize serializes the model description.
func (md *migrationDescriptionV10) Serialize() ([]byte, error) {
	return descriptionv10.Serialize(md.desc)
}

// Owner returns the owner of the model.
func (md *migrationDescriptionV10) Owner() names.UserTag {
	return md.desc.Owner()
}

// SetOwner sets the owner of the model.
func (md *migrationDescriptionV10) SetOwner(owner names.UserTag) {
	md.desc.SetOwner(owner)
}

// Users returns the users with access to the model.
func (md *migrationDescriptionV10) Users() []User {
	v10Users := md.desc.Users()
	users := make([]User, len(v10Users))
	for i, u := range v10Users {
		users[i] = u
	}
	return users
}

// ClearUsers removes all users from the model description.
func (md *migrationDescriptionV10) ClearUsers() {
	md.desc.SetUsers(nil)
}

// CloudCredential returns the cloud credential used by the model.
func (md *migrationDescriptionV10) CloudCredential() CloudCredential {
	return md.desc.CloudCredential()
}

// CloudRegion returns the cloud region the model is deployed to.
func (md *migrationDescriptionV10) CloudRegion() string {
	return md.desc.CloudRegion()
}

// SetCloudCredential sets the cloud credential for the model.
func (md *migrationDescriptionV10) SetCloudCredential(args CloudCredentialArgs) {
	md.desc.SetCloudCredential(descriptionv10.CloudCredentialArgs{
		Owner:      args.Owner,
		Cloud:      args.Cloud,
		Name:       args.Name,
		AuthType:   args.AuthType,
		Attributes: args.Attributes,
	})
}

// Config returns the model configuration.
func (md *migrationDescriptionV10) Config() map[string]any {
	return md.desc.Config()
}

// Applications returns the applications in the model.
func (md *migrationDescriptionV10) Applications() []Application {
	v10Apps := md.desc.Applications()
	apps := make([]Application, len(v10Apps))
	for i, a := range v10Apps {
		apps[i] = &applicationV10{app: a}
	}
	return apps
}

// applicationV10 wraps an application description.
type applicationV10 struct {
	app descriptionv10.Application
}

// Name returns the application name.
func (a *applicationV10) Name() string {
	return a.app.Name()
}

// Offers returns the application offers.
func (a *applicationV10) Offers() []ApplicationOffer {
	v10Offers := a.app.Offers()
	offers := make([]ApplicationOffer, len(v10Offers))
	for i, o := range v10Offers {
		offers[i] = o
	}
	return offers
}
