package cmd

import (
	"testing"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	"go.uber.org/mock/gomock"
)

type cmdMocks struct {
	client *mocks.MockJIMMAPI
	writer *mocks.MockWriter
	store  *mocks.MockClientStore
}

func setupCmdMocks(t *testing.T) *cmdMocks {
	t.Helper()
	ctrl := gomock.NewController(t)
	h := &cmdMocks{
		client: mocks.NewMockJIMMAPI(ctrl),
		writer: mocks.NewMockWriter(ctrl),
		store:  mocks.NewMockClientStore(ctrl),
	}
	t.Cleanup(ctrl.Finish)
	return h
}
