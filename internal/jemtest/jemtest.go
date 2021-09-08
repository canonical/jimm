package jemtest

import (
	"context"

	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

const (
	TestProviderType          = "dummy"
	TestCloudName             = "dummy"
	TestCloudRegionName       = "dummy-region"
	TestCloudEndpoint         = "dummy-endpoint"
	TestCloudIdentityEndpoint = "dummy-identity-endpoint"
	TestCloudStorageEndpoint  = "dummy-storage-endpoint"
)

type JEMSuite struct {
	JujuConnSuite

	// Params contains the parameters that are used to create the jem Pool
	// SetUpTest will create default values for the following fields if
	// they aren't set:
	//
	//     Database
	//     SessionPool
	//     ControllerAdmin
	//     PublicCloudMetadata
	Params jem.Params

	// The following fields are populated by SetUpTest.
	SessionPool *mgosession.Pool
	Pool        *jem.Pool
	JEM         *jem.JEM
}

func (s *JEMSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	if s.Params.DB == nil {
		s.Params.DB = s.Session.DB("jemtest")
		s.AddCleanup(func(c *gc.C) {
			s.Params.DB = nil
		})
	}
	if s.Params.SessionPool == nil {
		s.SessionPool = mgosession.NewPool(context.TODO(), s.Session, 10)
		s.Params.SessionPool = s.SessionPool
		s.AddCleanup(func(c *gc.C) {
			s.Params.SessionPool = nil
		})
	}
	if len(s.Params.ControllerAdmins) == 0 {
		s.Params.ControllerAdmins = []params.User{ControllerAdmin}
		s.AddCleanup(func(c *gc.C) {
			s.Params.ControllerAdmins = nil
		})
	}
	var err error
	if s.Params.PublicCloudMetadata == nil {
		s.Params.PublicCloudMetadata, _, err = cloud.PublicCloudMetadata()
		c.Assert(err, gc.Equals, nil)
		s.AddCleanup(func(c *gc.C) {
			s.Params.PublicCloudMetadata = nil
		})
	}
	s.Pool, err = jem.NewPool(context.TODO(), s.Params)
	c.Assert(err, gc.Equals, nil)

	s.JEM = s.Pool.JEM(context.TODO())
}

func (s *JEMSuite) TearDownTest(c *gc.C) {
	if s.JEM != nil {
		s.JEM.Close()
		s.JEM = nil
	}
	if s.Pool != nil {
		s.Pool.Close()
		s.Pool = nil
	}
	if s.SessionPool != nil {
		s.SessionPool.Close()
		s.SessionPool = nil
	}
	s.JujuConnSuite.TearDownTest(c)
}

// AddController adds a new controller to the system. The given controller
// document must specify the path for the new controller. The rest of the
// details will be completed from the JujuConnSuite's controller info if
// not specified. All controllers added by this method are public.
func (s *JEMSuite) AddController(c *gc.C, ctl *mongodoc.Controller) {
	if ctl.Path.User == "" || ctl.Path.Name == "" {
		c.Fatalf("controller path %q not fully defined.", ctl.Path)
	}
	info := s.APIInfo(c)
	if len(ctl.HostPorts) == 0 {
		hps, err := mongodoc.ParseAddresses(info.Addrs)
		c.Assert(err, gc.Equals, nil)
		ctl.HostPorts = [][]mongodoc.HostPort{hps}
	}
	if ctl.CACert == "" {
		ctl.CACert = info.CACert
	}
	if ctl.AdminUser == "" {
		ctl.AdminUser = info.Tag.Id()
	}
	if ctl.AdminPassword == "" {
		ctl.AdminPassword = info.Password
	}
	ctl.Public = true
	err := s.JEM.AddController(context.TODO(), NewIdentity(string(ctl.Path.User), ControllerAdmin), ctl)
	c.Assert(err, gc.Equals, nil)
}

// UpdateCredential updates the given credential.
func (s *JEMSuite) UpdateCredential(c *gc.C, cred *mongodoc.Credential) {
	_, err := s.JEM.UpdateCredential(context.TODO(), NewIdentity(string(cred.Path.User)), cred, jem.CredentialUpdate)
	c.Assert(err, gc.Equals, nil)
}

// CreateModel creates a new model based on the given model document. After
// Completion this model document will contain all of the model details. If
// mi is non-nil then the ModelInfo returned from CreateModel will be
// written to it. Any uers in the access map will be granted the specified
// access level on the newly created model.
func (s *JEMSuite) CreateModel(c *gc.C, m *mongodoc.Model, mi *jujuparams.ModelInfo, access map[params.User]jujuparams.UserAccessPermission) {
	params := jem.CreateModelParams{
		Path:           m.Path,
		ControllerPath: m.Controller,
		Credential:     m.Credential,
		Cloud:          m.Cloud,
		Region:         m.CloudRegion,
	}
	id := NewIdentity(string(m.Path.User))
	err := s.JEM.CreateModel(context.TODO(), id, params, mi)
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.GetModel(context.TODO(), id, jujuparams.ModelAdminAccess, m)
	c.Assert(err, gc.Equals, nil)
	for k, v := range access {
		err = s.JEM.GrantModel(context.TODO(), id, m, k, v)
		c.Assert(err, gc.Equals, nil)
	}
}

// A BoostrapSuite is a suite that adds a controller and a model in the
// SetUpTest.
type BootstrapSuite struct {
	JEMSuite

	Controller mongodoc.Controller
	Credential mongodoc.Credential
	Model      mongodoc.Model
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	s.JEMSuite.SetUpTest(c)

	s.Controller = mongodoc.Controller{
		Path: params.EntityPath{User: ControllerAdmin, Name: "controller-1"},
	}
	s.AddController(c, &s.Controller)

	s.Credential = EmptyCredential("bob", "cred")
	s.UpdateCredential(c, &s.Credential)
	s.Model = mongodoc.Model{
		Path:       params.EntityPath{User: "bob", Name: "model-1"},
		Controller: s.Controller.Path,
		Credential: s.Credential.Path,
	}
	s.CreateModel(c, &s.Model, nil, nil)
}

// CreateModel creates a new model based on the given model document. If
// the given model document does not specify either a Controller, Cloud, or
// Credential then the default controller will be used. After completion
// this model document will contain all of the model details. If mi is
// non-nil then the ModelInfo returned from CreateModel will be written to
// it. Any uers in the access map will be granted the specified access
// level on the newly created model.
func (s *BootstrapSuite) CreateModel(c *gc.C, m *mongodoc.Model, mi *jujuparams.ModelInfo, access map[params.User]jujuparams.UserAccessPermission) {
	if m.Cloud == "" && !m.Credential.IsZero() {
		m.Cloud = params.Cloud(m.Credential.Cloud)
	}
	if m.Controller.IsZero() && m.Cloud == "" {
		m.Controller = s.Controller.Path
	}
	s.JEMSuite.CreateModel(c, m, mi, access)
}

func EmptyCredential(user, name string) mongodoc.Credential {
	return mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			Cloud: TestCloudName,
			EntityPath: mongodoc.EntityPath{
				User: user,
				Name: name,
			},
		},
		Type:         "empty",
		ProviderType: TestProviderType,
	}
}
