// Copyright 2015 Canonical Ltd.

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	// Include any providers known to support JEM.
	// Avoid including provider/all to reduce build time.
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/local"
	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem"
	"github.com/CanonicalLtd/jem/config"
)

var (
	logger        = loggo.GetLogger("jemd")
	loggingConfig = flag.String("logging-config", "", "specify log levels for modules e.g. <root>=TRACE")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] <config path>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	if *loggingConfig != "" {
		if err := loggo.ConfigureLoggers(*loggingConfig); err != nil {
			fmt.Fprintf(os.Stderr, "cannot configure loggers: %v", err)
			os.Exit(1)
		}
	}
	if err := serve(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func serve(confPath string) error {
	logger.Debugf("reading configuration")
	conf, err := config.Read(confPath)
	if err != nil {
		return errgo.Notef(err, "cannot read config file %q", confPath)
	}
	if strings.Contains(conf.IdentityLocation, "v1/discharger") {
		// It's probably some old code that uses the old IdentityLocation
		// meaning.
		return errgo.Notef(err, "identity location must not contain discharge path")
	}

	logger.Debugf("connecting to mongo")
	session, err := mgo.Dial(conf.MongoAddr)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", conf.MongoAddr)
	}
	defer session.Close()
	db := session.DB("jem")

	ring := bakery.NewPublicKeyRing()
	ring.AddPublicKeyForLocation(conf.IdentityLocation, false, conf.IdentityPublicKey)

	logger.Debugf("setting up the API server")
	cfg := jem.ServerParams{
		DB:               db,
		StateServerAdmin: conf.StateServerAdmin,
		IdentityLocation: conf.IdentityLocation,
		PublicKeyLocator: ring,
		AgentUsername:    conf.AgentUsername,
		AgentKey:         conf.AgentKey,
	}
	server, err := jem.NewServer(cfg)
	if err != nil {
		return errgo.Notef(err, "cannot create new server at %q", conf.APIAddr)
	}

	logger.Infof("starting the API server")
	return http.ListenAndServe(conf.APIAddr, server)
}
