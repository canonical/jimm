package jem

import "github.com/CanonicalLtd/jimm/internal/pubsub"

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

func Pubsub(j *JEM) *pubsub.Hub {
	return j.pubsub
}
