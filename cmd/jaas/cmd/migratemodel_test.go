// Copyright 2025 Canonical.

// Note that this file is not an integration test
// because of limitations with the JujuConnSuite
// so it is placed under the cmd package.

package cmd

import (
	"context"
	"os"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/gnuflag"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	jjclient "github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

// migrateModelSuite is a test suite for the migrate model command.
// It does not perform integration tests like other suites because
// our test suite doesn't support spinning up multiple controllers
// so this behaviour is tested elsewhere instead.
type migrateModelSuite struct {
	jimmClient    *mocks.MockJIMMAPI
	migrateClient *mocks.MockMigrateAPI
	writer        *mocks.MockWriter
	store         *mocks.MockClientStore
}

var _ = gc.Suite(&migrateModelSuite{})

func (s *migrateModelSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.jimmClient = mocks.NewMockJIMMAPI(ctrl)
	s.migrateClient = mocks.NewMockMigrateAPI(ctrl)
	s.writer = mocks.NewMockWriter(ctrl)
	s.store = mocks.NewMockClientStore(ctrl)

	return ctrl
}

func (s *migrateModelSuite) TestMigrate(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, gc.IsNil)

	userMapping := `
# This is a comment
alice: alice@canonical.com
bob: bob@canonical.com
`
	_, err = userMappingFile.WriteString(userMapping)
	c.Assert(err, gc.IsNil)

	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"

	s.store.EXPECT().CurrentController().Return("target-controller", nil)
	s.store.EXPECT().ModelByName("target-controller", "owner/test-model").Return(&jjclient.ModelDetails{
		ModelUUID: testUUID,
	}, nil)

	s.migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{{
				UserName: "alice",
			}},
		}},
	}, nil)
	s.migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{{
		Users: []crossmodel.OfferUserDetails{
			{
				UserName: "bob",
			},
		}},
	}, nil)
	s.migrateClient.EXPECT().Close().Return(nil)

	prepareMigrateParams := &apiparams.PrepareModelMigrationRequest{
		BackingControllerName: "backing-controller",
		UserMapping:           map[string]string{"alice": "alice@canonical.com", "bob": "bob@canonical.com"},
		ModelTag:              "model-" + testUUID,
	}
	s.jimmClient.EXPECT().PrepareModelMigration(prepareMigrateParams).Return(apiparams.PrepareModelMigrationResponse{
		Token: "migration-token",
	}, nil)
	s.jimmClient.EXPECT().Close().Return(nil)

	s.store.EXPECT().AccountDetails("target-controller").Return(&jjclient.AccountDetails{
		User: "test-user",
	}, nil)
	s.store.EXPECT().ControllerByName("target-controller").Return(&jjclient.ControllerDetails{
		ControllerUUID: "target-controller-uuid",
		APIEndpoints:   []string{"endpoint1"},
		CACert:         "test-ca-cert",
	}, nil)

	migrationSpec := controllerapi.MigrationSpec{
		ModelUUID:             testUUID,
		SkipUserChecks:        true,
		TargetControllerUUID:  "target-controller-uuid",
		TargetControllerAlias: "target-controller",
		TargetAddrs:           []string{"endpoint1"},
		TargetCACert:          "test-ca-cert",
		TargetToken:           "migration-token",
		TargetUser:            "test-user",
	}
	s.migrateClient.EXPECT().InitiateMigration(migrationSpec).Return("migration-id", nil)

	s.writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		c.Check(string(b), gc.Equals, "migration-id\n")
		return len(b), nil
	})

	migrateCmd := &migrateModelCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return s.jimmClient, nil
		},
		jujuApiFunc: func() (MigrateAPI, error) {
			return s.migrateClient, nil
		},
		store: s.store,
	}
	f := gnuflag.NewFlagSet("test", gnuflag.ExitOnError)
	f.SetOutput(s.writer)
	migrateCmd.SetFlags(f)

	// Set args after settings flags to avoid resetting them.
	migrateCmd.targetController = "target-controller"
	migrateCmd.modelName = "owner/test-model"
	migrateCmd.backingController = "backing-controller"
	migrateCmd.userMappingFile = userMappingFile.Name()

	ctx := &cmd.Context{
		Context: context.Background(),
		Stdout:  s.writer,
	}
	err = migrateCmd.Run(ctx)
	c.Assert(err, gc.IsNil)
}

