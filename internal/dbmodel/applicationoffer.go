// Copyright 2021 Canonical Ltd.

package dbmodel

import (
	"time"

	"github.com/juju/charm/v12"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gorm.io/gorm"
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
	URL string `gorm:"unique;not null"`

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

// FromJujuApplicationOfferAdminDetails maps the Juju ApplicationOfferDetails struct type to a JIMM type
// such that it can be persisted.
func (o *ApplicationOffer) FromJujuApplicationOfferAdminDetailsV5(offerDetails jujuparams.ApplicationOfferAdminDetailsV5) {
	o.ApplicationName = offerDetails.ApplicationName
	o.ApplicationDescription = offerDetails.ApplicationDescription
	o.Name = offerDetails.OfferName
	o.UUID = offerDetails.OfferUUID
	o.URL = offerDetails.OfferURL
	o.CharmURL = offerDetails.CharmURL
	o.Endpoints = mapJujuRemoteEndpointsToJIMMRemoteEndpoints(offerDetails.Endpoints)
	o.Connections = mapJujuConnectionsToJIMMConnections(offerDetails.Connections)
}

// ToJujuApplicationOfferDetails maps the JIMM ApplicationOfferDetails struct type to a jujuparams type
// such that it can be sent over the wire.
func (o *ApplicationOffer) ToJujuApplicationOfferDetailsV5() jujuparams.ApplicationOfferAdminDetailsV5 {
	v5Details := jujuparams.ApplicationOfferDetailsV5{
		SourceModelTag:         o.Model.Tag().String(),
		OfferUUID:              o.UUID,
		OfferURL:               o.URL,
		OfferName:              o.Name,
		ApplicationDescription: o.ApplicationDescription,
		Endpoints:              mapJIMMRemoteEndpointsToJujuRemoteEndpoints(o.Endpoints),
	}

	v5AdminDetails := jujuparams.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: v5Details,
		ApplicationName:           o.ApplicationName,
		CharmURL:                  o.CharmURL,
		Connections:               mapJIMMConnectionsToJujuConnections(o.Connections),
	}

	return v5AdminDetails
}

// ApplicationOfferRemoteEndpoint represents a remote application endpoint.
type ApplicationOfferRemoteEndpoint struct {
	gorm.Model

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
	gorm.Model

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
	gorm.Model

	// ApplicationOffer is the application-offer associated with this connection.
	ApplicationOfferID uint
	ApplicationOffer   ApplicationOffer `gorm:"constraint:OnDelete:CASCADE"`

	SourceModelTag string
	RelationID     int
	IdentityName   string
	Endpoint       string
	IngressSubnets Strings
}

// mapJIMMRemoteEndpointsToJujuRemoteEndpoints maps the types between JIMM's
// remote endpoints type (with gorm embedded for persistence) to a jujuparams
// remote endpoint, such that it can be sent over the wire and contains the correct
// json tags.
func mapJIMMRemoteEndpointsToJujuRemoteEndpoints(endpoints []ApplicationOfferRemoteEndpoint) []jujuparams.RemoteEndpoint {
	mappedEndpoints := make([]jujuparams.RemoteEndpoint, len(endpoints))
	for i, endpoint := range endpoints {
		mappedEndpoints[i] = jujuparams.RemoteEndpoint{
			Name:      endpoint.Name,
			Role:      charm.RelationRole(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	return mappedEndpoints
}

// mapJujuRemoteEndpointsToJIMMRemoteEndpoints - See above for details, this does the opposite.
func mapJujuRemoteEndpointsToJIMMRemoteEndpoints(endpoints []jujuparams.RemoteEndpoint) []ApplicationOfferRemoteEndpoint {
	mappedEndpoints := make([]ApplicationOfferRemoteEndpoint, len(endpoints))
	for i, endpoint := range endpoints {
		mappedEndpoints[i] = ApplicationOfferRemoteEndpoint{
			Name:      endpoint.Name,
			Role:      string(endpoint.Role),
			Interface: endpoint.Interface,
			Limit:     endpoint.Limit,
		}
	}
	return mappedEndpoints
}

// mapJIMMConnectionsToJujuConnections maps the types between JIMM's
// offer connection type (with gorm embedded for persistence) to a jujuparams
// offer connection, such that it can be sent over the wire and contains the correct
// json tags.
func mapJIMMConnectionsToJujuConnections(connections []ApplicationOfferConnection) []jujuparams.OfferConnection {
	mappedConnections := make([]jujuparams.OfferConnection, len(connections))
	for i, connection := range connections {
		mappedConnections[i] = jujuparams.OfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationId:     connection.RelationID,
			Username:       connection.IdentityName,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
			// TODO(ale8k): Status is missing, do we need it??
		}
	}
	return mappedConnections
}

// mapJujuConnectionsToJIMMConnections - See above for details, this does the opposite.
func mapJujuConnectionsToJIMMConnections(connections []jujuparams.OfferConnection) []ApplicationOfferConnection {
	mappedConnections := make([]ApplicationOfferConnection, len(connections))
	for i, connection := range connections {
		mappedConnections[i] = ApplicationOfferConnection{
			SourceModelTag: connection.SourceModelTag,
			RelationID:     connection.RelationId,
			IdentityName:   connection.Username,
			Endpoint:       connection.Endpoint,
			IngressSubnets: connection.IngressSubnets,
		}
	}
	return mappedConnections
}
