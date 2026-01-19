package cmd

import (
	"bytes"
	"testing"

	jujucmd "github.com/juju/cmd/v3"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
)

type cmdMocks struct {
	client *mocks.MockJIMMAPI
	store  *mocks.MockClientStore
}

func setupCmdMocks(t *testing.T) *cmdMocks {
	t.Helper()
	ctrl := gomock.NewController(t)
	h := &cmdMocks{
		client: mocks.NewMockJIMMAPI(ctrl),
		store:  mocks.NewMockClientStore(ctrl),
	}
	t.Cleanup(ctrl.Finish)
	return h
}

func newTestContext(t *testing.T) *jujucmd.Context {
	return &jujucmd.Context{
		Context: t.Context(),
		Dir:     t.TempDir(),
		Stdin:   &bytes.Buffer{},
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}
