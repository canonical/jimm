// Copyright 2015 Canonical Ltd.

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	vault "github.com/hashicorp/vault/api"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	jem "github.com/CanonicalLtd/jimm"
	"github.com/CanonicalLtd/jimm/config"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
)

// websocketRequestTimeout is the amount of time a websocket connection
// will wait for a request before failing the connections. It is
// hardcoded in juju so I see no reason why it can't be here also.
const websocketRequestTimeout = 5 * time.Minute

var (
	// The logging-config flag is present for backward compatibility
	// only and will probably be removed in the future.
	loggingConfig = flag.String("logging-config", "", "specify log levels for modules e.g. <root>=TRACE")

	errSignaled = errgo.New("signaled")
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
	if *loggingConfig != "" && strings.ToUpper(*loggingConfig) != "INFO" {
		fmt.Fprintln(os.Stderr, "warning: ignoring --logging-config flag; use logging-level in configuration file instead")
	}
	conf, err := readConfig(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "STOP %s\n", err)
		os.Exit(2)
	}

	zapctx.LogLevel.SetLevel(conf.LoggingLevel)
	zaputil.InitLoggo(zapctx.Default, conf.LoggingLevel)

	fmt.Fprintln(os.Stderr, "START")
	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error { return interrupt(ctx) })
	eg.Go(func() error { return startServer(ctx, eg, conf) })
	err = eg.Wait()
	fmt.Fprintf(os.Stderr, "STOP %s\n", err)

	if errgo.Cause(err) != errSignaled {
		os.Exit(1)
	}
}

func readConfig(path string) (*config.Config, error) {
	conf, err := config.Read(path)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	conf.IdentityLocation = strings.TrimSuffix(conf.IdentityLocation, "/")
	if strings.HasSuffix(conf.IdentityLocation, "v1/discharger") {
		// It's probably some old code that uses the old IdentityLocation
		// meaning.
		return nil, errgo.Newf("identity location must not contain discharge path")
	}
	return conf, nil
}

func startServer(ctx context.Context, eg *errgroup.Group, conf *config.Config) error {
	zapctx.Debug(ctx, "connecting to mongo")
	session, err := mgo.Dial(conf.MongoAddr)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", conf.MongoAddr)
	}
	defer session.Close()
	if conf.DBName == "" {
		conf.DBName = "jem"
	}
	db := session.DB(conf.DBName)

	zapctx.Debug(ctx, "loading TLS configuration")
	tlsConfig, err := conf.TLSConfig()
	if err != nil {
		return errgo.Mask(err)
	}

	var vaultClient *vault.Client
	if conf.Vault.Address != "" {
		zapctx.Debug(ctx, "connecting to vault server")
		var err error
		vaultClient, err = startVaultClient(ctx, eg, conf.Vault)
		if err != nil {
			return errgo.Mask(err)
		}
	}

	zapctx.Debug(ctx, "setting up the API server")
	locator := httpbakery.NewThirdPartyLocator(nil, nil)
	if conf.MaxMgoSessions == 0 {
		conf.MaxMgoSessions = 100
	}

	cfg := jem.ServerParams{
		DB:                      db,
		MaxMgoSessions:          conf.MaxMgoSessions,
		ControllerAdmin:         conf.ControllerAdmin,
		IdentityLocation:        conf.IdentityLocation,
		CharmstoreLocation:      conf.CharmstoreLocation,
		MeteringLocation:        conf.MeteringLocation,
		ThirdPartyLocator:       locator,
		AgentUsername:           conf.AgentUsername,
		AgentKey:                conf.AgentKey,
		RunMonitor:              true,
		ControllerUUID:          conf.ControllerUUID,
		WebsocketRequestTimeout: websocketRequestTimeout,
		GUILocation:             conf.GUILocation,
		UsageSenderURL:          conf.UsageSenderURL,
		UsageSenderSpoolPath:    conf.UsageSenderSpoolDir,
		Domain:                  conf.Domain,
		PublicCloudMetadata:     conf.PublicCloudMetadata,
		Pubsub: &pubsub.Hub{
			MaxConcurrency: conf.MaxPubsubConcurrency,
		},
		JujuDashboardLocation: conf.JujuDashboardLocation,
		VaultClient:           vaultClient,
		VaultPath:             conf.Vault.KVPath,
	}

	server, err := jem.NewServer(ctx, cfg)
	if err != nil {
		return errgo.Notef(err, "cannot create new server at %q", conf.APIAddr)
	}
	handler := server.(http.Handler)
	if conf.AccessLog != "" {
		accesslog := &lumberjack.Logger{
			Filename:   conf.AccessLog,
			MaxSize:    500, // megabytes
			MaxBackups: 3,
			MaxAge:     28, //days
		}
		handler = handlers.CombinedLoggingHandler(accesslog, handler)
	}

	zapctx.Info(ctx, "starting the API server")
	httpServer := &http.Server{
		Addr:      conf.APIAddr,
		Handler:   handler,
		TLSConfig: tlsConfig,
	}
	eg.Go(func() error {
		<-ctx.Done()
		ctx1, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx1); err != nil {
			zapctx.Error(ctx, "HTTP server shutdown failed", zap.Error(err))
		}
		return nil
	})
	if tlsConfig == nil {
		eg.Go(httpServer.ListenAndServe)
	} else {
		eg.Go(func() error {
			return httpServer.ListenAndServeTLS("", "")
		})
	}

	return nil
}

func interrupt(ctx context.Context) error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-c:
		if sig == syscall.SIGTERM {
			return errgo.WithCausef(nil, errSignaled, "terminating")
		}
		return errgo.WithCausef(nil, errSignaled, "interrupted")
	case <-ctx.Done():
		return ctx.Err()
	}
}
