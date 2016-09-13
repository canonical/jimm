package jem

import (
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/limitpool"
)

var (
	ControllerLocationQuery    = (Database).controllerLocationQuery
	RandIntn                   = &randIntn
	CredentialAddController    = (Database).credentialAddController
	CredentialRemoveController = (Database).credentialRemoveController
	UpdateCredential           = (Database).updateCredential
	UpdateControllerCredential = (*JEM).updateControllerCredential
)

func MakeDatabase(db *mgo.Database, g limitpool.Gauge) Database {
	return Database{
		Database: db,
		g:        g,
	}
}
