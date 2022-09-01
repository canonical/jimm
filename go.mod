module github.com/CanonicalLtd/jimm

go 1.16

require (
	cloud.google.com/go/compute v1.8.0 // indirect
	github.com/Azure/go-autorest/autorest v0.11.24 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/canonical/candid v1.11.0
	github.com/canonical/go-dqlite v1.11.1
	github.com/canonical/go-service v1.0.0
	github.com/frankban/quicktest v1.14.3
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20220204130128-afeebcc9521d
	github.com/gobwas/glob v0.2.4-0.20181002190808-e7a84e9525fe // indirect
	github.com/google/go-cmp v0.5.8
	github.com/google/uuid v1.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/vault/api v1.1.0
	github.com/jackc/pgconn v1.7.0
	github.com/jackc/pgx/v4 v4.9.0
	github.com/juju/aclstore/v2 v2.1.0 // indirect
	github.com/juju/charm/v8 v8.0.1
	github.com/juju/charm/v9 v9.0.4
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9 // indirect
	github.com/juju/cmd/v3 v3.0.0
	github.com/juju/errors v1.0.0
	github.com/juju/gnuflag v1.0.0
	github.com/juju/juju v0.0.0-20220804232154-95186b2e0c2d
	github.com/juju/loggo v1.0.0
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/mutex v0.0.0-20180619145857-d21b13acf4bf // indirect
	github.com/juju/names/v4 v4.0.0
	github.com/juju/rpcreflect v1.0.0
	github.com/juju/testing v1.0.1
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2
	github.com/juju/version v0.0.0-20210303051006-2015802527a8
	github.com/juju/version/v2 v2.0.1
	github.com/juju/zaputil v0.0.0-20190326175239-ef53049637ac
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.29.0 // indirect
	github.com/rogpeppe/fastuuid v1.2.0
	go.uber.org/zap v1.17.0
	golang.org/x/crypto v0.0.0-20220829220503-c86fa9a7ed90 // indirect
	golang.org/x/net v0.0.0-20220826154423-83b083e8dc8b // indirect
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.0.0-20220829200755-d48e67d00261 // indirect
	golang.org/x/term v0.0.0-20220722155259-a9ba230a4035 // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/genproto v0.0.0-20220829175752-36a9c930ecbf // indirect
	google.golang.org/grpc v1.49.0 // indirect
	google.golang.org/grpc/examples v0.0.0-20220831213702-ddcda5f76a3b // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/juju/names.v2 v2.0.0-20190813004204-e057c73bd1be // indirect
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
	google.golang.org/grpc/naming => google.golang.org/grpc v1.29.1
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07
)
