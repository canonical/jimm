package jem

var (
	RandIntn                   = &randIntn
	CredentialAddController    = (*Database).credentialAddController
	CredentialRemoveController = (*Database).credentialRemoveController
	UpdateCredential           = (*Database).updateCredential
	UpdateControllerCredential = (*JEM).updateControllerCredential
	SetCredentialUpdates       = (*Database).setCredentialUpdates
	ClearCredentialUpdate      = (*Database).clearCredentialUpdate
	NewDatabase                = newDatabase
	WallClock                  = &wallClock
)

func DatabaseClose(db *Database) {
	db.session.Close()
}

func DatabaseSessionIsDead(db *Database) bool {
	return !db.session.MayReuse()
}
