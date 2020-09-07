module github.com/CanonicalLtd/jimm

require (
	github.com/armon/go-metrics v0.3.3 // indirect
	github.com/aws/aws-sdk-go v1.30.10 // indirect
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40 // indirect
	github.com/canonical/candid v1.4.2
	github.com/flosch/pongo2 v0.0.0-20180809100617-24195e6d38b0 // indirect
	github.com/frankban/quicktest v1.10.0
	github.com/google/go-cmp v0.5.1
	github.com/gopherjs/gopherjs v0.0.0-20181103185306-d547d1d9531e // indirect
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/schema v1.0.2 // indirect
	github.com/gorilla/websocket v1.4.2
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/hashicorp/go-msgpack v1.1.5 // indirect
	github.com/hashicorp/vault/api v1.0.5-0.20200709165743-f98572ac11c9
	github.com/joyent/gocommon v0.0.0-20161202192317-b78708995d1c // indirect
	github.com/joyent/gosdc v0.0.0-20161202192312-ec8b3503a75e // indirect
	github.com/joyent/gosign v0.0.0-20161114191744-9abcee278795 // indirect
	github.com/juju/aclstore v0.0.0-20180706073322-7fc1cdaacf01
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/httprequest v1.0.1
	github.com/juju/juju v0.0.0-20200907071117-a5e6e716f158
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/lru v0.0.0-20190314140547-92a0afabdc41 // indirect
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/mgosession v1.0.0
	github.com/juju/names/v4 v4.0.0-20200424054733-9a8294627524
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/simplekv v1.0.0
	github.com/juju/testing v0.0.0-20200706033705-4c23f9c453cd
	github.com/juju/txn v0.0.0-20190612234757-afeb83d59782 // indirect
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/version v0.0.0-20191219164919-81c1be00b9a6
	github.com/julienschmidt/httprouter v1.3.0
	github.com/lestrrat/go-jspointer v0.0.0-20180109105637-d5f7c71bfd03 // indirect
	github.com/lestrrat/go-jsref v0.0.0-20170215062819-50df7b2d07d7 // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20161018213530-a6a42341b50d // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161021065934-cf70aae60f5b // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20180220043741-569c97477ae8 // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20180223064246-8204d40bbcd7 // indirect
	github.com/masterzen/simplexml v0.0.0-20190410153822-31eea3082786 // indirect
	github.com/mattn/go-runewidth v0.0.3 // indirect
	github.com/prometheus/client_golang v1.7.1
	github.com/rogpeppe/fastuuid v0.0.0-20150106093220-6724a57986af
	github.com/vmware/govmomi v0.22.2 // indirect
	go.uber.org/multierr v1.5.0 // indirect
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20200728195943-123391ffb6de
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	gopkg.in/amz.v3 v3.0.0-20200811022415-7b63e5e39741 // indirect
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/ini.v1 v1.39.0 // indirect
	gopkg.in/juju/charm.v6 v6.0.0-20200415131143-ad2e04a67e7b // indirect
	gopkg.in/juju/charmrepo.v3 v3.0.2-0.20191105112621-5ca139ef9e6b // indirect
	gopkg.in/juju/environschema.v1 v1.0.0
	gopkg.in/juju/worker.v1 v1.0.0-20191018043616-19a698a7150f
	gopkg.in/macaroon-bakery.v2 v2.2.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.3.0
)

replace (
	github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20170523021020-a27b59fe2be9
	gopkg.in/mgo.v2 => github.com/juju/mgo v0.0.0-20190418114320-e9d4866cb7fc
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20181113141958-2025133c3826
)

replace github.com/hashicorp/raft => github.com/juju/raft v1.0.1-0.20190319034642-834fca2f9ffc

go 1.12

replace gopkg.in/macaroon-bakery.v2-unstable => github.com/wallyworld/macaroon-bakery v0.0.0-20200108032212-15effef1340d
