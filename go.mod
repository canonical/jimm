module github.com/CanonicalLtd/jimm

go 1.16

require (
	github.com/Azure/go-autorest/autorest v0.11.24 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/canonical/candid v1.12.2
	github.com/canonical/go-dqlite v1.11.1
	github.com/canonical/go-service v1.0.0
	github.com/frankban/quicktest v1.14.3
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.1
	github.com/gobwas/glob v0.2.4-0.20181002190808-e7a84e9525fe // indirect
	github.com/google/go-cmp v0.5.9
	github.com/google/uuid v1.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/vault/api v1.8.2
	github.com/jackc/pgconn v1.7.0
	github.com/jackc/pgx/v4 v4.9.0
	github.com/juju/charm/v8 v8.0.6
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9 // indirect
	github.com/juju/cmd/v3 v3.0.0
	github.com/juju/errors v1.0.0
	github.com/juju/gnuflag v1.0.0
	github.com/juju/juju v0.0.0-20230228224222-7b871e782195
	github.com/juju/loggo v1.0.0
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/mutex v0.0.0-20180619145857-d21b13acf4bf // indirect
	github.com/juju/names/v4 v4.0.0
	github.com/juju/rpcreflect v1.0.0
	github.com/juju/testing v1.0.2
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0 // indirect
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2
	github.com/juju/version v0.0.0-20210303051006-2015802527a8
	github.com/juju/version/v2 v2.0.0
	github.com/juju/zaputil v0.0.0-20190326175239-ef53049637ac
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/prometheus/client_golang v1.13.0
	github.com/rogpeppe/fastuuid v1.2.0
	go.uber.org/zap v1.17.0
	golang.org/x/net v0.4.0 // indirect
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/macaroon-bakery.v2 v2.3.0
	gopkg.in/macaroon.v2 v2.1.0
	gorm.io/driver/postgres v1.0.5
	gorm.io/driver/sqlite v1.1.4-0.20201029040614-e1caf3738eb9
	gorm.io/gorm v1.20.6
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20170523021020-a27b59fe2be9
	github.com/mattn/go-sqlite3 => github.com/mattn/go-sqlite3 v1.14.5
	// This is copied from the go.mod file in github.com/lxc/lxd
	// It is needed to avoid this error when running go list -m
	// go: google.golang.org/grpc/naming@v0.0.0-00010101000000-000000000000: invalid version: unknown revision 000000000000
	google.golang.org/grpc/naming => google.golang.org/grpc v1.29.1
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07
)
