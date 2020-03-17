module github.com/CanonicalLtd/jimm

require (
	github.com/Azure/go-ntlmssp v0.0.0-20180810175552-4a21cbd618b4 // indirect
	github.com/ChrisTrenkamp/goxpath v0.0.0-20170922090931-c385f95c6022 // indirect
	github.com/altoros/gosigma v0.0.0-20170523021020-a27b59fe2be9 // indirect
	github.com/bmizerany/pat v0.0.0-20170815010413-6226ea591a40 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/docker/distribution v2.7.0+incompatible // indirect
	github.com/elazarl/goproxy v0.0.0-20190911111923-ecfe977594f1 // indirect
	github.com/flosch/pongo2 v0.0.0-20180809100617-24195e6d38b0 // indirect
	github.com/google/go-cmp v0.4.0
	github.com/google/go-querystring v0.0.0-20170111101155-53e6ce116135 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20181103185306-d547d1d9531e // indirect
	github.com/gorilla/handlers v0.0.0-20170224193955-13d73096a474
	github.com/gorilla/schema v1.0.2 // indirect
	github.com/gorilla/websocket v1.4.0
	github.com/hashicorp/raft v1.0.0 // indirect
	github.com/joyent/gocommon v0.0.0-20161202192317-b78708995d1c // indirect
	github.com/joyent/gosdc v0.0.0-20161202192312-ec8b3503a75e // indirect
	github.com/joyent/gosign v0.0.0-20161114191744-9abcee278795 // indirect
	github.com/juju/aclstore v0.0.0-20180706073322-7fc1cdaacf01
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a // indirect
	github.com/juju/clock v0.0.0-20190205081909-9c5c9712527c
	github.com/juju/cmd v0.0.0-20200108104440-8e43f3faa5c9
	github.com/juju/errors v0.0.0-20190930114154-d42613fe1ab9
	github.com/juju/gnuflag v0.0.0-20171113085948-2ce1bb71843d
	github.com/juju/go-oracle-cloud v0.0.0-20170510162943-95ad2a088ab9 // indirect
	github.com/juju/httprequest v1.0.1
	github.com/juju/juju v0.0.0-20200302114844-534e9744d306
	github.com/juju/loggo v0.0.0-20190526231331-6e530bcce5d8
	github.com/juju/mgomonitor v0.0.0-20181029151116-52206bb0cd31
	github.com/juju/mgosession v1.0.0
	github.com/juju/names v0.0.0-20160330150533-8a0aa0963bba
	github.com/juju/persistent-cookiejar v0.0.0-20170428161559-d67418f14c93
	github.com/juju/rpcreflect v0.0.0-20190806165913-cca922e065df
	github.com/juju/simplekv v1.0.0
	github.com/juju/testing v0.0.0-20191001232224-ce9dec17d28b
	github.com/juju/usso v1.0.1 // indirect
	github.com/juju/utils v0.0.0-20200116185830-d40c2fe10647
	github.com/juju/version v0.0.0-20191219164919-81c1be00b9a6
	github.com/julienschmidt/httprouter v1.3.0
	github.com/lestrrat/go-jspointer v0.0.0-20180109105637-d5f7c71bfd03 // indirect
	github.com/lestrrat/go-jsref v0.0.0-20170215062819-50df7b2d07d7 // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20161018213530-a6a42341b50d // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161021065934-cf70aae60f5b // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20180220043741-569c97477ae8 // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20180223064246-8204d40bbcd7 // indirect
	github.com/lunixbochs/vtclean v0.0.0-20180621232353-2d01aacdc34a // indirect
	github.com/masterzen/winrm v0.0.0-20181112102303-a196a4ff2a86 // indirect
	github.com/mattn/go-runewidth v0.0.3 // indirect
	github.com/prometheus/client_golang v1.0.0
	github.com/rogpeppe/fastuuid v0.0.0-20150106093220-6724a57986af
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20191206172530-e9b2fee46413
	gopkg.in/CanonicalLtd/candidclient.v1 v1.2.0
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/httprequest.v1 v1.2.0
	gopkg.in/ini.v1 v1.39.0 // indirect
	gopkg.in/juju/charmrepo.v3 v3.0.2-0.20191105112621-5ca139ef9e6b // indirect
	gopkg.in/juju/environschema.v1 v1.0.0
	gopkg.in/juju/names.v3 v3.0.0-20200131033104-139ecaca454c
	gopkg.in/juju/worker.v1 v1.0.0-20191018043616-19a698a7150f
	gopkg.in/macaroon-bakery.v2 v2.2.0
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/retry.v1 v1.0.3
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.2.8
)

replace (
	github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20170523021020-a27b59fe2be9
	gopkg.in/mgo.v2 => github.com/juju/mgo v0.0.0-20190418114320-e9d4866cb7fc
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20181113141958-2025133c3826
)

replace github.com/hashicorp/raft => github.com/juju/raft v1.0.1-0.20190319034642-834fca2f9ffc

go 1.12

replace gopkg.in/macaroon-bakery.v2-unstable => github.com/wallyworld/macaroon-bakery v0.0.0-20200108032212-15effef1340d

replace github.com/juju/juju => github.com/mhilton/juju-juju v0.0.0-20200306113540-bbcb8d9711a2
