// Copyright 2018 Canonical Ltd.

package kubetest

import (
	"io/ioutil"
	"os"

	errgo "gopkg.in/errgo.v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var ErrDisabled = errgo.New("kubernetes testing disabled")

// LoadConfig loads the kubernetes client config in the same way as
// kubectl will by default (controlled by the KUBECONFIG environment
// variable). If the environment variable KUBETESTDISABLE is non-empty
// then an error with the cause ErrDisabled will be returned. Otherwise
// any error will be from loading the configuration. If the environment
// variable KUBETESTCONTEXT is non-empty then the CurrentContext value of
// the returned configuration will be set to this value.
func LoadConfig() (*api.Config, error) {
	if os.Getenv("KUBETESTDISABLE") != "" {
		return nil, ErrDisabled
	}
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := loader.Load()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if context := os.Getenv("KUBETESTCONTEXT"); context != "" {
		config.CurrentContext = context
	}
	return config, nil
}

// ServerURL determines the current server URL specified in the given
// config.
func ServerURL(config *api.Config) string {
	return config.Clusters[config.Contexts[config.CurrentContext].Cluster].Server
}

// CACertificate determines the current CA Certificate value in the given
// config. If the config specifies the certificate as a file and there is
// an error loading the file then CACertificate will panic.
func CACertificate(config *api.Config) string {
	certdata := config.Clusters[config.Contexts[config.CurrentContext].Cluster].CertificateAuthorityData
	if len(certdata) > 0 {
		return string(certdata)
	}
	certfile := config.Clusters[config.Contexts[config.CurrentContext].Cluster].CertificateAuthority
	if certfile == "" {
		return ""
	}
	f, err := os.Open(certfile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

// Username determines the current username specified in the given
// config.
func Username(config *api.Config) string {
	return config.AuthInfos[config.Contexts[config.CurrentContext].AuthInfo].Username
}

// Password determines the current password specified in the given
// config.
func Password(config *api.Config) string {
	return config.AuthInfos[config.Contexts[config.CurrentContext].AuthInfo].Password
}
