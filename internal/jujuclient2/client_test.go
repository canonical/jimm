// Copyright 2024 Canonical.
package jujuclient2_test

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v5"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/jujuclient2"
	"github.com/canonical/jimm/v3/internal/jujuclient2/mocks"
)

type jimm struct {
	ModelManagerFactory jujuclient2.ModelManagerClientFactoryFunc
}

func TestModelManagerGetterFunc(t *testing.T) {
	c := qt.New(t)
	ctrl := gomock.NewController(c)

	j := jimm{
		ModelManagerFactory: func(p jujuclient2.DialParams) (jujuclient2.ModelManagerClient, error) {
			mockMM := mocks.NewMockModelManagerClient(ctrl)
			mockMM.EXPECT().ChangeModelCredential(gomock.Any(), gomock.Any()).AnyTimes()
			mockMM.EXPECT().Close().AnyTimes()
			return mockMM, nil
		},
	}

	mmc, _ := j.ModelManagerFactory(jujuclient2.DialParams{})
	defer mmc.Close()

	mmc.ChangeModelCredential(names.ModelTag{}, names.CloudCredentialTag{})
}

func TestJujuClientOptionCorrectlyMocks(t *testing.T) {
	c := qt.New(t)
	ctl := gomock.NewController(c)
	mockDialer := mocks.NewMockDialer(ctl)
	mockDialer.EXPECT().Dial(gomock.Any())
	mockMmc := mocks.NewMockModelManagerClient(ctl)

	mockMmc.
		EXPECT().
		ChangeModelCredential(gomock.Any(), gomock.Any()).
		Return(errors.New("mocked client"))

	jjc := jujuclient2.NewJujuClient(
		mockDialer,
		jujuclient2.WithNewModelManager(mockMmc),
	)

	mmc, _ := jjc.ModelManager(jujuclient2.DialParams{})
	defer mmc.Close()

	err := mmc.ChangeModelCredential(names.ModelTag{}, names.CloudCredentialTag{})
	c.Assert(err, qt.IsNotNil)
}
