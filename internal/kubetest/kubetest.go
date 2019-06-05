// Copyright 2018 Canonical Ltd.

package kubetest

import (
	"io/ioutil"
	"os"

	gc "gopkg.in/check.v1"
	errgo "gopkg.in/errgo.v1"
	apicorev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
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

// StartMonitor starts a monitor that listens for new namespaces created
// whilst a test is running, removing them when the test is done.
func StartMonitor(c *gc.C, config api.Config) *Monitor {
	clientconfig, err := clientcmd.NewDefaultClientConfig(config, &clientcmd.ConfigOverrides{}).ClientConfig()
	c.Assert(err, gc.Equals, nil)
	client, err := kubernetes.NewForConfig(clientconfig)
	c.Assert(err, gc.Equals, nil)
	nsclient := client.CoreV1().Namespaces()
	nss, err := nsclient.List(metav1.ListOptions{})
	m := &Monitor{
		done: make(chan struct{}),
		wait: make(chan struct{}),
	}

	go m.watch(c, nsclient, nss.ResourceVersion)
	return m
}

// A Monitor listens for new namespaces being created during a test and
// removes them when Done is called.
type Monitor struct {
	done, wait chan struct{}
}

// Done closes the monitor removing and new namespaces that were created
// during the test.
func (m *Monitor) Done() {
	close(m.done)
	<-m.wait
}

func (m *Monitor) watch(c *gc.C, client corev1.NamespaceInterface, rv string) {
	w, err := client.Watch(metav1.ListOptions{
		Watch:           true,
		ResourceVersion: rv,
	})
	c.Check(err, gc.Equals, nil)
	defer w.Stop()

	var wch <-chan watch.Event
	if w != nil {
		wch = w.ResultChan()
	}
	var done bool
	nss := make(map[string]struct{})
	for {
		select {
		case ev := <-wch:
			switch ev.Type {
			case watch.Added:
				ns := ev.Object.(*apicorev1.Namespace)
				c.Logf("namespace %q created", ns.Name)
				nss[ns.Name] = struct{}{}
			case watch.Deleted:
				ns := ev.Object.(*apicorev1.Namespace)
				c.Logf("namespace %q deleted", ns.Name)
				delete(nss, ns.Name)
			}
		case <-m.done:
			m.done = nil
			done = true
			for k := range nss {
				err := client.Delete(k, nil)
				c.Check(err, gc.Equals, nil)
			}
		}
		if done && len(nss) == 0 {
			close(m.wait)
			return
		}
	}
}
