// Copyright 2017 Canonical Ltd.

// register-existing-models obtains authorization credentials to collect usage information
// for all models that don't have them already.
// It is designed to be called once only, but will do nothing if called a second time.
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	omnibusapi "github.com/CanonicalLtd/omnibus/plans/api"
	"github.com/juju/gnuflag"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v1"

	"github.com/juju/loggo"
	"github.com/uber-go/zap"

	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
)

var (
	NewUsageSenderAuthorizationClient = func(url string, client *httpbakery.Client) (UsageSenderAuthorizationClient, error) {
		return omnibusapi.NewAuthorizationClient(url, omnibusapi.HTTPClient(client))
	}
)

// UsageSenderAuthorizationClient is used to obtain authorization to
// collect and report usage metrics.
type UsageSenderAuthorizationClient interface {
	AuthorizeReseller(plan, charm, application, applicationOwner, applicationUser string) (*macaroon.Macaroon, error)
}

type Config struct {
	MongoAddr         string            `yaml:"mongo-addr"`
	IdentityPublicKey *bakery.PublicKey `yaml:"identity-public-key"`
	IdentityLocation  string            `yaml:"identity-location"`
	AgentUsername     string            `yaml:"agent-username"`
	AgentKey          *bakery.KeyPair   `yaml:"agent-key"`
	LoggingLevel      zap.Level         `yaml:"logging-level"`
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
	if len(missing) != 0 {
		return fmt.Errorf("missing fields %s in config file", strings.Join(missing, ", "))
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

	ctx := setUpLogging(context.Background(), conf.LoggingLevel)
	zapctx.Debug(ctx, "connecting to mongo")
	session, err := mgo.Dial(conf.MongoAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to mongo: %v\n", err)
		os.Exit(2)
	}
	defer session.Close()

	err = updateModels(ctx, session.DB("jem").C("models"), conf)
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
			bclient.Key = conf.AgentKey
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

var zapToLoggo = map[zap.Level]loggo.Level{
	zap.DebugLevel: loggo.TRACE, // Include trace and debug level messages.
	zap.InfoLevel:  loggo.INFO,
	zap.WarnLevel:  loggo.WARNING,
	zap.ErrorLevel: loggo.ERROR, // Include error and critical level messages.
}

func setUpLogging(ctx context.Context, level zap.Level) context.Context {
	// Set up the root zap logger.
	// TODO use zap.AddCaller when it works OK with log wrappers.
	logger := zap.New(zap.NewJSONEncoder(timestampFormatter()), zap.Output(os.Stderr), level)
	zapctx.Default = logger

	// Set up loggo so that it will write to the root zap logger.
	loggo.ReplaceDefaultWriter(zaputil.NewLoggoWriter(logger))

	// Configure loggo so that it will log at the right level.
	loggo.DefaultContext().ApplyConfig(map[string]loggo.Level{
		"<root>": zapToLoggo[level],
	})
	return zapctx.WithLogger(ctx, logger)
}

const rfc3339Milli = "2006-01-02T15:04:05.000Z"

// timestampFormatter formats logging timestamps
// in RFC3339 format with millisecond precision.
func timestampFormatter() zap.TimeFormatter {
	return zap.TimeFormatter(func(t time.Time) zap.Field {
		return zap.String("ts", t.UTC().Format(rfc3339Milli))
	})
}
