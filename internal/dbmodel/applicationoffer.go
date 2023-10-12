// Copyright 2021 Canonical Ltd.

package dbmodel

import (
	"sort"
	"strings"
	"time"

	"github.com/juju/charm/v11"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
)

// An ApplicationOffer is an offer for an application.
type ApplicationOffer struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Model is the model that this offer is for.
	ModelID uint
	Model   Model

	// Application is the application that this offer is for.
	ApplicationName string

	ApplicationDescription string

	// Name is the name of the offer.
	Name string

	// UUID is the unique ID of the offer.
	UUID string `gorm:"not null;uniqueIndex"`

	// Application offer URL.
	URL string

	// Users contains the users with access to the application offer.
	Users []UserApplicationOfferAccess `gorm:"foreignKey:ApplicationOfferID;references:ID"`

	// Endpoints contains remote endpoints for the application offer.
	Endpoints []ApplicationOfferRemoteEndpoint

	// Spaces contains spaces in remote models for the application offer.
	Spaces []ApplicationOfferRemoteSpace

	// Bindings contains bindings for the application offer.
	Bindings StringMap

	// Connections contains details about connections to the application offer.
	Connections []ApplicationOfferConnection

	// CharmURL is the URL of the charm deployed to the application.
	CharmURL string `gorm:"column:charm_url"`
}

// Tag returns a names.Tag for the application-offer.
func (o ApplicationOffer) Tag() names.Tag {
	return o.ResourceTag()
}

// ResourceTag returns the tag for the application-offer. This method
// is intended to be used in places where we expect to see
// a concrete type names.ApplicationOfferTag instead of the
// names.Tag interface.
func (o ApplicationOffer) ResourceTag() names.ApplicationOfferTag {
	return names.NewApplicationOfferTag(o.UUID)
}

// SetTag sets the application-offer's UUID from the given tag.
func (o *ApplicationOffer) SetTag(t names.ApplicationOfferTag) {
	o.UUID = t.Id()
}

// UserAccess returns the access level for the specified user.
func (o *ApplicationOffer) UserAccess(u *User) string {
	for _, ou := range o.Users {
		if u.Username == ou.Username {
			return ou.Access
		}
	}
	return ""
}

