package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/mgo.v2"
	"launchpad.net/loggo"

	"github.com/CanonicalLtd/jem/cmd/jem-proxy/internal/proxy"
)

var (
	logger        = loggo.GetLogger("jem-proxy")
	loggingConfig = flag.String("logging-config", "", "specify log levels for modules e.g. <root>=TRACE")
	addr          = flag.String("listen", ":17070", "specify the address to listen on")
	mongo         = flag.String("mongo", "", "address of the mongodb database")
	cert          = flag.String("cert", "", "file containing TLS certificate")
	key           = flag.String("key", "", "file containing TLS key")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if *loggingConfig != "" {
		if err := loggo.ConfigureLoggers(*loggingConfig); err != nil {
			fmt.Fprintf(os.Stderr, "cannot configure loggers: %v\n", err)
			os.Exit(1)
		}
	}
	s, err := mgo.Dial(*mongo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to mongo: %v\n", err)
		os.Exit(1)
	}
	proxy, err := proxy.NewProxy(s.DB("jem"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create proxy: %v\n", err)
		os.Exit(1)
	}
	if err := http.ListenAndServeTLS(*addr, *cert, *key, proxy); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
