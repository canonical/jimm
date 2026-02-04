// Copyright 2026 Canonical.

package jimmtest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"

	cofga "github.com/canonical/ofga"
	petname "github.com/dustinkirkland/golang-petname"
	"github.com/go-chi/chi/v5"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/core/constraints"
	jclient "github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/jsoncodec"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimmhttp"
	"github.com/canonical/jimm/v3/internal/jujuapi"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/utils"
)

const (
	// ControllersConfigEnvVar is the environment variable that specifies
	// the path to the controllers config file.
	ControllersConfigEnvVar = "JIMM_BACKING_CONTROLLER_CONFIG"
	TestE2EProviderType     = "lxd"
	TestE2ECloudName        = "localhost"
	TestE2ECloudRegionName  = "localhost"
	Microk8sCloudNameEnv    = "JIMM_MICROK8S_TEST_CLOUD_NAME"
)

// ControllerInfo holds the controller connection information
// retrieved from the configuration file.
type ControllerInfo struct {
	UUID     string   `yaml:"uuid"`
	Addrs    []string `yaml:"addrs"`
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	CACert   string   `yaml:"ca-cert"`
}

// ControllersConfig holds the top-level structure of the controller.yaml file.
type ControllersConfig struct {
	Controllers map[string]ControllerInfo `yaml:"controllers"`
}

// GetControllersConfig reads and parses the controller.yaml configuration file
// from the path specified in the JIMM_BACKING_CONTROLLER_CONFIG environment variable.
func (s *E2ESuite) GetControllersConfig(c *gc.C) *ControllersConfig {
	configPath := os.Getenv(ControllersConfigEnvVar)
	c.Assert(configPath, gc.Not(gc.Equals), "", gc.Commentf(
		"%s environment variable is not set. "+
			"Set it to the path of your controllers.yaml file or configure it in VS Code settings.",
		ControllersConfigEnvVar))

	data, err := os.ReadFile(configPath)
	c.Assert(err, gc.IsNil, gc.Commentf(
		"failed to read controller config file: %s. "+
			"Generate it using 'make generate-test-env'", configPath))

	var config ControllersConfig
	err = yaml.Unmarshal(data, &config)
	c.Assert(err, gc.IsNil, gc.Commentf("failed to parse controller config file"))

	c.Assert(len(config.Controllers), gc.Not(gc.Equals), 0, gc.Commentf("no controllers found in config"))

	// If cert ending newline was lost to YAML parsing, add it back
	for name, info := range config.Controllers {
		if !strings.HasPrefix(info.CACert, "\n") {
			info.CACert += "\n"
			config.Controllers[name] = info
		}
	}

	return &config
}

// GetControllerConfig retrieves the controller information
// for the given controller name from the controllers config file.
func (s *E2ESuite) GetControllerConfig(c *gc.C, name string) ControllerInfo {
	config := s.GetControllersConfig(c)
	info, ok := config.Controllers[name]
	c.Assert(ok, gc.Equals, true, gc.Commentf("controller %q not found in config", name))
	return info
}

// GetOneControllerConfig retrieves one controller configuration
// from the controllers config file. It can be used in tests when
// a valid controller config is needed.
func (s *E2ESuite) GetOneControllerConfig(c *gc.C) (string, ControllerInfo) {
	config := s.GetControllersConfig(c)
	for name, info := range config.Controllers {
		return name, info
	}
	c.Fatal("no controllers found in config")
	return "", ControllerInfo{}
}

// Validate asserts that all required fields are set for this controller.
func (info *ControllerInfo) Validate(c *gc.C, name string) {
	c.Assert(info.UUID, gc.Not(gc.Equals), "", gc.Commentf("uuid is not set for controller %q", name))
	c.Assert(len(info.Addrs), gc.Not(gc.Equals), 0, gc.Commentf("addrs is not set for controller %q", name))
	c.Assert(info.Username, gc.Not(gc.Equals), "", gc.Commentf("username is not set for controller %q", name))
	c.Assert(info.Password, gc.Not(gc.Equals), "", gc.Commentf("password is not set for controller %q", name))
	c.Assert(info.CACert, gc.Not(gc.Equals), "", gc.Commentf("ca-cert is not set for controller %q", name))
}

