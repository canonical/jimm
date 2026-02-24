// Copyright 2026 Canonical.

package cmd

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	cookiejar "github.com/juju/persistent-cookiejar"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	jimmapiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

type addModelMocks struct {
	client      *mocks.MockJIMMAPI
	cloudClient *mocks.MockAddModelCloudAPI
	store       *mocks.MockClientStore
}

func TestAddModel(t *testing.T) {
	c := qt.New(t)

	tests := []struct {
		about                string
		cloudRegion          string
		credentialName       string
		expectCloudCall      bool
		expectListUserClouds bool
		modelName            string
	}{{
		about:           "with cloud region and credential specified",
		cloudRegion:     "test-cloud/test-region",
		credentialName:  "credAlice",
		expectCloudCall: true,
		modelName:       "test-model",
	}, {
		about:           "credentials not specified",
		cloudRegion:     "test-cloud/test-region",
		credentialName:  "",
		expectCloudCall: true,
		modelName:       "test-model",
	}, {
		about:           "region not specified",
		cloudRegion:     "test-cloud",
		credentialName:  "",
		expectCloudCall: true,
		modelName:       "test-model",
	}, {
		about:                "cloud not specified",
		cloudRegion:          "",
		credentialName:       "",
		expectCloudCall:      false,
		expectListUserClouds: true,
		modelName:            "test-model",
	}}

	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			ctrl := gomock.NewController(c)
			s := &addModelMocks{
				client:      mocks.NewMockJIMMAPI(ctrl),
				store:       mocks.NewMockClientStore(ctrl),
				cloudClient: mocks.NewMockAddModelCloudAPI(ctrl),
			}
			c.Cleanup(ctrl.Finish)

			jar, err := cookiejar.New(&cookiejar.Options{NoPersist: true})
			c.Assert(err, qt.IsNil)

			s.store.EXPECT().CurrentController().Return("test-controller", nil)
			s.store.EXPECT().ControllerByName("test-controller").Return(&jujuclient.ControllerDetails{}, nil).AnyTimes()
			s.store.EXPECT().CookieJar("test-controller").Return(jar, nil).AnyTimes()
			s.store.EXPECT().AccountDetails("test-controller").Return(&jujuclient.AccountDetails{
				User: "alice@canonical.com",
			}, nil)
			s.store.EXPECT().CredentialForCloud("test-cloud").Return(&cloud.CloudCredential{
				DefaultCredential: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/credAlice").String(),
			}, nil).AnyTimes()

			if test.expectCloudCall {
				s.cloudClient.EXPECT().Cloud(gomock.Any()).Return(cloud.Cloud{
					Name: "test-cloud",
					Type: "dummy",
					Regions: []cloud.Region{{
						Name: "test-region",
					}}}, nil)
			}

			if test.expectListUserClouds {
				s.client.EXPECT().ListUserClouds(&jimmapiparams.ListUserCloudsRequest{
					UserTag: names.NewUserTag("alice@canonical.com").String(),
				}).Return(map[names.CloudTag]cloud.Cloud{
					names.NewCloudTag("test-cloud"): {
						Name: "test-cloud",
						Type: "dummy",
						Regions: []cloud.Region{{
							Name: "test-region",
						}},
					},
				}, nil)
			}

			s.cloudClient.EXPECT().UserCredentials(names.NewUserTag("alice@canonical.com"), names.NewCloudTag("test-cloud")).Return([]names.CloudCredentialTag{names.NewCloudCredentialTag("test-cloud/alice@canonical.com/credAlice")}, nil)
			s.store.EXPECT().UpdateModel("test-controller", "alice@canonical.com/"+test.modelName, gomock.Any()).Return(nil)
			s.store.EXPECT().SetCurrentModel("test-controller", "alice@canonical.com/"+test.modelName).Return(nil)
			s.client.EXPECT().AddModelToController(gomock.Any()).Return(jujuparams.ModelInfo{
				Name:               test.modelName,
				UUID:               "test-uuid",
				Type:               "iaas",
				OwnerTag:           names.NewUserTag("alice@canonical.com").String(),
				CloudTag:           names.NewCloudTag("test-cloud").String(),
				CloudRegion:        "test-region",
				CloudCredentialTag: names.NewCloudCredentialTag("test-cloud/alice@canonical.com/credAlice").String(),
			}, nil)

			command := &addModelCommand{
				jimmAPIFunc: func(root api.Connection) JIMMAPI {
					return s.client
				},
				cloudAPIFunc: func(root api.Connection) AddModelCloudAPI {
					return s.cloudClient
				},
			}
			mCommand := modelcmd.WrapController(command)
			err = mCommand.SetControllerName("", true)
			c.Assert(err, qt.IsNil)
			mCommand.SetClientStore(s.store)
			command.apiRoot = &mockAPIConnection{}

			command.Name = test.modelName
			command.CloudRegion = test.cloudRegion
			command.CredentialName = test.credentialName
			command.targetController = "test-controller"

			initCommand(c, command,
				test.modelName,
				test.cloudRegion,
				"--target-controller", "test-controller",
				"--credential", test.credentialName)

			ctx := newTestContext(c)
			err = mCommand.Run(ctx)
			c.Assert(err, qt.IsNil)
			if test.credentialName != "" {
				stderr := cmdtesting.Stderr(ctx)
				c.Assert(stderr, qt.Contains, "Using credential 'credAlice' cached in controller\n")
			}
		})
	}
}

type mockAPIConnection struct {
	api.Connection
}

func (m *mockAPIConnection) Close() error {
	return nil
}
