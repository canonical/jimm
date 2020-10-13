module github.com/CanonicalLtd/jimm

go 1.14

require (
	github.com/canonical/candid v1.4.2
	github.com/frankban/quicktest v1.11.0
	github.com/google/go-cmp v0.5.2
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/websocket v1.4.2
	github.com/hashicorp/vault/api v1.0.5-0.20200709165743-f98572ac11c9
	github.com/juju/aclstore v0.0.0-20180706073322-7fc1cdaacf01
	github.com/juju/charm/v8 v8.0.0-20200925052646-bb021a575610
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/errors v0.0.0-20200330140219-3fe23663418f
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/httprequest v1.0.1
	github.com/juju/juju v0.0.0-20201009195340-dcc51b432f65
	github.com/juju/loggo v0.0.0-20200526014432-9ce3a2e09b5e
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/mgosession v1.0.0
	github.com/juju/names/v4 v4.0.0-20200929085019-be23e191fee0
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/rpcreflect v0.0.0-20200416001309-bb46e9ba1476
	github.com/juju/simplekv v1.0.0
	github.com/juju/testing v0.0.0-20200923013621-75df6121fbb0
	github.com/juju/utils v0.0.0-20200604140309-9d78121a29e0
	github.com/juju/version v0.0.0-20191219164919-81c1be00b9a6
	github.com/julienschmidt/httprouter v1.3.0
	github.com/prometheus/client_golang v1.7.1
	github.com/rogpeppe/fastuuid v0.0.0-20150106093220-6724a57986af
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	gopkg.in/check.v1 v1.0.0-20200902074654-038fdea0a05b
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/httprequest.v1 v1.2.1
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

replace gopkg.in/macaroon-bakery.v2-unstable => github.com/wallyworld/macaroon-bakery v0.0.0-20200108032212-15effef1340d