// ToAPIInfo converts the ControllerInfo to a juju api.Info struct.
func (info *ControllerInfo) ToAPIInfo() *api.Info {
	return &api.Info{
		ControllerUUID: info.UUID,
		Addrs:          info.Addrs,
		Tag:            names.NewUserTag(info.Username),
		Password:       info.Password,
		CACert:         info.CACert,
	}
}

// WebsocketE2ESuite is a suite that initialises a JIMM with
// an externally bootstrapped controller, and provides
// methods to open websocket connections to the JIMM API.
type WebsocketE2ESuite struct {
	E2ESuite

	Params     jujuapi.Params
	APIHandler http.Handler
	HTTP       *httptest.Server

	Credential2 *dbmodel.CloudCredential
	Model2      *dbmodel.Model
	Model3      *dbmodel.Model

	cancelFnc context.CancelFunc
}

type LoginDetails struct {
	Info          *api.Info
	Username      string
	Lp            api.LoginProvider
	DialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error)
}

func (s *WebsocketE2ESuite) SetUpTest(c *gc.C) {
	ctx, cancelFnc := context.WithCancel(context.Background())
	s.cancelFnc = cancelFnc

	s.E2ESuite.SetUpTest(c)

	s.Params.ControllerUUID = ControllerUUID

	mux := chi.NewRouter()
	mountHandler := func(path string, h jimmhttp.JIMMHttpHandler) {
		mux.Mount(path, h.Routes())
	}
	mux.Handle("/api", jujuapi.APIHandler(ctx, s.JIMM, s.Params))
	mountHandler(
		"/model/{uuid}/{type:charms|applications}",
		jimmhttp.NewHTTPProxyHandler(s.JIMM),
	)
	mux.Handle("/model/*", http.StripPrefix("/model", jujuapi.ModelHandler(ctx, s.JIMM, s.Params)))
	jwks := jimmhttp.NewWellKnownHandler(s.JIMM.CredentialStore)
	mux.HandleFunc("/.well-known/jwks.json", jwks.JWKS)
	mux.Handle("/migrate/logtransfer", jujuapi.LogTransferHandler(ctx, s.JIMM, s.Params))

	s.APIHandler = mux
	s.HTTP = httptest.NewTLSServer(s.APIHandler)

	s.AddAdminUser(c, "alice@canonical.com")

	cct := names.NewCloudCredentialTag(TestE2ECloudName + "/charlie@canonical.com/cred")
	cloudCredentials := s.GetExistingClientCredentialsForCloud(c, TestE2ECloudName)
	s.UpdateCloudCredential(c, cct, cloudCredentials)
	s.Credential2 = new(dbmodel.CloudCredential)
	s.Credential2.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.Credential2)
	c.Assert(err, gc.Equals, nil)
	mt := s.AddModel(c, names.NewUserTag("charlie@canonical.com"), petname.Generate(2, "-"), names.NewCloudTag(TestE2ECloudName), TestE2ECloudRegionName, cct)
	s.Model2 = new(dbmodel.Model)
	s.Model2.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model2)
	c.Assert(err, gc.Equals, nil)
	mt = s.AddModel(c, names.NewUserTag("charlie@canonical.com"), petname.Generate(2, "-"), names.NewCloudTag(TestE2ECloudName), TestE2ECloudRegionName, cct)
	s.Model3 = new(dbmodel.Model)
	s.Model3.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model3)
	c.Assert(err, gc.Equals, nil)

	bobIdentity, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, gc.IsNil)

	bob := openfga.NewUser(
		bobIdentity,
		s.OFGAClient,
	)
	err = bob.SetModelAccess(ctx, s.Model3.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, gc.Equals, nil)
}

type DeployApplicationParams struct {
	App     string
	Charm   string
	Channel string
}

// DeployApplication deploys a charm in the specified model as the given user.
func (s *WebsocketE2ESuite) DeployApplication(c *gc.C, user *openfga.User, modelTag names.Tag, params DeployApplicationParams) {
	modelTagConv, ok := modelTag.(names.ModelTag)
	c.Assert(ok, gc.Equals, true)

	conn := s.Open(c, nil, user.Name, &modelTagConv)
	defer conn.Close()

	client := application.NewClient(conn)
	channel := utils.Ptr("latest/stable")
	if params.Channel != "" {
		channel = &params.Channel
	}

	_, _, errs := client.DeployFromRepository(application.DeployFromRepositoryArg{
		CharmName:       params.Charm,
		ApplicationName: params.App,
		Channel:         channel,
		NumUnits:        utils.Ptr(1),
		Cons:            constraints.Value{Arch: utils.Ptr(runtime.GOARCH)},
	})
	for _, err := range errs {
		c.Assert(err, gc.Equals, nil)
	}
}

