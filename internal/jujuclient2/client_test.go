// Copyright 2024 Canonical.
package jujuclient2_test

import (
	"context"
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/jujuclient2"
	"github.com/canonical/jimm/v3/internal/jujuclient2/mocks"
)

func TestJujuClientOptionCorrectlyMocks(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()
	ctl := gomock.NewController(c)
	mockDialer := mocks.NewMockDialer(ctl)
	mockDialer.EXPECT().Dial(gomock.Any(), gomock.Any(), gomock.Any())
	mockMmc := mocks.NewMockModelManagerClient(ctl)

	mockMmc.
		EXPECT().
		ChangeModelCredential(gomock.Any(), gomock.Any()).
		Return(errors.New("mocked client"))

	jjc := jujuclient2.NewJujuClient(
		mockDialer,
		jujuclient2.WithNewModelManager(mockMmc),
	)

	mmc, _ := jjc.ModelManager(ctx, nil, names.ModelTag{})

	err := mmc.ChangeModelCredential(names.ModelTag{}, names.CloudCredentialTag{})
	c.Assert(err, qt.IsNotNil)
}
