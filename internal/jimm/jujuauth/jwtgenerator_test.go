// Copyright 2024 Canonical.

package jujuauth_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/names/v5"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/jujuauth"
	"github.com/canonical/jimm/v3/internal/jimmjwx"
	"github.com/canonical/jimm/v3/internal/openfga"
)

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

func TestJWTGeneratorMakeLoginToken(t *testing.T) {
	c := qt.New(t)

	ct := names.NewControllerTag(uuid.New().String())
	mt := names.NewModelTag(uuid.New().String())

	tests := []struct {
		about             string
		username          string
		database          *testDatabase
		accessChecker     *testAccessChecker
		jwtService        *testJWTService
		expectedError     string
		expectedJWTParams jimmjwx.JWTParams
	}{{
		about:    "initial login, all is well",
		username: "eve@canonical.com",
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
			User:       names.NewUserTag("eve@canonical.com").String(),
			Access: map[string]string{
				ct.String():                              "superuser",
				mt.String():                              "admin",
				names.NewCloudTag("test-cloud").String(): "add-model",
			},
		},
	}, {
		about:    "model access check fails",
		username: "eve@canonical.com",
		accessChecker: &testAccessChecker{
			modelAccessCheckErr: errors.E("a test error"),
		},
		jwtService:    &testJWTService{},
		expectedError: "a test error",
	}, {
		about:    "controller access check fails",
		username: "eve@canonical.com",
		accessChecker: &testAccessChecker{
			modelAccess: map[string]string{
				mt.String(): "admin",
			},
			controllerAccessCheckErr: errors.E("a test error"),
		},
		expectedError: "a test error",
	}, {
		about:    "get controller from db fails",
		username: "eve@canonical.com",
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
		about:    "cloud access check fails",
		username: "eve@canonical.com",
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
		about:    "jwt service errors out",
		username: "eve@canonical.com",
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
		generator := jujuauth.New(test.database, test.accessChecker, test.jwtService)
		generator.SetTags(mt, ct)

		i, err := dbmodel.NewIdentity(test.username)
		c.Assert(err, qt.IsNil)
		_, err = generator.MakeLoginToken(context.Background(), &openfga.User{
			Identity: i,
		})
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
			User:       names.NewUserTag("eve@canonical.com").String(),
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
			User:       names.NewUserTag("eve@canonical.com").String(),
			Access: map[string]string{
				ct.String():                              "superuser",
				mt.String():                              "admin",
				names.NewCloudTag("test-cloud").String(): "add-model",
				"entity1":                                "access_level1",
			},
		},
	}}

	for _, test := range tests {
		generator := jujuauth.New(
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

		i, err := dbmodel.NewIdentity("eve@canonical.com")
		c.Assert(err, qt.IsNil)
		_, err = generator.MakeLoginToken(context.Background(), &openfga.User{
			Identity: i,
		})
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