func (s *WebsocketE2ESuite) TearDownTest(c *gc.C) {
	if s.cancelFnc != nil {
		s.cancelFnc()
	}
	if s.HTTP != nil {
		s.HTTP.Close()
	}
	s.E2ESuite.TearDownTest(c)
}

// OpenNoAssert creates a new websocket connection to the test server, using the
// connection info specified in info, authenticating as the given user.
// If info is nil then default values will be used.
func (s *WebsocketE2ESuite) OpenNoAssert(c *gc.C, d LoginDetails, modelTag *names.ModelTag) (api.Connection, error) {
	var inf api.Info
	if d.Info != nil {
		inf = *d.Info
	}
	u, err := url.Parse(s.HTTP.URL)
	c.Assert(err, gc.Equals, nil)
	inf.Addrs = []string{
		u.Host,
	}
	w := new(bytes.Buffer)
	err = pem.Encode(w, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: s.HTTP.TLS.Certificates[0].Certificate[0],
	})
	c.Assert(err, gc.Equals, nil)
	inf.CACert = w.String()
	if modelTag != nil {
		inf.ModelTag = *modelTag
	}

	if d.Lp == nil {
		d.Lp = NewUserSessionLogin(c, d.Username)
	}

	dialOpts := api.DialOpts{
		InsecureSkipVerify: true,
		LoginProvider:      d.Lp,
	}

	if d.DialWebsocket != nil {
		dialOpts.DialWebsocket = d.DialWebsocket
	}

	return api.Open(&inf, dialOpts)
}

func (s *WebsocketE2ESuite) Open(c *gc.C, info *api.Info, username string, modelTag *names.ModelTag) api.Connection {
	ld := LoginDetails{Info: info, Username: username}
	conn, err := s.OpenNoAssert(c, ld, modelTag)
	c.Assert(err, gc.Equals, nil)
	return conn
}

func (s *WebsocketE2ESuite) OpenWithDialWebsocket(
	c *gc.C,
	info *api.Info,
	username string,
	dialWebsocket func(ctx context.Context, urlStr string, tlsConfig *tls.Config, ipAddr string) (jsoncodec.JSONConn, error),
) api.Connection {
	ld := LoginDetails{Info: info, Username: username, DialWebsocket: dialWebsocket}
	conn, err := s.OpenNoAssert(c, ld, nil)
	c.Assert(err, gc.Equals, nil)
	return conn
}

// E2ESuite is a suite that initialises a JIMM with
// an externally bootstrapped controller.
// It creates cloud credential, and creates a model.
type E2ESuite struct {
	JIMMSuite
	LoggingSuite

	CloudCredential *dbmodel.CloudCredential
	Model           *dbmodel.Model

	// cleanupFuncs holds functions to be called during TearDownTest.
	cleanupFuncs []func()
	// mutex to protect cleanupFuncs slice
	// tests are not run in parallel, but multiple goroutines
	// may add cleanup functions.
	mutexCleanup sync.Mutex
}

func (s *E2ESuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *E2ESuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *E2ESuite) SetUpTest(c *gc.C) {
	s.cleanupFuncs = nil
	s.UseHardcodedJWKS(c)
	s.JIMMSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)

	// Add all controllers from the config file
	config := s.GetControllersConfig(c)
	for name, info := range config.Controllers {
		info.Validate(c, name)
		controller := s.AddController(c, name, info.ToAPIInfo())

		// Grant fixture users bob and charlie with permission to
		// add models to the controller.
		err := s.OFGAClient.AddRelation(context.Background(), cofga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("bob@canonical.com")),
			Relation: ofganames.CanAddModelRelation,
			Target:   ofganames.ConvertTag(controller.ResourceTag()),
		}, cofga.Tuple{
			Object:   ofganames.ConvertTag(names.NewUserTag("charlie@canonical.com")),
			Relation: ofganames.CanAddModelRelation,
			Target:   ofganames.ConvertTag(controller.ResourceTag()),
		})
		c.Assert(err, gc.Equals, nil)
	}

	cct := names.NewCloudCredentialTag(TestE2ECloudName + "/bob@canonical.com/cred")
	cloudCredentials := s.GetExistingClientCredentialsForCloud(c, TestE2ECloudName)
	s.UpdateCloudCredential(c, cct, cloudCredentials)
	ctx := context.Background()
	s.CloudCredential = new(dbmodel.CloudCredential)
	s.CloudCredential.SetTag(cct)
	err := s.JIMM.Database.GetCloudCredential(ctx, s.CloudCredential)
	c.Assert(err, gc.Equals, nil)

	mt := s.AddModel(c, names.NewUserTag("bob@canonical.com"), petname.Generate(2, "-"), names.NewCloudTag(TestE2ECloudName), TestE2ECloudName, cct)
	s.Model = new(dbmodel.Model)
	s.Model.SetTag(mt)
	err = s.JIMM.Database.GetModel(ctx, s.Model)
	c.Assert(err, gc.Equals, nil)
}

