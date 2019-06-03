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
	SelectRandomController     = (*JEM).selectRandomController
	UpdateControllerCredential = (*JEM).updateControllerCredential
	Shuffle                    = &shuffle
)
