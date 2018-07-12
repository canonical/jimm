// Copyright 2017 Canonical Ltd.

// register-existing-models obtains authorization credentials to collect usage information
// for all models that don't have them already.
// It is designed to be called once only, but will do nothing if called a second time.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	plansapi "github.com/CanonicalLtd/plans-client/api"
	"github.com/juju/gnuflag"
	"go.uber.org/zap/zapcore"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery/agent"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
)

var (
	NewUsageSenderAuthorizationClient = func(url string, client *httpbakery.Client) (UsageSenderAuthorizationClient, error) {
		return plansapi.NewPlanClient(url, plansapi.HTTPClient(client))
	}
)

// UsageSenderAuthorizationClient is used to obtain authorization to
// collect and report usage metrics.
type UsageSenderAuthorizationClient interface {
	AuthorizeReseller(plan, charm, application, applicationOwner, applicationUser string) (*macaroon.Macaroon, error)
}

type Config struct {
	MongoAddr         string            `yaml:"mongo-addr"`
	DBName            string            `yaml:"dbname"`
	IdentityPublicKey *bakery.PublicKey `yaml:"identity-public-key"`
	IdentityLocation  string            `yaml:"identity-location"`
	AgentUsername     string            `yaml:"agent-username"`
	AgentKey          *bakery.KeyPair   `yaml:"agent-key"`
	LoggingLevel      zapcore.Level     `yaml:"logging-level"`
	UsageSenderURL    string            `yaml:"usage-sender-url,omitempty"`
	JIMMPlan          string            `yaml:"jimm-plan"`
	JIMMCharm         string            `yaml:"jimm-charm"`
	JIMMOwner         string            `yaml:"jimm-owner"`
	JIMMName          string            `yaml:"jimm-name"`
}

func (c *Config) validate() error {
	var missing []string
	if c.MongoAddr == "" {
		missing = append(missing, "mongo-addr")
	}
	if c.IdentityLocation == "" {
		missing = append(missing, "identity-location")
	}
	if c.UsageSenderURL == "" {
		missing = append(missing, "usage-sender-url")
	}
	if c.JIMMPlan == "" {
		missing = append(missing, "jimm-plan")
	}
	if c.JIMMCharm == "" {
		missing = append(missing, "jimm-charm")
	}
	if c.JIMMOwner == "" {
		missing = append(missing, "jimm-owner")
	}
	if c.JIMMName == "" {
		missing = append(missing, "jimm-name")
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing fields %s in config file", strings.Join(missing, ", "))
	}

	return nil
}

func readConfig(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read %q", path)
	}
	var conf Config
	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse %q", path)
	}
	if err := conf.validate(); err != nil {
		return nil, errgo.Mask(err)
	}
	return &conf, nil
}

func main() {
	gnuflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s <config path>\n", filepath.Base(os.Args[0]))
		gnuflag.PrintDefaults()
		os.Exit(2)
	}
	gnuflag.Parse(false)
	args := gnuflag.Args()
	if len(args) == 0 {
		gnuflag.Usage()
	}
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", args[1:])
		os.Exit(2)
	}

	conf, err := readConfig(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read the config file: %v\n", err)
		os.Exit(2)
	}
	if conf.DBName == "" {
		conf.DBName = "jem"
	}

	ctx := context.Background()
	zapctx.LogLevel.SetLevel(conf.LoggingLevel)
	zaputil.InitLoggo(zapctx.Default, conf.LoggingLevel)

	zapctx.Debug(ctx, "connecting to mongo")
	session, err := mgo.Dial(conf.MongoAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to mongo: %v\n", err)
		os.Exit(2)
	}
	defer session.Close()

	err = updateModels(ctx, session.DB(conf.DBName).C("models"), conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func updateModels(ctx context.Context, collection *mgo.Collection, conf *Config) error {
	var models []mongodoc.Model
	err := collection.Find(nil).All(&models)
	if err != nil {
		return errgo.Notef(err, "model query failed")
	}
	var client UsageSenderAuthorizationClient
	for _, model := range models {
		if len(model.UsageSenderCredentials) != 0 {
			continue
		}
		if client == nil {
			bclient := httpbakery.NewClient()
			if conf.AgentKey == nil {
				return errgo.New("agent key not set")
			}
			bclient.Key = conf.AgentKey
			idmURL, err := url.Parse(conf.IdentityLocation)
			if err != nil {
				return errgo.Notef(err, "cannot parse identity location URL %q", conf.IdentityLocation)
			}
			agent.SetUpAuth(bclient, idmURL, conf.AgentUsername)
			client, err = NewUsageSenderAuthorizationClient(conf.UsageSenderURL, bclient)
			if err != nil {
				return errgo.Notef(err, "cannot make omnibus authorization client")
			}
		}
		credentials, err := client.AuthorizeReseller(
			conf.JIMMPlan,
			conf.JIMMCharm,
			conf.JIMMName,
			conf.JIMMOwner,
			string(model.Path.User),
		)
		if err != nil {
			return errgo.Notef(err, "cannot obtain authorization to collect usage metrics")
		}
		data, err := json.Marshal(credentials)
		if err != nil {
			return errgo.Notef(err, "cannot marshal metrics authorization credentials")
		}
		err = collection.UpdateId(model.Id, bson.M{"$set": bson.M{"usagesendercredentials": data}})
		if err != nil {
			return errgo.Notef(err, "failed to update model")
		}
	}
	return nil
}
