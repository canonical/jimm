package jem

var (
	RandIntn                    = &randIntn
	WallClock                   = &wallClock
	NewDatabase                 = newDatabase
	ClearCredentialUpdate       = (*Database).clearCredentialUpdate
	CredentialAddController     = (*Database).credentialAddController
	CredentialRemoveController  = (*Database).credentialRemoveController
	SetCredentialUpdates        = (*Database).setCredentialUpdates
	UpdateCredential            = (*Database).updateCredential
	UpdateControllerCredential  = (*JEM).updateControllerCredential
	Shuffle                     = &shuffle
	MongodocAPIHostPorts        = mongodocAPIHostPorts
	ControllerUpdateCredentials = (*JEM).controllerUpdateCredentials
	DoControllers               = (*JEM).doControllers
)
