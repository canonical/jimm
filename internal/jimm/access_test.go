// Copyright 2023 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmjwx"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"
)

// testAuthenticator is an authenticator implementation intended
// for testing the token generator.
type testAuthenticator struct {
	username string
	err      error
}

// Authenticate implements the Authenticate method of the Authenticator interface.
func (ta *testAuthenticator) Authenticate(ctx context.Context, req *jujuparams.LoginRequest) (*openfga.User, error) {
	if ta.err != nil {
		return nil, ta.err
	}
	user := &dbmodel.User{
		Username: ta.username,
	}
	return openfga.NewUser(user, nil), nil
}

// testDatabase is a database implementation intended for testing the token generator.
type testDatabase struct {
	ctl dbmodel.Controller
	err error
}

// GetController implements the GetController method of the JWTGeneratorDatabase interface.
func (tdb *testDatabase) GetController(ctx context.Context, controller *dbmodel.Controller) error {
	if tdb.err != nil {
		return tdb.err
	}
	*controller = tdb.ctl
	return nil
}

// testAccessChecker is an access checker implementation intended for testing the
// token generator.
type testAccessChecker struct {
	controllerAccess         map[string]string
	controllerAccessCheckErr error
	modelAccess              map[string]string
	modelAccessCheckErr      error
	cloudAccess              map[string]string
	cloudAccessCheckErr      error
	permissions              map[string]string
	permissionCheckErr       error
}

// GetUserModelAccess implements the GetUserModelAccess method of the JWTGeneratorAccessChecker interface.
func (tac *testAccessChecker) GetUserModelAccess(ctx context.Context, user *openfga.User, mt names.ModelTag) (string, error) {
	if tac.modelAccessCheckErr != nil {
		return "", tac.modelAccessCheckErr
	}
	return tac.modelAccess[mt.String()], nil
}

// GetUserControllerAccess implements the GetUserControllerAccess method of the JWTGeneratorAccessChecker interface.
func (tac *testAccessChecker) GetUserControllerAccess(ctx context.Context, user *openfga.User, ct names.ControllerTag) (string, error) {
	if tac.controllerAccessCheckErr != nil {
		return "", tac.controllerAccessCheckErr
	}
	return tac.controllerAccess[ct.String()], nil
}

// GetUserCloudAccess implements the GetUserCloudAccess method of the JWTGeneratorAccessChecker interface.
func (tac *testAccessChecker) GetUserCloudAccess(ctx context.Context, user *openfga.User, ct names.CloudTag) (string, error) {
	if tac.cloudAccessCheckErr != nil {
		return "", tac.cloudAccessCheckErr
	}
	return tac.cloudAccess[ct.String()], nil
}

// CheckPermission implements the CheckPermission methods of the JWTGeneratorAccessChecker interface.
func (tac *testAccessChecker) CheckPermission(ctx context.Context, user *openfga.User, accessMap map[string]string, permissions map[string]interface{}) (map[string]string, error) {
	if tac.permissionCheckErr != nil {
		return nil, tac.permissionCheckErr
	}
	access := make(map[string]string)
	for k, v := range accessMap {
		access[k] = v
	}
	for k, v := range tac.permissions {
		access[k] = v
	}
	return access, nil
}

// testJWTService is a jwt service implementation intended for testing the token generator.
type testJWTService struct {
	newJWTError error

	params jimmjwx.JWTParams
}

// NewJWT implements the NewJWT methods of the JWTService interface.
func (t *testJWTService) NewJWT(ctx context.Context, params jimmjwx.JWTParams) ([]byte, error) {
	if t.newJWTError != nil {
		return nil, t.newJWTError
	}
	t.params = params
	return []byte("test jwt"), nil
}