// Cleanup registers a function to be called during TearDownTest.
func (s *E2ESuite) Cleanup(f func()) {
	s.mutexCleanup.Lock()
	defer s.mutexCleanup.Unlock()
	s.cleanupFuncs = append(s.cleanupFuncs, f)
}

func (s *E2ESuite) AddModel(c *gc.C, owner names.UserTag, name string, cloud names.CloudTag, region string, cred names.CloudCredentialTag) names.ModelTag {
	mt := s.JIMMSuite.AddModel(c, addModelArgs{
		owner:  owner,
		name:   name,
		cloud:  cloud,
		region: region,
		cred:   cred,
	})
	s.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, mt)
	})
	return mt
}

func (s *E2ESuite) AddModelToController(c *gc.C, owner names.UserTag, name string, cloud names.CloudTag, region string, cred names.CloudCredentialTag, controllerName string) names.ModelTag {
	mt := s.JIMMSuite.AddModel(c, addModelArgs{
		owner:                owner,
		name:                 name,
		cloud:                cloud,
		region:               region,
		cred:                 cred,
		targetControllerName: controllerName,
	})
	s.Cleanup(func() {
		s.DestroyModelAndDeleteFromDatabase(c, mt)
	})
	return mt
}

func (s *E2ESuite) TearDownTest(c *gc.C) {
	// Run cleanup functions in reverse order (LIFO), matching *testing.T.Cleanup behavior
	for i := len(s.cleanupFuncs) - 1; i >= 0; i-- {
		s.cleanupFuncs[i]()
	}
	s.JIMMSuite.TearDownTest(c)
}

func (s *E2ESuite) GetExistingClientCredentialsForCloud(c *gc.C, cloudName string) jujuparams.CloudCredential {
	store := jclient.NewFileClientStore()
	existingCredentials, err := store.AllCredentials()
	c.Assert(err, gc.IsNil)
	cred, ok := existingCredentials[cloudName]
	c.Assert(ok, gc.Equals, true)
	cloudCredentials := jujuparams.CloudCredential{}
	for _, cred := range cred.AuthCredentials {
		cloudCredentials.AuthType = string(cred.AuthType())
		cloudCredentials.Attributes = cred.Attributes()
		break
	}
	return cloudCredentials
}

// GetMicrok8sCloudAndCloudCredential retrieves the microk8s cloud
// and corresponding cloud credential for use in tests.
// The name of the microk8s cloud is read from the
// JIMM_MICROK8S_TEST_CLOUD_NAME environment variable.
func (s *E2ESuite) GetMicrok8sCloudAndCloudCredential(c *gc.C) (cloud.Cloud, jujuparams.CloudCredential) {
	testCloudName := os.Getenv(Microk8sCloudNameEnv)
	c.Assert(testCloudName, gc.Not(gc.Equals), "", gc.Commentf(
		"%s environment variable is not set. "+
			"Set it to the name of your microk8s cloud added to juju or configure it in VS Code settings.",
		Microk8sCloudNameEnv))
	cloud, err := common.CloudByName(testCloudName)
	c.Assert(err, gc.IsNil)
	cloud.HostCloudRegion = TestE2EProviderType + "/" + TestE2ECloudRegionName
	cloud.Name = petname.Generate(2, "-")
	credential := s.GetExistingClientCredentialsForCloud(c, testCloudName)
	return *cloud, credential
}
