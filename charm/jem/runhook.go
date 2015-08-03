// The JEM charm implements a gocharm-based Juju charm
// that requires a mongodb relation and provides an HTTP
// relation.
package jem

import (
	"fmt"
	"strings"

	"github.com/juju/gocharm/charmbits/httpservice"
	_ "github.com/juju/gocharm/charmbits/mongodbrelation"
	"github.com/juju/gocharm/hook"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"

	// We need to import any providers (for the time being,
	// until the juju API exposes environment schemas)
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/local"

	"github.com/CanonicalLtd/jem"
)

func RegisterHooks(r *hook.Registry) {
	var j jemCharm
	r.RegisterContext(j.setContext, &j.state)
	j.svc.Register(r.Clone("svc"), "jem", "apiserver", newHandler)
	r.RegisterConfig("identity-location", charm.Option{
		Type:        "string",
		Description: "location of the identity manager",
		Default:     "https://api.jujucharms.com/identity",
	})
	r.RegisterConfig("identity-public-key", charm.Option{
		Type:        "string",
		Description: "public key of the identity manager",
		Default:     "o/yOqSNWncMo1GURWuez/dGR30TscmmuIxgjztpoHEY=",
	})
	r.RegisterConfig("agent-name", charm.Option{
		Type:        "string",
		Description: "agent name",
	})
	r.RegisterConfig("agent-public-key", charm.Option{
		Type:        "string",
		Description: "public key for agent authentication",
	})
	r.RegisterConfig("agent-private-key", charm.Option{
		Type:        "string",
		Description: "private key for agent authentication",
	})
	// Register the config-changed hook to ensure that
	// the changed hook will be called with the configuration
	// changes.
	r.RegisterHook("start", j.start)
	r.RegisterHook("config-changed", nopHook)
	r.RegisterHook("stop", j.stop)
	r.RegisterHook("*", j.changed)
}

// jemCharm represents the hook side JEM charm.
type jemCharm struct {
	state localState
	ctxt  *hook.Context
	svc   httpservice.Service
}

type localState struct {
	Started bool
}

func (j *jemCharm) start() error {
	j.state.Started = true
	return nil
}

func (j *jemCharm) stop() error {
	j.state.Started = false
	return nil
}

// setContext sets the hook context.
func (j *jemCharm) setContext(ctxt *hook.Context) error {
	j.ctxt = ctxt
	return nil
}

// setStatusf sets the status of the charm's service.
func (j *jemCharm) setStatusf(status hook.Status, f string, a ...interface{}) error {
	msg := fmt.Sprintf(f, a...)
	j.ctxt.Logf("status %s; %s", status, msg)
	return j.ctxt.SetStatus(status, msg)
}

// changed is called after any registered hook functions have executed.
// It determines all the desired parameters and starts or stops
// the server as appropriate.
func (j *jemCharm) changed() (err error) {
	if !j.state.Started {
		j.ctxt.Logf("stopping service")
		return j.svc.Stop()
	}
	p, err := j.getParams()
	if err != nil {
		j.setStatusf(hook.StatusBlocked, "bad config: %v", err)
		return j.svc.Stop()
	}
	j.setStatusf(hook.StatusActive, "")
	return j.svc.Start(p)
}

// getParams gets all required configuration parameters.
func (j *jemCharm) getParams() (params, error) {
	var p params
	if err := j.ctxt.GetAllConfig(&p); err != nil {
		return params{}, errgo.Notef(err, "invalid config")
	}
	if p.AgentName != "" && (p.AgentPublicKey == nil || p.AgentPrivateKey == nil) {
		return params{}, errgo.Newf("agent-name given but agent-private-key or agent-public-key missing")
	}
	if p.IdentityLocation == "" || p.IdentityPublicKey == nil {
		return params{}, errgo.Newf("identity-location or identity-public-key missing")
	}
	return p, nil
}

// params represents all configuration parameters that are passed
// to the server.
type params struct {
	IdentityLocation  string            `json:"identity-location"`
	IdentityPublicKey *bakery.PublicKey `json:"identity-public-key"`

	AgentName       string             `json:"agent-name"`
	AgentPublicKey  *bakery.PublicKey  `json:"agent-public-key"`
	AgentPrivateKey *bakery.PrivateKey `json:"agent-private-key"`
}

// relations holds the relations are required by the service.
type relations struct {
	Session *mgo.Session
}

// newHandler creates the JEM handler using the given parameters
// and relations.
func newHandler(p params, rel *relations) (httpservice.Handler, error) {
	keyring := bakery.NewPublicKeyRing()
	idloc := strings.TrimSuffix(p.IdentityLocation, "/") + "/"
	keyring.AddPublicKeyForLocation(idloc, true, p.IdentityPublicKey)
	config := jem.ServerParams{
		DB:               rel.Session.DB("jem"),
		StateServerAdmin: "noone", // required but ignored.
		IdentityLocation: p.IdentityLocation,
		PublicKeyLocator: keyring,
		AgentUsername:    p.AgentName,
		AgentKey: &bakery.KeyPair{
			Public:  *p.AgentPublicKey,
			Private: *p.AgentPrivateKey,
		},
	}
	return jem.NewServer(config)
}

func nopHook() error {
	return nil
}
