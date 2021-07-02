module github.com/CanonicalLtd/jimm

go 1.16

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/canonical/candid v1.4.2
	github.com/canonical/go-dqlite v1.8.0
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/frankban/quicktest v1.11.3
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20210309064400-d73aa8f92aa2
	github.com/gobwas/glob v0.2.4-0.20181002190808-e7a84e9525fe // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/go-cmp v0.5.5
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/vault/api v1.0.5-0.20200709165743-f98572ac11c9
	github.com/jackc/pgconn v1.7.0
	github.com/jackc/pgx/v4 v4.9.0
	github.com/juju/aclstore v0.0.0-20180706073322-7fc1cdaacf01
	github.com/juju/charm/v8 v8.0.0-20210615153946-469f7216a7d5
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/juju v0.0.0-20210630005308-95b319ca0ac1
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/mgo/v2 v2.0.0-20210414025616-e854c672032f
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/mgosession/v2 v2.0.0
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/simplekv v1.0.1
	github.com/juju/testing v0.0.0-20210324180055-18c50b0c2098
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2
	github.com/juju/version/v2 v2.0.0-20210319015800-dcfac8f4f057
	github.com/julienschmidt/httprouter v1.3.0
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/prometheus/client_golang v1.7.1
	github.com/rogpeppe/fastuuid v1.2.0
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b // indirect
	golang.org/x/tools v0.1.4 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/juju/environschema.v1 v1.0.1-0.20201027142642-c89a4490670a
	gopkg.in/juju/worker.v1 v1.0.0-20191018043616-19a698a7150f
	gopkg.in/macaroon-bakery.v2 v2.3.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gorm.io/driver/postgres v1.0.5
	gorm.io/driver/sqlite v1.1.4-0.20201029040614-e1caf3738eb9
	gorm.io/gorm v1.20.6
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20200420012028-063911838a9e
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07
)

replace github.com/hashicorp/raft => github.com/juju/raft v2.0.0-20200420012049-88ad3b3f0a54+incompatible

replace github.com/mattn/go-sqlite3 => github.com/mattn/go-sqlite3 v1.14.5
