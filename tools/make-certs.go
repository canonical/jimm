// Copyright 2018 Canonical Ltd.

// make-certs is a tool for making certificates for use with JIMM server
// running on localhost for QA purposes. A root certificate is created
// with a new key that is discarded at the end of the process. This
// certificate is used to sign second certificate for the following
// addresses:
//	- ip6-localhost
//	- juju-apiserver
//	- localhost
// 	- localhost.localdomain
//	- 127.0.0.1
//	- ::1

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

func main() {
	if err := createCerts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}

func createCerts() error {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	rootTemplate := x509.Certificate{
		SerialNumber: big.NewInt(0),
		Subject: pkix.Name{
			CommonName: "JIMM QA Root",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(20 * 365 * 24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	rootCertBytes, err := x509.CreateCertificate(rand.Reader, &rootTemplate, &rootTemplate, rootKey.Public(), rootKey)
	if err != nil {
		return err
	}
	rootCert, err := x509.ParseCertificate(rootCertBytes)
	if err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             rootTemplate.NotBefore,
		NotAfter:              rootTemplate.NotAfter,
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames: []string{
			"ip6-localhost",
			"juju-apiserver",
			"localhost",
			"localhost.localdomain",
		},
		IPAddresses: []net.IP{
			net.ParseIP("127.0.0.1"),
			net.ParseIP("::1"),
		},
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, rootCert, key.Public(), rootKey)
	if err != nil {
		return err
	}
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	err = pem.Encode(os.Stdout, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		return err
	}
	err = pem.Encode(os.Stdout, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rootCertBytes,
	})
	if err != nil {
		return err
	}
	err = pem.Encode(os.Stdout, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})
	if err != nil {
		return err
	}
	return nil
}
