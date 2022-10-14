// Copyright 2020 Canonical Ltd.

// Package conv converts data structures between the JIMM internal
// representation and the juju API representation.
package conv

import (
	"fmt"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

// ToCloudCredentialTag creates a juju cloud credential tag from the given
// CredentialPath.
func ToCloudCredentialTag(p mongodoc.CredentialPath) names.CloudCredentialTag {
	if p.IsZero() {
		return names.CloudCredentialTag{}
	}
	user := ToUserTag(params.User(p.User))
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", p.Cloud, user.Id(), p.Name))
}

// FromCloudCredentialTag creates a CredentialPath from the given juju
// cloud credential tag.
func FromCloudCredentialTag(t names.CloudCredentialTag) (mongodoc.CredentialPath, error) {
	if t.IsZero() {
		return mongodoc.CredentialPath{}, nil
	}
	user, err := FromUserTag(t.Owner())
	if err != nil {
		return mongodoc.CredentialPath{}, errgo.Mask(err, errgo.Is(ErrLocalUser))
	}
	return mongodoc.CredentialPath{
		Cloud: t.Cloud().Id(),
		EntityPath: mongodoc.EntityPath{
			User: string(user),
			Name: t.Name(),
		},
	}, nil
}

// ToTaggedCredential converts the given mongodoc.Credential to a
// jujuparams.TaggedCredential.
func ToTaggedCredential(cred *mongodoc.Credential) jujuparams.TaggedCredential {
	return jujuparams.TaggedCredential{
		Tag: ToCloudCredentialTag(cred.Path).String(),
		Credential: jujuparams.CloudCredential{
			AuthType:   string(cred.Type),
			Attributes: cred.Attributes,
		},
	}
}
