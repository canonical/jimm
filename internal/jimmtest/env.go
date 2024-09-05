// Copyright 2024 Canonical.

package jimmtest

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"sigs.k8s.io/yaml"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
)

const (
	TestProviderType          = "dummy"
	TestCloudName             = "dummy"
	TestCloudRegionName       = "dummy-region"
	TestCloudEndpoint         = "dummy-endpoint"
	TestCloudIdentityEndpoint = "dummy-identity-endpoint"
	TestCloudStorageEndpoint  = "dummy-storage-endpoint"
)

type Environment struct {
	Clouds           []Cloud           `json:"clouds"`
	CloudCredentials []CloudCredential `json:"cloud-credentials"`
	CloudDefaults    []CloudDefaults   `json:"cloud-defaults"`
	Controllers      []Controller      `json:"controllers"`
	Models           []Model           `json:"models"`
	Users            []User            `json:"users"`
	UserDefaults     []UserDefaults    `json:"user-defaults"`
}

func ParseEnvironment(c Tester, env string) *Environment {
	var e Environment

	err := yaml.Unmarshal([]byte(env), &e)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return &e
}

func (e *Environment) Cloud(name string) *Cloud {
	for i := range e.Clouds {
		if e.Clouds[i].Name == name {
			e.Clouds[i].env = e
			return &e.Clouds[i]
		}
	}
	return nil
}

func (e *Environment) CloudCredential(owner, cloud, name string) *CloudCredential {
	for i := range e.CloudCredentials {
		if e.CloudCredentials[i].Name == name {
			return &e.CloudCredentials[i]
		}
	}
	return nil
}

func (e *Environment) CloudDefault(owner, cloud string) *CloudDefaults {
	for i := range e.CloudDefaults {
		if e.CloudDefaults[i].User == owner && e.CloudDefaults[i].Cloud == cloud {
			return &e.CloudDefaults[i]
		}
	}
	return nil
}

func (e *Environment) UserDefault(owner string) *UserDefaults {
	for i := range e.UserDefaults {
		if e.UserDefaults[i].User == owner {
			return &e.UserDefaults[i]
		}
	}
	return nil
}

func (e *Environment) Controller(name string) *Controller {
	for i := range e.Controllers {
		if e.Controllers[i].Name == name {
			e.Controllers[i].env = e
			return &e.Controllers[i]
		}
	}
	return nil
}

func (e *Environment) Model(owner, name string) *Model {
	for i := range e.Models {
		if e.Models[i].Owner == owner && e.Models[i].Name == name {
			e.Models[i].env = e
			return &e.Models[i]
		}
	}
	return nil
}

func (e *Environment) User(name string) *User {
	for i := range e.Users {
		if e.Users[i].Username == name {
			e.Users[i].env = e
			return &e.Users[i]
		}
	}
	u := User{
		Username: name,
		env:      e,
	}
	e.Users = append(e.Users, u)
	return &e.Users[len(e.Users)-1]
}

// addUserRelations adds permissions the user should have.
func (u User) addUserRelations(c *qt.C, jimmTag names.ControllerTag, db db.Database, client *openfga.OFGAClient) {
	if u.ControllerAccess == "superuser" {
		dbUser := u.DBObject(c, db)
		u := openfga.NewUser(&dbUser, client)
		err := u.SetControllerAccess(context.Background(), jimmTag, ofganames.AdministratorRelation)
		c.Assert(err, qt.IsNil)
	}
}

// addCloudRelations adds permissions the cloud should have and adds permissions for users to the cloud.
func (cl Cloud) addCloudRelations(c *qt.C, db db.Database, client *openfga.OFGAClient) {
	for _, u := range cl.Users {
		dbUser := cl.env.User(u.User).DBObject(c, db)
		var relation openfga.Relation
		switch u.Access {
		case "admin":
			relation = ofganames.AdministratorRelation
		case "add-model":
			relation = ofganames.CanAddModelRelation
		default:
			c.Fatalf("unknown cloud access level: %s", u.Access)
		}
		if client != nil {
			user := openfga.NewUser(&dbUser, client)
			err := user.SetCloudAccess(context.Background(), cl.dbo.ResourceTag(), relation)
			c.Assert(err, qt.IsNil)
		}
	}
}

