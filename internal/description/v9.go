// Copyright 2025 Canonical.

package description

import (
	descriptionv9 "github.com/juju/description/v9"
	"github.com/juju/names/v5"
)

// migrationDescriptionV9 implements the Description interface.
type migrationDescriptionV9 struct {
	desc descriptionv9.Model
}

// Serialize serializes the model description.
func (md *migrationDescriptionV9) Serialize() ([]byte, error) {
	return descriptionv9.Serialize(md.desc)
}

// Owner returns the owner of the model.
func (md *migrationDescriptionV9) Owner() names.UserTag {
	return md.desc.Owner()
}

// SetOwner sets the owner of the model.
func (md *migrationDescriptionV9) SetOwner(owner names.UserTag) {
	md.desc.SetOwner(owner)
}

// Users returns the users with access to the model.
func (md *migrationDescriptionV9) Users() []User {
	v9Users := md.desc.Users()
	users := make([]User, len(v9Users))
	for i, u := range v9Users {
		users[i] = u
	}
	return users
}

// ClearUsers removes all users from the model description.
func (md *migrationDescriptionV9) ClearUsers() {
	md.desc.SetUsers(nil)
}

// CloudCredential returns the cloud credential used by the model.
func (md *migrationDescriptionV9) CloudCredential() CloudCredential {
	return md.desc.CloudCredential()
}

// CloudRegion returns the cloud region the model is deployed to.
func (md *migrationDescriptionV9) CloudRegion() string {
	return md.desc.CloudRegion()
}

// SetCloudCredential sets the cloud credential for the model.
func (md *migrationDescriptionV9) SetCloudCredential(args CloudCredentialArgs) {
	md.desc.SetCloudCredential(descriptionv9.CloudCredentialArgs{
		Owner:      args.Owner,
		Cloud:      args.Cloud,
		Name:       args.Name,
		AuthType:   args.AuthType,
		Attributes: args.Attributes,
	})
}

// Config returns the model configuration.
func (md *migrationDescriptionV9) Config() map[string]any {
	return md.desc.Config()
}

// Applications returns the applications in the model.
func (md *migrationDescriptionV9) Applications() []Application {
	v9Apps := md.desc.Applications()
	apps := make([]Application, len(v9Apps))
	for i, a := range v9Apps {
		apps[i] = &applicationV9{app: a}
	}
	return apps
}

// applicationV9 wraps an application description.
type applicationV9 struct {
	app descriptionv9.Application
}

// Name returns the application name.
func (a *applicationV9) Name() string {
	return a.app.Name()
}

// Offers returns the application offers.
func (a *applicationV9) Offers() []ApplicationOffer {
	v9Offers := a.app.Offers()
	offers := make([]ApplicationOffer, len(v9Offers))
	for i, o := range v9Offers {
		offers[i] = o
	}
	return offers
}
