// Copyright 2020 Canonical Ltd.

// Package conv converts data structures between the JIMM internal
// representation and the juju API representation.
package conv

import (
	"fmt"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

// ToCloudCredentialTag creates a juju cloud credential tag from the given
// CredentialPath.
func ToCloudCredentialTag(p params.CredentialPath) names.CloudCredentialTag {
	if p.IsZero() {
		return names.CloudCredentialTag{}
	}
	user := ToUserTag(p.User)
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", p.Cloud, user.Id(), p.Name))
}

// ToTaggedCredential converts the given mongodoc.Credential to a
// jujuparams.TaggedCredential.
func ToTaggedCredential(cred *mongodoc.Credential) jujuparams.TaggedCredential {
	return jujuparams.TaggedCredential{
		Tag: ToCloudCredentialTag(cred.Path.ToParams()).String(),
		Credential: jujuparams.CloudCredential{
			AuthType:   string(cred.Type),
			Attributes: cred.Attributes,
		},
	}
}
