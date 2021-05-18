package jem

var (
	RandIntn                    = &randIntn
	WallClock                   = &wallClock
	Shuffle                     = &shuffle
	MongodocAPIHostPorts        = mongodocAPIHostPorts
	ControllerUpdateCredentials = (*JEM).controllerUpdateCredentials
	CredentialAddController     = (*JEM).credentialAddController
	CredentialsRemoveController = (*JEM).credentialsRemoveController
	SetCredentialUpdates        = (*JEM).setCredentialUpdates
	UpdateControllerCredential  = (*JEM).updateControllerCredential
	OfferConnectionsToMongodoc  = offerConnectionsToMongodoc
)
