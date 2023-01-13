package names

import (
	"regexp"

	"github.com/juju/names/v4"
)

// JIMM handles applicationoffers via UUID, as such
// this is a JIMM variant of juju names applicationoffer
// to target UUID rather than applicationoffer name.

const (
	ApplicationOfferTagKind = names.ApplicationOfferTagKind
)

var (
	validUUID = regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
)

// ApplicationOfferTag represents a tag used to describe a JIMM applicationoffer.
type ApplicationOfferTag struct {
	uuid string
}

// NewApplicationOfferTag returns the tag of an applicationoffer with the given applicationoffer UUID.
func NewApplicationOfferTag(uuid string) ApplicationOfferTag {
	return ApplicationOfferTag{uuid: uuid}
}

// ParseModelTag parses an environ tag string.
func ParseApplicationOfferTag(applicationofferTag string) (ApplicationOfferTag, error) {
	tag, err := ParseTag(applicationofferTag)
	if err != nil {
		return ApplicationOfferTag{}, err
	}
	et, ok := tag.(ApplicationOfferTag)
	if !ok {
		return ApplicationOfferTag{}, invalidTagError(applicationofferTag, ApplicationOfferTagKind)
	}
	return et, nil
}

func (t ApplicationOfferTag) String() string { return t.Kind() + "-" + t.Id() }
func (t ApplicationOfferTag) Kind() string   { return ApplicationOfferTagKind }
func (t ApplicationOfferTag) Id() string     { return t.uuid }

// IsValidApplicationOfferTag returns whether id is a valid applicationoffer UUID.
func IsValidApplicationOfferTag(id string) bool {
	return validUUID.MatchString(id)
}
