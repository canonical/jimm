// Copyright 2025 Canonical.

package cmd

import (
	"os"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/cmd/v3/cmdtesting"
	controllerapi "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	jjclient "github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/cmd/jaas/cmd/mocks"
	apiparams "github.com/canonical/jimm/v3/pkg/api/params"
)

func setupMigrateAPIMock(c *qt.C) *mocks.MockMigrateAPI {
	ctrl := gomock.NewController(c)
	c.Cleanup(ctrl.Finish)
	return mocks.NewMockMigrateAPI(ctrl)
}

func TestMigrate(t *testing.T) {
	c := qt.New(t)

	mocks := setupCmdMocks(c)
	migrateClient := setupMigrateAPIMock(c)

	userMappingFile, err := os.CreateTemp(c.TempDir(), "")
	c.Assert(err, qt.IsNil)

	userMapping := `
# This is a comment
alice: alice@canonical.com
bob: bob@canonical.com
`
	_, err = userMappingFile.WriteString(userMapping)
	c.Assert(err, qt.IsNil)

	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"

	mocks.store.EXPECT().CurrentController().Return("target-controller", nil)
	mocks.store.EXPECT().ModelByName("target-controller", "owner/test-model").Return(&jjclient.ModelDetails{
		ModelUUID: testUUID,
	}, nil)

	migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{{
				UserName: "alice",
			}},
		}},
	}, nil)
	migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{{
		Users: []crossmodel.OfferUserDetails{
			{
				UserName: "bob",
			},
		}},
	}, nil)
	migrateClient.EXPECT().Close().Return(nil)

	prepareMigrateParams := &apiparams.PrepareModelMigrationRequest{
		BackingControllerName: "backing-controller",
		UserMapping:           map[string]string{"alice": "alice@canonical.com", "bob": "bob@canonical.com"},
		ModelTag:              "model-" + testUUID,
	}
	mocks.client.EXPECT().PrepareModelMigration(prepareMigrateParams).Return(apiparams.PrepareModelMigrationResponse{
		Token: "migration-token",
	}, nil)
	mocks.client.EXPECT().Close().Return(nil)

	mocks.store.EXPECT().AccountDetails("target-controller").Return(&jjclient.AccountDetails{
		User: "test-user",
	}, nil)
	mocks.store.EXPECT().ControllerByName("target-controller").Return(&jjclient.ControllerDetails{
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
	migrateClient.EXPECT().InitiateMigration(migrationSpec).Return("migration-id", nil)

	migrateCmd := &migrateModelCommand{
		jimmAPIFunc: func() (JIMMAPI, error) {
			return mocks.client, nil
		},
		jujuApiFunc: func() (MigrateAPI, error) {
			return migrateClient, nil
		},
	}
	migrateCmd.SetClientStore(mocks.store)

	initCommand(c, migrateCmd, "owner/test-model",
		"target-controller",
		"--backing-controller", "backing-controller",
		"--user-mapping", userMappingFile.Name(),
	)

	ctx := newTestContext(c)

	mcmd := modelcmd.WrapBase(migrateCmd)
	err = mcmd.Run(ctx)
	c.Assert(err, qt.IsNil)

	res := cmdtesting.Stdout(ctx)
	c.Assert(res, qt.Equals, "migration-id\n")
}

func TestMigrate_ReadUserMappingFileWithNull(t *testing.T) {
	c := qt.New(t)

	userMappingFile, err := os.CreateTemp(c.TempDir(), "")
	c.Assert(err, qt.IsNil)

	userMappingContent := `
alice: alice@canonical.com
bob: null
`
	_, err = userMappingFile.WriteString(userMappingContent)
	c.Assert(err, qt.IsNil)

	migrateCmd := &migrateModelCommand{
		userMappingFile: userMappingFile.Name(),
	}
	userMapping, err := migrateCmd.parseUserMappingFile()
	c.Assert(err, qt.IsNil)
	c.Assert(userMapping, qt.HasLen, 2)
	c.Assert(userMapping["alice"], qt.Equals, "alice@canonical.com")
	c.Assert(userMapping["bob"], qt.Equals, "")
}

func TestValidateUserMapping_SkipUsers(t *testing.T) {
	c := qt.New(t)
	migrateClient := setupMigrateAPIMock(c)

	userMapping := map[string]string{
		"alice": "alice@canonical.com",
		"bob":   "",
	}
	migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{
				{UserName: "alice"},
				{UserName: "bob"},
			},
		}},
	}, nil)
	migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{
		{
			Users: nil,
		},
	}, nil)
	migrateCmd := &migrateModelCommand{}
	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	err := migrateCmd.validateUserMapping(userMapping, testUUID, "user/foo", migrateClient)
	c.Assert(err, qt.IsNil)
}