func (s *migrateModelSuite) TestMigrate_ReadUserMappingFileWithNull(c *gc.C) {
	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, gc.IsNil)

	userMappingContent := `
alice: alice@canonical.com
bob: null
`
	_, err = userMappingFile.WriteString(userMappingContent)
	c.Assert(err, gc.IsNil)

	migrateCmd := &migrateModelCommand{
		userMappingFile: userMappingFile.Name(),
	}
	userMapping, err := migrateCmd.parseUserMappingFile()
	c.Assert(err, gc.IsNil)
	c.Assert(userMapping, gc.HasLen, 2)
	c.Assert(userMapping["alice"], gc.Equals, "alice@canonical.com")
	c.Assert(userMapping["bob"], gc.Equals, "")
}

func (s *migrateModelSuite) TestValidateUserMapping_SkipUsers(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	userMapping := map[string]string{
		"alice": "alice@canonical.com",
		"bob":   "",
	}
	s.migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{
				{UserName: "alice"},
				{UserName: "bob"},
			},
		}},
	}, nil)
	s.migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{
		{
			Users: nil,
		},
	}, nil)
	migrateCmd := &migrateModelCommand{}
	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	err := migrateCmd.validateUserMapping(userMapping, testUUID, "user/foo", s.migrateClient)
	c.Assert(err, gc.IsNil)
}

func (s *migrateModelSuite) TestValidateUserMapping_HandleEveryoneUser(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	userMapping := map[string]string{
		"alice": "alice@canonical.com",
	}
	s.migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{
				{UserName: "alice"},
				{UserName: "everyone@external"},
			},
		}},
	}, nil)
	s.migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{{
		Users: []crossmodel.OfferUserDetails{
			{
				UserName: "everyone@external",
			},
		}},
	}, nil)
	migrateCmd := &migrateModelCommand{}
	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	err := migrateCmd.validateUserMapping(userMapping, testUUID, "user/foo", s.migrateClient)
	c.Assert(err, gc.IsNil)
}

func (s *migrateModelSuite) TestValidateUserMapping_MissingUsers(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	userMapping := map[string]string{}
	s.migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{{
				UserName: "alice",
				Access:   jujuparams.ModelAdminAccess,
			}},
		}},
	}, nil)
	s.migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{
		{
			OfferName: "test-offer",
			Users: []crossmodel.OfferUserDetails{{
				UserName: "bob",
				Access:   permission.ConsumeAccess,
			}},
		},
	}, nil)
	migrateCmd := &migrateModelCommand{}
	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	err := migrateCmd.validateUserMapping(userMapping, testUUID, "user/foo", s.migrateClient)
	c.Assert(err, gc.ErrorMatches, `(?ms).*^expected user "alice" who has admin access to the model$.*`)
	c.Assert(err, gc.ErrorMatches, `(?ms).*^expected user "bob" who has consume access to offer "test-offer"$.*`)
}

func (s *migrateModelSuite) TestReadUserMapping(c *gc.C) {
	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, gc.IsNil)

	userMapping := `
# This is a comment
alice: alice@canonical.com
bob: bob@canonical.com
`
	_, err = userMappingFile.WriteString(userMapping)
	c.Assert(err, gc.IsNil)

	migrateCmd := &migrateModelCommand{
		userMappingFile: userMappingFile.Name(),
	}
	mapping, err := migrateCmd.parseUserMappingFile()
	c.Assert(err, gc.IsNil)
	c.Assert(mapping, gc.DeepEquals, map[string]string{
		"alice": "alice@canonical.com",
		"bob":   "bob@canonical.com",
	})
}

func (s *migrateModelSuite) TestReadUserMappingFailsWithEmptyYaml(c *gc.C) {
	userMappingFile, err := os.CreateTemp(c.MkDir(), "")
	c.Assert(err, gc.IsNil)

	// Invalid YAML content
	_, err = userMappingFile.WriteString("")
	c.Assert(err, gc.IsNil)

	migrateCmd := &migrateModelCommand{
		userMappingFile: userMappingFile.Name(),
	}
	_, err = migrateCmd.parseUserMappingFile()
	c.Assert(err, gc.ErrorMatches, "user mapping file is empty or not properly formatted")
}

func (s *migrateModelSuite) TestCommandsFailsWithMissingArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, NewMigrateModelCommandForTesting(jjclient.NewMemStore(), nil), "myController")
	c.Assert(err, gc.ErrorMatches, "Missing controller name and model target arguments")
}
