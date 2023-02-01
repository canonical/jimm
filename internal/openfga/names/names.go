// Copyright 2023 CanonicalLtd.

// Package names holds functions used by other jimm components to
// create valid OpenFGA tags.
package names

import (
	"fmt"

	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
	"github.com/juju/names/v4"
)

// UserTag returns a string containing the
// OpenFGA tag for the user based on the username.
func UserTag(user names.UserTag) string {
	return fmt.Sprintf("user:%s", user.Id())
}

// GroupTag returns a string containing the
// OpenFGA tag for the group based on its id.
func GroupTag(group jimmnames.GroupTag) string {
	return fmt.Sprintf("group:%s", group.Id())
}

// ControllerTag returns a string containing the
// OpenFGA tag for the controller based on its uuid.
func ControllerTag(controller names.ControllerTag) string {
	return fmt.Sprintf("controller:%s", controller.Id())
}

// ModelTag returns a string containing the
// OpenFGA tag for the controller based on its uuid.
func ModelTag(model names.ModelTag) string {
	return fmt.Sprintf("model:%s", model.Id())
}

// ApplicationOfferTag returns a string containing the
// OpenFGA tag for the applicatio offer based on its uuid.
func ApplicationOfferTag(offer names.ApplicationOfferTag) string {
	return fmt.Sprintf("applicationoffer:%s", offer.Id())
}