func TestValidateUserMapping_HandleEveryoneUser(t *testing.T) {
	c := qt.New(t)
	migrateClient := setupMigrateAPIMock(c)

	userMapping := map[string]string{
		"alice": "alice@canonical.com",
	}
	migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{
				{UserName: "alice"},
				{UserName: "everyone@external"},
			},
		}},
	}, nil)
	migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{{
		Users: []crossmodel.OfferUserDetails{
			{
				UserName: "everyone@external",
			},
		}},
	}, nil)
	migrateCmd := &migrateModelCommand{}
	testUUID := "93608db4-f1cb-4da5-9926-8233981aef0a"
	err := migrateCmd.validateUserMapping(userMapping, testUUID, "user/foo", migrateClient)
	c.Assert(err, qt.IsNil)
}

func TestValidateUserMapping_MissingUsers(t *testing.T) {
	c := qt.New(t)
	migrateClient := setupMigrateAPIMock(c)

	userMapping := map[string]string{}
	migrateClient.EXPECT().ModelInfo(gomock.Any()).Return([]jujuparams.ModelInfoResult{{
		Result: &jujuparams.ModelInfo{
			Users: []jujuparams.ModelUserInfo{{
				UserName: "alice",
				Access:   jujuparams.ModelAdminAccess,
			}},
		}},
	}, nil)
	migrateClient.EXPECT().ListOffers(gomock.Any()).Return([]*crossmodel.ApplicationOfferDetails{
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
	err := migrateCmd.validateUserMapping(userMapping, testUUID, "user/foo", migrateClient)
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user "alice" who has admin access to the model$.*`)
	c.Assert(err, qt.ErrorMatches, `(?ms).*^expected user "bob" who has consume access to offer "test-offer"$.*`)
}

func TestReadUserMapping(t *testing.T) {
	c := qt.New(t)
	userMappingFile, err := os.CreateTemp(c.TempDir(), "")
	c.Assert(err, qt.IsNil)

	userMapping := `
# This is a comment
alice: alice@canonical.com
bob: bob@canonical.com
`
	_, err = userMappingFile.WriteString(userMapping)
	c.Assert(err, qt.IsNil)

	migrateCmd := &migrateModelCommand{
		userMappingFile: userMappingFile.Name(),
	}
	mapping, err := migrateCmd.parseUserMappingFile()
	c.Assert(err, qt.IsNil)
	c.Assert(mapping, qt.DeepEquals, map[string]string{
		"alice": "alice@canonical.com",
		"bob":   "bob@canonical.com",
	})
}

func TestReadUserMappingFailsWithEmptyYaml(t *testing.T) {
	c := qt.New(t)

	userMappingFile, err := os.CreateTemp(c.TempDir(), "")
	c.Assert(err, qt.IsNil)

	// Invalid YAML content
	_, err = userMappingFile.WriteString("")
	c.Assert(err, qt.IsNil)

	migrateCmd := &migrateModelCommand{
		userMappingFile: userMappingFile.Name(),
	}
	_, err = migrateCmd.parseUserMappingFile()
	c.Assert(err, qt.ErrorMatches, "user mapping file is empty or not properly formatted")
}

func TestCommandsFailsWithMissingArgs(t *testing.T) {
	c := qt.New(t)

	migrateCmd := &migrateModelCommand{}

	err := initCommandWithError(migrateCmd)
	c.Assert(err, qt.ErrorMatches, "missing controller name and model target arguments")
}
