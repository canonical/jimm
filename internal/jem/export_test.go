package jem

import "unsafe"

var (
	ControllerLocationQuery    = (Database).controllerLocationQuery
	RandIntn                   = &randIntn
	CredentialAddController    = (Database).credentialAddController
	CredentialRemoveController = (Database).credentialRemoveController
	UpdateCredential           = (Database).updateCredential
	UpdateControllerCredential = (*JEM).updateControllerCredential
	SetCredentialUpdates       = (Database).setCredentialUpdates
	NewDatabase                = newDatabase
	WallClock                  = &wallClock
)

func RefCount(db Database) uintptr {
	return uintptr(unsafe.Pointer(db.cnt))
}
