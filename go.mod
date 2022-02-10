module github.com/CanonicalLtd/jimm

go 1.16

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/canonical/candid v1.10.1
	github.com/canonical/go-dqlite v1.8.0
	github.com/canonical/go-service v1.0.0
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/frankban/quicktest v1.13.0
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.0-20210309064400-d73aa8f92aa2
	github.com/gobwas/glob v0.2.4-0.20181002190808-e7a84e9525fe // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.2.0
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.1
	github.com/hashicorp/vault/api v1.1.0
	github.com/jackc/pgconn v1.7.0
	github.com/jackc/pgx/v4 v4.9.0
	github.com/juju/aclstore/v2 v2.1.0 // indirect
	github.com/juju/charm/v8 v8.0.0-20220131000804-ba1fde373826
	github.com/juju/cmd/v3 v3.0.0-20210809234809-65029dab4cd0
	github.com/juju/errors v0.0.0-20210818161939-5560c4c073ff
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/juju v0.0.0-20220201060324-f1924dcf59c8
	github.com/juju/loggo v0.0.0-20210728185423-eebad3a902c4
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/testing v0.0.0-20210324180055-18c50b0c2098
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0 // indirect
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2
	github.com/juju/version v0.0.0-20210303051006-2015802527a8
	github.com/juju/version/v2 v2.0.0-20211007103408-2e8da085dc23
	github.com/juju/zaputil v0.0.0-20190326175239-ef53049637ac
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.29.0 // indirect
	github.com/rogpeppe/fastuuid v1.2.0
	go.uber.org/zap v1.10.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b // indirect
	golang.org/x/tools v0.1.4 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/juju/names.v2 v2.0.0-20190813004204-e057c73bd1be
	gopkg.in/macaroon-bakery.v2 v2.3.0
	gopkg.in/macaroon.v2 v2.1.0
	gorm.io/driver/postgres v1.0.5
	gorm.io/driver/sqlite v1.1.4-0.20201029040614-e1caf3738eb9
	gorm.io/gorm v1.20.6
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20170523021020-a27b59fe2be9
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07
)

replace github.com/mattn/go-sqlite3 => github.com/mattn/go-sqlite3 v1.14.5
