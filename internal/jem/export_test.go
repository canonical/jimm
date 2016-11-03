package jem

var (
	RandIntn                   = &randIntn
	WallClock                  = &wallClock
	NewDatabase                = newDatabase
	ClearCredentialUpdate      = (*Database).clearCredentialUpdate
	CredentialAddController    = (*Database).credentialAddController
	CredentialRemoveController = (*Database).credentialRemoveController
	SetCredentialUpdates       = (*Database).setCredentialUpdates
	UpdateCredential           = (*Database).updateCredential
	SelectController           = (*JEM).selectController
	UpdateControllerCredential = (*JEM).updateControllerCredential
	ErrCanceled                = errCanceled
	RunWithContext             = runWithContext
)

func DatabaseClose(db *Database) {
	db.session.Close()
}

func DatabaseSessionIsDead(db *Database) bool {
	return !db.session.MayReuse()
}