func TestAuditLogAccess(t *testing.T) {
	c := qt.New(t)

	ofgaClient, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: ofgaClient,
	}
	ctx := context.Background()

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	adminUser := openfga.NewUser(&dbmodel.User{Username: "alice"}, j.OpenFGAClient)
	err = adminUser.SetControllerAccess(ctx, j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	user := openfga.NewUser(&dbmodel.User{Username: "bob"}, j.OpenFGAClient)

	// admin user can grant other users audit log access.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.Tag())
	c.Assert(err, qt.IsNil)

	access := user.GetAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.AuditLogViewerRelation)

	// re-granting access does not result in error.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.Tag())
	c.Assert(err, qt.IsNil)

	// admin user can revoke other users audit log access.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.Tag())
	c.Assert(err, qt.IsNil)

	access = user.GetAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.NoRelation)

	// re-revoking access does not result in error.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.Tag())
	c.Assert(err, qt.IsNil)

	// non-admin user cannot grant audit log access
	err = j.GrantAuditLogAccess(ctx, user, adminUser.Tag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	// non-admin user cannot revoke audit log access
	err = j.RevokeAuditLogAccess(ctx, user, adminUser.Tag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func TestJWTGeneratorMakeLoginToken(t *testing.T) {
	c := qt.New(t)

	ct := names.NewControllerTag(uuid.New().String())
	mt := names.NewModelTag(uuid.New().String())

	tests := []struct {
		about             string
		authenticator     *testAuthenticator
		database          *testDatabase
		accessChecker     *testAccessChecker
		jwtService        *testJWTService
		expectedError     string
		expectedJWTParams jimmjwx.JWTParams
	}{{
		about: "initial login, all is well",
		authenticator: &testAuthenticator{
			username: "eve@external",
		},
		database: &testDatabase{
			ctl: dbmodel.Controller{
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					CloudRegion: dbmodel.CloudRegion{
						Cloud: dbmodel.Cloud{
							Name: "test-cloud",
						},
					},
				}},
			},
		},
		accessChecker: &testAccessChecker{
			modelAccess: map[string]string{
				mt.String(): "admin",
			},
			controllerAccess: map[string]string{
				ct.String(): "superuser",
			},
			cloudAccess: map[string]string{
				names.NewCloudTag("test-cloud").String(): "add-model",
			},
		},
		jwtService: &testJWTService{},
		expectedJWTParams: jimmjwx.JWTParams{
			Controller: ct.Id(),
			User:       names.NewUserTag("eve@external").String(),
			Access: map[string]string{
				ct.String():                              "superuser",
				mt.String():                              "admin",
				names.NewCloudTag("test-cloud").String(): "add-model",
			},
		},
	}, {
		about: "authorization fails",
		authenticator: &testAuthenticator{
			username: "eve@external",
			err:      errors.E("a test error"),
		},
		expectedError: "a test error",
	}, {
		about: "model access check fails",
		authenticator: &testAuthenticator{
			username: "eve@external",
		},
		accessChecker: &testAccessChecker{
			modelAccessCheckErr: errors.E("a test error"),
		},
		jwtService:    &testJWTService{},
		expectedError: "a test error",
	}, {
		about: "controller access check fails",
		authenticator: &testAuthenticator{
			username: "eve@external",
		},
		accessChecker: &testAccessChecker{
			modelAccess: map[string]string{
				mt.String(): "admin",
			},
			controllerAccessCheckErr: errors.E("a test error"),
		},
		expectedError: "a test error",
	}, {
		about: "get controller from db fails",
		authenticator: &testAuthenticator{
			username: "eve@external",
		},
		database: &testDatabase{
			err: errors.E("a test error"),
		},
		accessChecker: &testAccessChecker{
			modelAccess: map[string]string{
				mt.String(): "admin",
			},
			controllerAccess: map[string]string{
				ct.String(): "superuser",
			},
		},
		expectedError: "failed to fetch controller",
	}, {
		about: "cloud access check fails",
		authenticator: &testAuthenticator{
			username: "eve@external",
		},
		database: &testDatabase{
			ctl: dbmodel.Controller{
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					CloudRegion: dbmodel.CloudRegion{
						Cloud: dbmodel.Cloud{
							Name: "test-cloud",
						},
					},
				}},
			},
		},
		accessChecker: &testAccessChecker{
			modelAccess: map[string]string{
				mt.String(): "admin",
			},
			controllerAccess: map[string]string{
				ct.String(): "superuser",
			},
			cloudAccessCheckErr: errors.E("a test error"),
		},
		expectedError: "failed to check user's cloud access",
	}, {
		about: "jwt service errors out",
		authenticator: &testAuthenticator{
			username: "eve@external",
		},
		database: &testDatabase{
			ctl: dbmodel.Controller{
				CloudRegions: []dbmodel.CloudRegionControllerPriority{{
					CloudRegion: dbmodel.CloudRegion{
						Cloud: dbmodel.Cloud{
							Name: "test-cloud",
						},
					},
				}},
			},
		},
		accessChecker: &testAccessChecker{
			modelAccess: map[string]string{
				mt.String(): "admin",
			},
			controllerAccess: map[string]string{
				ct.String(): "superuser",
			},
			cloudAccess: map[string]string{
				names.NewCloudTag("test-cloud").String(): "add-model",
			},
		},
		jwtService: &testJWTService{
			newJWTError: errors.E("a test error"),
		},
		expectedError: "a test error",
	}}

	for _, test := range tests {
		generator := jimm.NewJWTGenerator(test.authenticator, test.database, test.accessChecker, test.jwtService)
		generator.SetTags(mt, ct)

		_, err := generator.MakeLoginToken(context.Background(), &jujuparams.LoginRequest{})
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(test.jwtService.params, qt.DeepEquals, test.expectedJWTParams)
		}
	}
}

