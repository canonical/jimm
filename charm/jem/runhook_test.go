package jem_test

import (
	"fmt"

	"github.com/juju/gocharm/charmbits/service"
	"github.com/juju/gocharm/hook"
	"github.com/juju/gocharm/hook/hooktest"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/charm/jem"
	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/jemclient"
	"github.com/CanonicalLtd/jem/params"
)

type suite struct {
	jujutesting.MgoSuite

	idmSrv *idmtest.Server
}

var _ = gc.Suite(&suite{})

func (s *suite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
}

func (s *suite) TearDownTest(c *gc.C) {
	s.idmSrv.Close()
	s.MgoSuite.TearDownTest(c)
}

func (s *suite) TestRegister(c *gc.C) {
	r := hook.NewRegistry()
	jem.RegisterHooks(r)
	c.Assert(r.RegisteredRelations(), jc.DeepEquals, map[string]charm.Relation{
		"mongodb": {
			Name:      "mongodb",
			Role:      charm.RoleRequirer,
			Interface: "mongodb",
			Limit:     1,
			Scope:     charm.ScopeGlobal,
		},
		"apiserver": {
			Name:      "apiserver",
			Role:      charm.RoleProvider,
			Interface: "http",
			Scope:     charm.ScopeGlobal,
		},
	})
}

func (s *suite) TestServer(c *gc.C) {
	runner := &hooktest.Runner{
		HookStateDir:  c.MkDir(),
		RegisterHooks: jem.RegisterHooks,
		Logger:        c,
	}

	service.NewService = hooktest.NewServiceFunc(runner, nil)

	runHook := func(hookName string, relId hook.RelationId, relUnit hook.UnitId, expectRecord [][]string) {
		runner.Record = nil
		err := runner.RunHook(hookName, relId, relUnit)
		c.Assert(err, gc.IsNil)
		c.Assert(runner.Record, jc.DeepEquals, expectRecord)
	}

	runHook("install", "", "", nil)

	runHook("start", "", "", [][]string{
		{"status-set", "blocked", "bad config: identity-location or identity-public-key missing"},
	})

	runner.Config = map[string]interface{}{
		"agent-name": "name",
	}
	runHook("config-changed", "", "", [][]string{
		{"status-set", "blocked", "bad config: agent-name given but agent-private-key or agent-public-key missing"},
	})

	// Provide all the necessary config. The server still won't
	// be running though, because it requires the mongodb
	// relation to be joined.
	httpPort := jujutesting.FindTCPPort()
	key, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)

	runner.Config = map[string]interface{}{
		"http-port":           httpPort,
		"agent-name":          "name",
		"agent-public-key":    key.Public.String(),
		"agent-private-key":   key.Private.String(),
		"identity-location":   s.idmSrv.URL.String(),
		"identity-public-key": s.idmSrv.PublicKey.String(),
	}
	runHook("config-changed", "", "", [][]string{
		{"open-port", fmt.Sprintf("%d/tcp", httpPort)},
		{"status-set", "active", ""},
	})

	// Join the mongodb relation, but don't provide any relation
	// attributes yet, because that's usual in the live Juju.
	runner.Relations = map[hook.RelationId]map[hook.UnitId]map[string]string{
		"rel0": {
			"mongoservice/0": {},
		},
	}
	runner.RelationIds = map[string][]hook.RelationId{
		"mongodb": {"rel0"},
	}
	runHook("mongodb-relation-joined", "rel0", "mongoservice/0", [][]string{
		{"status-set", "active", ""},
	})

	// relation-changed is always run after relation-joined.
	runHook("mongodb-relation-changed", "rel0", "mongoservice/0", [][]string{
		{"status-set", "active", ""},
	})

	// Add the mongodb relation unit settings.
	runner.Relations = map[hook.RelationId]map[hook.UnitId]map[string]string{
		"rel0": {
			"mongoservice/0": {
				"hostname": "localhost",
				"port":     fmt.Sprint(jujutesting.MgoServer.Port()),
			},
		},
	}
	runHook("mongodb-relation-changed", "rel0", "mongoservice/0", [][]string{
		{"status-set", "active", ""},
	})

	// Check that the server really is running now.
	client := jemclient.New(jemclient.NewParams{
		BaseURL: fmt.Sprintf("http://localhost:%d/", httpPort),
		Client:  s.idmSrv.Client("alice"),
	})
	resp, err := client.ListEnvironments(&params.ListEnvironments{})
	c.Assert(err, gc.IsNil)
	c.Assert(resp, jc.DeepEquals, &params.ListEnvironmentsResponse{})

	runHook("stop", "", "", nil)
}
