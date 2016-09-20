package jem

var (
	ControllerLocationQuery    = (*Database).controllerLocationQuery
	RandIntn                   = &randIntn
	CredentialAddController    = (*Database).credentialAddController
	CredentialRemoveController = (*Database).credentialRemoveController
	UpdateCredential           = (*Database).updateCredential
	UpdateControllerCredential = (*JEM).updateControllerCredential
	SetCredentialUpdates       = (*Database).setCredentialUpdates
	ClearCredentialUpdate      = (*Database).clearCredentialUpdate
	NewDatabase                = newDatabase
	WallClock                  = &wallClock
	DatabaseDecRef             = (*Database).decRef
)

func DatabaseSessionIsDead(db *Database) bool {
	return db.status.isDead()
}

func DatabaseSetAlive(db *Database) {
	db.status = 0
}
