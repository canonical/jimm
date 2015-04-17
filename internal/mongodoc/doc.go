package mongodoc

import (
	"github.com/CanonicalLtd/jem/params"
)

// StateServer holds information on a given state server.
type StateServer struct {
	Id   string `bson:"_id"` // Actually user/name.
	User string
	Name string
	Info params.ServerInfo
}