func TestJWTGeneratorMakeToken(t *testing.T) {
	c := qt.New(t)

	ct := names.NewControllerTag(uuid.New().String())
	mt := names.NewModelTag(uuid.New().String())

	tests := []struct {
		about                 string
		checkPermissions      map[string]string
		checkPermissionsError error
		jwtService            *testJWTService
		expectedError         string
		permissions           map[string]interface{}
		expectedJWTParams     jimmjwx.JWTParams
	}{{
		about:      "all is well",
		jwtService: &testJWTService{},
		expectedJWTParams: jimmjwx.JWTParams{
			Controller: ct.Id(),
			User:       names.NewUserTag("eve@external").String(),
			Access: map[string]string{
				ct.String():                              "superuser",
				mt.String():                              "admin",
				names.NewCloudTag("test-cloud").String(): "add-model",
			},
		},
	}, {
		about:      "check permission fails",
		jwtService: &testJWTService{},
		permissions: map[string]interface{}{
			"entity1": "access_level1",
		},
		checkPermissionsError: errors.E("a test error"),
		expectedError:         "a test error",
	}, {
		about:      "additional permissions need checking",
		jwtService: &testJWTService{},
		permissions: map[string]interface{}{
			"entity1": "access_level1",
		},
		checkPermissions: map[string]string{
			"entity1": "access_level1",
		},
		expectedJWTParams: jimmjwx.JWTParams{
			Controller: ct.Id(),
			User:       names.NewUserTag("eve@external").String(),
			Access: map[string]string{
				ct.String():                              "superuser",
				mt.String():                              "admin",
				names.NewCloudTag("test-cloud").String(): "add-model",
				"entity1":                                "access_level1",
			},
		},
	}}

	for _, test := range tests {
		generator := jimm.NewJWTGenerator(
			&testAuthenticator{
				username: "eve@external",
			},
			&testDatabase{
				ctl: dbmodel.Controller{
					CloudRegions: []dbmodel.CloudRegionControllerPriority{{
						CloudRegion: dbmodel.CloudRegion{
							Cloud: dbmodel.Cloud{
								Name: "test-cloud",
							},
						},
					}},
				},
			},
			&testAccessChecker{
				modelAccess: map[string]string{
					mt.String(): "admin",
				},
				controllerAccess: map[string]string{
					ct.String(): "superuser",
				},
				cloudAccess: map[string]string{
					names.NewCloudTag("test-cloud").String(): "add-model",
				},
				permissions:        test.checkPermissions,
				permissionCheckErr: test.checkPermissionsError,
			},
			test.jwtService,
		)
		generator.SetTags(mt, ct)

		_, err := generator.MakeLoginToken(context.Background(), &jujuparams.LoginRequest{})
		c.Assert(err, qt.IsNil)

		_, err = generator.MakeToken(context.Background(), test.permissions)
		if test.expectedError != "" {
			c.Assert(err, qt.ErrorMatches, test.expectedError)
		} else {
			c.Assert(err, qt.IsNil)
			c.Assert(test.jwtService.params, qt.DeepEquals, test.expectedJWTParams)
		}
	}
}
