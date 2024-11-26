// Copyright 2024 Canonical.

package dbmodel

import (
	"time"

	"github.com/juju/names/v5"
)

// An ApplicationOffer is an offer for an application.
type ApplicationOffer struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Model is the model that this offer is for.
	ModelID uint
	Model   Model

	// Name is the name of the offer.
	Name string

	// UUID is the unique ID of the offer.
	UUID string `gorm:"not null;uniqueIndex"`

	// Application offer URL.
	URL string `gorm:"unique;not null"`
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