// addModelRelations adds permissions the model should have and adds permissions for users to the model.
func (m Model) addModelRelations(c *qt.C, db db.Database, client *openfga.OFGAClient) {
	owner := openfga.NewUser(&m.dbo.Owner, client)
	err := owner.SetModelAccess(context.Background(), m.dbo.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	for _, u := range m.Users {
		dbUser := m.env.User(u.User).DBObject(c, db)
		var relation openfga.Relation
		switch u.Access {
		case "admin":
			relation = ofganames.AdministratorRelation
		case "write":
			relation = ofganames.WriterRelation
		case "read":
			relation = ofganames.ReaderRelation
		default:
			c.Fatalf("unknown model access: %s %s", dbUser.Name, u.Access)
		}
		user := openfga.NewUser(&dbUser, client)
		err := user.SetModelAccess(context.Background(), m.dbo.ResourceTag(), relation)
		c.Assert(err, qt.IsNil)
	}

	err = client.AddControllerModel(context.Background(), m.dbo.Controller.ResourceTag(), m.dbo.ResourceTag())
	c.Assert(err, qt.IsNil)
}

// addControllerRelations adds permissions the model should have and adds permissions for users to the controller.
func (ctl Controller) addControllerRelations(c *qt.C, client *openfga.OFGAClient) {
	if ctl.dbo.AdminIdentityName != "" {
		userIdentity, err := dbmodel.NewIdentity(ctl.dbo.AdminIdentityName)
		c.Assert(err, qt.IsNil)

		user := openfga.NewUser(
			userIdentity,
			client,
		)
		err = user.SetControllerAccess(context.Background(), ctl.dbo.ResourceTag(), ofganames.AdministratorRelation)
		c.Assert(err, qt.IsNil)
	}
	err := client.AddCloudController(context.Background(), names.NewCloudTag(ctl.Cloud), ctl.dbo.ResourceTag())
	c.Assert(err, qt.IsNil)
}

func (e *Environment) addJIMMRelations(c *qt.C, jimmTag names.ControllerTag, db db.Database, client *openfga.OFGAClient) {
	for _, user := range e.Users {
		user.addUserRelations(c, jimmTag, db, client)
	}
	for _, controller := range e.Controllers {
		err := client.AddController(context.Background(), jimmTag, controller.dbo.ResourceTag())
		c.Assert(err, qt.IsNil)
	}
	for _, cl := range e.Clouds {
		cl.addCloudRelations(c, db, client)
	}
	for _, m := range e.Models {
		m.addModelRelations(c, db, client)
	}
	for _, ctl := range e.Controllers {
		ctl.addControllerRelations(c, client)
	}
}

func (e *Environment) PopulateDBAndPermissions(c *qt.C, jimmTag names.ControllerTag, db db.Database, client *openfga.OFGAClient) {
	e.PopulateDB(c, db)
	c.Assert(client, qt.IsNotNil)
	e.addJIMMRelations(c, jimmTag, db, client)
}

func (e *Environment) PopulateDB(c Tester, db db.Database) {
	for i := range e.Users {
		e.Users[i].env = e
		e.Users[i].DBObject(c, db)
	}

	for i := range e.Clouds {
		e.Clouds[i].env = e
		e.Clouds[i].DBObject(c, db)
	}
	for i := range e.CloudCredentials {
		e.CloudCredentials[i].env = e
		e.CloudCredentials[i].DBObject(c, db)
	}
	for i := range e.CloudDefaults {
		e.CloudDefaults[i].env = e
		e.CloudDefaults[i].DBObject(c, db)
	}
	for i := range e.Controllers {
		e.Controllers[i].env = e
		e.Controllers[i].DBObject(c, db)
	}
	for i := range e.Models {
		e.Models[i].env = e
		e.Models[i].DBObject(c, db)
	}
	for i := range e.UserDefaults {
		e.UserDefaults[i].env = e
		e.UserDefaults[i].DBObject(c, db)
	}
}

// UserDefaults represents user's default configuration for a new model.
type UserDefaults struct {
	User     string                 `json:"user"`
	Defaults map[string]interface{} `json:"defaults"`

	env *Environment
	dbo dbmodel.IdentityModelDefaults
}

func (cd *UserDefaults) DBObject(c Tester, db db.Database) dbmodel.IdentityModelDefaults {
	if cd.dbo.ID != 0 {
		return cd.dbo
	}

	cd.dbo.Identity = cd.env.User(cd.User).DBObject(c, db)
	cd.dbo.Defaults = cd.Defaults

	err := db.SetIdentityModelDefaults(context.Background(), &cd.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return cd.dbo
}

// CloudDefaults represents default cloud/region configuration for a new model.
type CloudDefaults struct {
	User     string                 `json:"user"`
	Cloud    string                 `json:"cloud"`
	Region   string                 `json:"region"`
	Defaults map[string]interface{} `json:"defaults"`

	env *Environment
	dbo dbmodel.CloudDefaults
}

func (cd *CloudDefaults) DBObject(c Tester, db db.Database) dbmodel.CloudDefaults {
	if cd.dbo.ID != 0 {
		return cd.dbo
	}

	cd.dbo.Identity = cd.env.User(cd.User).DBObject(c, db)
	cd.dbo.Cloud = cd.env.Cloud(cd.Cloud).DBObject(c, db)
	cd.dbo.Region = cd.Region
	cd.dbo.Defaults = cd.Defaults

	err := db.SetCloudDefaults(context.Background(), &cd.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return cd.dbo
}

// A Cloud represents the definition of a cloud in a test environment.
type Cloud struct {
	Name            string        `json:"name"`
	Type            string        `json:"type"`
	HostCloudRegion string        `json:"host-cloud-region"`
	Regions         []CloudRegion `json:"regions"`
	Users           []UserAccess  `json:"users"`

	env *Environment
	dbo dbmodel.Cloud
}

// A CloudRegion represents the definition of a cloud region in a test
// environment.
type CloudRegion struct {
	Name string `json:"name"`
}

// DBObject returns a database object for the specified cloud, suitable
// for adding to the database.
func (cl *Cloud) DBObject(c Tester, db db.Database) dbmodel.Cloud {
	if cl.dbo.ID != 0 {
		return cl.dbo
	}

	cl.dbo.Name = cl.Name
	cl.dbo.Type = cl.Type
	cl.dbo.HostCloudRegion = cl.HostCloudRegion
	for _, r := range cl.Regions {
		cl.dbo.Regions = append(cl.dbo.Regions, dbmodel.CloudRegion{
			Name: r.Name,
		})
	}

	err := db.AddCloud(context.Background(), &cl.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return cl.dbo
}

// A CloudCredential represents the definition of a cloud credential in a
// test environment.
type CloudCredential struct {
	Owner      string            `json:"owner"`
	Cloud      string            `json:"cloud"`
	Name       string            `json:"name"`
	AuthType   string            `json:"auth-type"`
	Attributes map[string]string `json:"attributes"`

	env *Environment
	dbo dbmodel.CloudCredential
}

func (cc *CloudCredential) DBObject(c Tester, db db.Database) dbmodel.CloudCredential {
	if cc.dbo.ID != 0 {
		return cc.dbo
	}
	cc.dbo.Name = cc.Name
	cc.dbo.Cloud = cc.env.Cloud(cc.Cloud).DBObject(c, db)
	cc.dbo.CloudName = cc.dbo.Cloud.Name
	cc.dbo.Owner = cc.env.User(cc.Owner).DBObject(c, db)
	cc.dbo.OwnerIdentityName = cc.dbo.Owner.Name
	cc.dbo.AuthType = cc.AuthType
	cc.dbo.Attributes = cc.Attributes

	err := db.SetCloudCredential(context.Background(), &cc.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return cc.dbo
}

// A Controller represents the definition of a controller in a test
// environment.
type Controller struct {
	Name          string                          `json:"name"`
	UUID          string                          `json:"uuid"`
	Cloud         string                          `json:"cloud"`
	CloudRegion   string                          `json:"region"`
	CloudRegions  []CloudRegionControllerPriority `json:"cloud-regions"`
	AgentVersion  string                          `json:"agent-version"`
	AdminUser     string                          `json:"admin-user"`
	AdminPassword string                          `json:"admin-password"`

	env *Environment
	dbo dbmodel.Controller
}

func (ctl *Controller) DBObject(c Tester, db db.Database) dbmodel.Controller {
	if ctl.dbo.ID != 0 {
		return ctl.dbo
	}
	ctl.dbo.Name = ctl.Name
	ctl.dbo.UUID = ctl.UUID
	ctl.dbo.AgentVersion = ctl.AgentVersion
	ctl.dbo.AdminIdentityName = ctl.AdminUser
	ctl.dbo.AdminPassword = ctl.AdminPassword
	ctl.dbo.CloudName = ctl.Cloud
	ctl.dbo.CloudRegion = ctl.CloudRegion
	ctl.dbo.CloudRegions = make([]dbmodel.CloudRegionControllerPriority, len(ctl.CloudRegions))
	for i, cr := range ctl.CloudRegions {
		cl := ctl.env.Cloud(cr.Cloud).DBObject(c, db)
		ctl.dbo.CloudRegions[i] = dbmodel.CloudRegionControllerPriority{
			CloudRegion: cl.Region(cr.Region),
			Priority:    cr.Priority,
		}
	}

	err := db.AddController(context.Background(), &ctl.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return ctl.dbo
}

// CloudRegionControllerPriority represents the priority with which a
// a controller should be selected for a particular cloud region.
type CloudRegionControllerPriority struct {
	Cloud    string `json:"cloud"`
	Region   string `json:"region"`
	Priority uint   `json:"priority"`
}

// A Model represents the definition of a model in a test environment.
type Model struct {
	Name                string       `json:"name"`
	Owner               string       `json:"owner"`
	UUID                string       `json:"uuid"`
	Controller          string       `json:"controller"`
	MigrationController string       `json:"migration-controller"`
	Cloud               string       `json:"cloud"`
	CloudRegion         string       `json:"region"`
	CloudCredential     string       `json:"cloud-credential"`
	Users               []UserAccess `json:"users"`

	Type          string                   `json:"type"`
	DefaultSeries string                   `json:"default-series"`
	Life          string                   `json:"life"`
	Status        jujuparams.EntityStatus  `json:"status"`
	SLA           *jujuparams.ModelSLAInfo `json:"sla"`
	AgentVersion  string                   `json:"agent-version"`
	Cores         int64                    `json:"cores"`
	Machines      int64                    `json:"machines"`
	Units         int64                    `json:"units"`

	env *Environment
	dbo dbmodel.Model
}

func (m *Model) DBObject(c Tester, db db.Database) dbmodel.Model {
	if m.dbo.ID != 0 {
		return m.dbo
	}
	m.dbo.Name = m.Name
	m.dbo.Owner = m.env.User(m.Owner).DBObject(c, db)
	if m.UUID != "" {
		m.dbo.UUID.String = m.UUID
		m.dbo.UUID.Valid = true
	}
	m.dbo.Controller = m.env.Controller(m.Controller).DBObject(c, db)
	m.env.Controller(m.Controller)
	migrationControllerID := sql.NullInt32{}
	if m.MigrationController != "" {
		//nolint:gosec // Database IDs for tests will fit into int32.
		migrationControllerID.Int32 = int32(m.env.Controller(m.MigrationController).dbo.ID)
		migrationControllerID.Valid = true
	}
	m.dbo.MigrationControllerID = migrationControllerID
	m.dbo.CloudRegion = m.env.Cloud(m.Cloud).DBObject(c, db).Region(m.CloudRegion)
	m.dbo.CloudCredential = m.env.CloudCredential(m.Owner, m.Cloud, m.CloudCredential).DBObject(c, db)

	m.dbo.Type = m.Type
	m.dbo.DefaultSeries = m.DefaultSeries
	m.dbo.Life = m.Life
	m.dbo.Status.FromJujuEntityStatus(m.Status)
	m.dbo.Status.Version = m.AgentVersion
	if m.SLA != nil {
		m.dbo.SLA.FromJujuModelSLAInfo(*m.SLA)
	}
	m.dbo.Cores = m.Cores
	m.dbo.Machines = m.Machines
	m.dbo.Units = m.Units

	err := db.AddModel(context.Background(), &m.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return m.dbo
}

type User struct {
	Username         string `json:"username"`
	DisplayName      string `json:"display-name"`
	ControllerAccess string `json:"controller-access"`

	env *Environment
	dbo dbmodel.Identity
}

func (u *User) DBObject(c Tester, db db.Database) dbmodel.Identity {
	if u.dbo.ID != 0 {
		return u.dbo
	}
	u.dbo.Name = u.Username
	u.dbo.DisplayName = u.DisplayName

	err := db.GetIdentity(context.Background(), &u.dbo)
	if err != nil {
		c.Fatalf("err is not nil: %s", err)
	}

	return u.dbo
}

// UserAccess represents user access to am object in a test environment.
type UserAccess struct {
	User           string `json:"user"`
	Access         string `json:"access"`
	LastConnection string `json:"last-connection"`
}

// ParseMachineHardware parses a string representation of a machine's
// hardware profile. The string should consist of a series of whitespace
// separated <key>=<value> pairs. Unrecognised keys are silently ignored.
// ParseMachineHardware will panic if the value cannot be parsed to the
// correct type for the key. If the given string is empty then a nil value
// is retuned.
func ParseMachineHardware(s string) *jujuparams.MachineHardware {
	if s == "" {
		return nil
	}
	var hw jujuparams.MachineHardware
	for _, f := range strings.Fields(s) {
		var err error

		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "arch":
			hw.Arch = &parts[1]
		case "mem":
			hw.Mem = new(uint64)
			*hw.Mem, err = strconv.ParseUint(parts[1], 0, 64)
			if err != nil {
				panic(err)
			}
		case "root-disk":
			hw.RootDisk = new(uint64)
			*hw.RootDisk, err = strconv.ParseUint(parts[1], 0, 64)
			if err != nil {
				panic(err)
			}
		case "cores":
			hw.Cores = new(uint64)
			*hw.Cores, err = strconv.ParseUint(parts[1], 0, 64)
			if err != nil {
				panic(err)
			}
		case "cpu-power":
			hw.CpuPower = new(uint64)
			*hw.CpuPower, err = strconv.ParseUint(parts[1], 0, 64)
			if err != nil {
				panic(err)
			}
		case "tags":
			tags := strings.Split(parts[1], ",")
			hw.Tags = &tags
		case "availability-zone":
			hw.AvailabilityZone = &parts[1]
		}
	}
	return &hw
}