// FromJujuApplicationOfferAdminDetails fills in the information from jujuparams ApplicationOfferAdminDetails.
func (o *ApplicationOffer) FromJujuApplicationOfferAdminDetails(offerDetails jujuparams.ApplicationOfferAdminDetails) {
	o.ApplicationName = offerDetails.ApplicationName
	o.ApplicationDescription = offerDetails.ApplicationDescription
	o.Name = offerDetails.OfferName
	o.UUID = offerDetails.OfferUUID
	o.URL = offerDetails.OfferURL
	o.Bindings = offerDetails.Bindings
	o.CharmURL = offerDetails.CharmURL

	o.Endpoints = make([]ApplicationOfferRemoteEndpoint, len(offerDetails.Endpoints))
	for i, endpoint := range offerDetails.Endpoints {
		o.Endpoints[i] = ApplicationOfferRemoteEndpoint{
			Name:      endpoint.Name,
			Role:      string(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}

	o.Users = []UserApplicationOfferAccess{}
	for _, user := range offerDetails.Users {
		if strings.IndexByte(user.UserName, '@') < 0 {
			// skip controller local users.
			continue
		}
		o.Users = append(o.Users, UserApplicationOfferAccess{
			Username: user.UserName,
			User: User{
				Username: user.UserName,
			},
			Access: user.Access,
		})
	}

	o.Spaces = make([]ApplicationOfferRemoteSpace, len(offerDetails.Spaces))
	for i, space := range offerDetails.Spaces {
		o.Spaces[i] = ApplicationOfferRemoteSpace{
			CloudType:          space.CloudType,
			Name:               space.Name,
			ProviderID:         space.ProviderId,
			ProviderAttributes: space.ProviderAttributes,
		}
	}

	o.Connections = make([]ApplicationOfferConnection, len(offerDetails.Connections))
	for i, connection := range offerDetails.Connections {
		o.Connections[i] = ApplicationOfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationID:     connection.RelationId,
			Username:       connection.Username,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
		}
	}
}

// ToJujuApplicationOfferDetails returns a jujuparams ApplicationOfferAdminDetails based on the application offer.
func (o *ApplicationOffer) ToJujuApplicationOfferDetails() jujuparams.ApplicationOfferAdminDetails {
	endpoints := make([]jujuparams.RemoteEndpoint, len(o.Endpoints))
	for i, endpoint := range o.Endpoints {
		endpoints[i] = jujuparams.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      charm.RelationRole(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	spaces := make([]jujuparams.RemoteSpace, len(o.Spaces))
	for i, space := range o.Spaces {
		spaces[i] = jujuparams.RemoteSpace{
			CloudType:          space.CloudType,
			Name:               space.Name,
			ProviderId:         space.ProviderID,
			ProviderAttributes: space.ProviderAttributes,
		}
	}
	users := make([]jujuparams.OfferUserDetails, len(o.Users))
	for i, ua := range o.Users {
		users[i] = jujuparams.OfferUserDetails{
			UserName:    ua.User.Username,
			DisplayName: ua.User.DisplayName,
			Access:      ua.Access,
		}

	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].UserName < users[j].UserName
	})

	connections := make([]jujuparams.OfferConnection, len(o.Connections))
	for i, connection := range o.Connections {
		connections[i] = jujuparams.OfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationId:     connection.RelationID,
			Username:       connection.Username,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
		}
	}
	return jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         o.Model.Tag().String(),
			OfferUUID:              o.UUID,
			OfferURL:               o.URL,
			OfferName:              o.Name,
			ApplicationDescription: o.ApplicationDescription,
			Endpoints:              endpoints,
			Spaces:                 spaces,
			Bindings:               o.Bindings,
			Users:                  users,
		},
		ApplicationName: o.ApplicationName,
		CharmURL:        o.CharmURL,
		Connections:     connections,
	}
}

// ApplicationOfferRemoteEndpoint represents a remote application endpoint.
type ApplicationOfferRemoteEndpoint struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// ApplicationOffer is the application-offer associated with this endpoint.
	ApplicationOfferID uint
	ApplicationOffer   ApplicationOffer

	Name      string
	Role      string
	Interface string
	Limit     int
}

// ApplicationOfferRemoteSpace represents a space in some remote model.
type ApplicationOfferRemoteSpace struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// ApplicationOffer is the application-offer associated with this space.
	ApplicationOfferID uint
	ApplicationOffer   ApplicationOffer `gorm:"constraint:OnDelete:CASCADE"`

	CloudType          string
	Name               string
	ProviderID         string
	ProviderAttributes Map
}

// ApplicationOfferConnection holds details about a connection to an offer.
type ApplicationOfferConnection struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// ApplicationOffer is the application-offer associated with this connection.
	ApplicationOfferID uint
	ApplicationOffer   ApplicationOffer `gorm:"constraint:OnDelete:CASCADE"`

	SourceModelTag string
	RelationID     int
	Username       string
	Endpoint       string
	IngressSubnets Strings
}

// A UserApplicationOfferAccess maps the access level for a user on an
// application-offer.
type UserApplicationOfferAccess struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// User is the user associated with this access.
	Username string
	User     User `gorm:"foreignKey:Username;references:Username"`

	// ApplicationOffer is the application-offer associated with this
	// access.
	ApplicationOfferID uint
	ApplicationOffer   ApplicationOffer `gorm:"foreignKey:ApplicationOfferID;references:ID"`

	// Access is the access level for to the application offer to the user.
	Access string
}

// TableName overrides the table name gorm will use to find
// UserApplicationOfferAccess records.
func (UserApplicationOfferAccess) TableName() string {
	return "user_application_offer_access"
}
