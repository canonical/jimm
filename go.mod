module github.com/canonical/jimm/v3

go 1.22.5

require (
	github.com/antonlindstrom/pgstore v0.0.0-20220421113606-e3a6e3fed12a
	github.com/canonical/go-service v1.0.0
	github.com/canonical/ofga v0.10.0
	github.com/canonical/rebac-admin-ui-handlers v0.1.2
	github.com/coreos/go-oidc/v3 v3.9.0
	github.com/dustinkirkland/golang-petname v0.0.0-20231002161417-6a283f1aaaf2
	github.com/frankban/quicktest v1.14.6
	github.com/go-chi/chi/v5 v5.0.12
	github.com/go-chi/render v1.0.2
	github.com/go-macaroon-bakery/macaroon-bakery/v3 v3.0.1
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/sessions v1.2.1
	github.com/gorilla/websocket v1.5.1
	github.com/gosuri/uitable v0.0.4
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/hashicorp/vault/api v1.13.0
	github.com/hashicorp/vault/api/auth/approle v0.6.0
	github.com/itchyny/gojq v0.12.12
	github.com/jackc/pgconn v1.14.3
	github.com/jackc/pgx/v4 v4.18.3
	github.com/juju/charm/v12 v12.0.2
	github.com/juju/cmd/v3 v3.0.15
	github.com/juju/errors v1.0.0
	github.com/juju/gnuflag v1.0.0
	github.com/juju/http/v2 v2.0.1
	github.com/juju/juju v0.0.0-20240730101146-fe07e5f4cbd7
	github.com/juju/loggo v1.0.0
	github.com/juju/names/v4 v4.0.0
	github.com/juju/names/v5 v5.0.0
	github.com/juju/rpcreflect v1.2.0
	github.com/juju/testing v1.1.0
	github.com/juju/utils/v2 v2.0.0-20210305225158-eedbe7b6b3e2
	github.com/juju/version v0.0.0-20210303051006-2015802527a8
	github.com/juju/version/v2 v2.0.1
	github.com/juju/zaputil v0.0.0-20190326175239-ef53049637ac
	github.com/lestrrat-go/iter v1.0.2
	github.com/lestrrat-go/jwx/v2 v2.0.21
	github.com/mattn/go-colorable v0.1.13
	github.com/oklog/ulid/v2 v2.1.0
	github.com/openfga/go-sdk v0.2.2
	github.com/prometheus/client_golang v1.19.1
	github.com/rogpeppe/fastuuid v1.2.0
	github.com/rs/cors v1.11.1
	github.com/stretchr/testify v1.9.0
	go.uber.org/zap v1.24.0
	golang.org/x/oauth2 v0.21.0
	golang.org/x/sync v0.7.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/errgo.v1 v1.0.1
	gopkg.in/httprequest.v1 v1.2.1
	gopkg.in/macaroon.v2 v2.1.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/postgres v1.0.5
	gorm.io/gorm v1.20.6
	sigs.k8s.io/yaml v1.3.0
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.9.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3 v3.0.0-beta.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2 v2.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault v1.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork v1.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys v1.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.0.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/Rican7/retry v0.3.1 // indirect
	github.com/adrg/xdg v0.3.3 // indirect
	github.com/ajg/form v1.5.1 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.24.0 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.26.2 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.16.13 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.14.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.2.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.5.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.7.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.142.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.24.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/iam v1.28.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.10.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.10.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.18.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.21.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.26.6 // indirect
	github.com/aws/smithy-go v1.19.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/canonical/go-dqlite v1.21.0 // indirect
	github.com/canonical/lxd v0.0.0-20231214113525-e676fc63c50a // indirect
	github.com/cenkalti/backoff/v3 v3.2.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cjlapao/common-go v0.0.39 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/creack/pty v1.1.15 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/flosch/pongo2 v0.0.0-20200913210552-0d938eb266f3 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/gdamore/tcell/v2 v2.5.1 // indirect
	github.com/getkin/kin-openapi v0.125.0 // indirect
	github.com/go-goose/goose/v5 v5.0.0-20230421180421-abaee9096e3a // indirect
	github.com/go-jose/go-jose/v3 v3.0.3 // indirect
	github.com/go-jose/go-jose/v4 v4.0.1 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-macaroon-bakery/macaroonpb v1.0.0 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.8 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.22.0 // indirect
	github.com/gobwas/glob v0.2.4-0.20181002190808-e7a84e9525fe // indirect
	github.com/goccy/go-json v0.10.2 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/renameio v1.0.1 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/gorilla/schema v1.4.1 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.8 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.6 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/im7mortal/kmutex v1.0.1 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/invopop/yaml v0.2.0 // indirect
	github.com/itchyny/timefmt-go v0.1.5 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgtype v1.14.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/juju/ansiterm v1.0.0 // indirect
	github.com/juju/blobstore/v3 v3.0.2 // indirect
	github.com/juju/clock v1.1.1 // indirect
	github.com/juju/collections v1.0.4 // indirect
	github.com/juju/description/v5 v5.0.4 // indirect
	github.com/juju/featureflag v1.0.0 // indirect
	github.com/juju/go4 v0.0.0-20160222163258-40d72ab9641a // indirect
	github.com/juju/gojsonpointer v0.0.0-20150204194629-afe8b77aa08f // indirect
	github.com/juju/gojsonreference v0.0.0-20150204194633-f0d24ac5ee33 // indirect
	github.com/juju/gojsonschema v1.0.0 // indirect
	github.com/juju/gomaasapi/v2 v2.2.0 // indirect
	github.com/juju/idmclient/v2 v2.0.0 // indirect
	github.com/juju/jsonschema v1.0.0 // indirect
	github.com/juju/loggo/v2 v2.0.0 // indirect
	github.com/juju/lru v1.0.0 // indirect
	github.com/juju/lumberjack/v2 v2.0.2 // indirect
	github.com/juju/mgo/v2 v2.0.2 // indirect
	github.com/juju/mgo/v3 v3.0.4 // indirect
	github.com/juju/mutex/v2 v2.0.0 // indirect
	github.com/juju/naturalsort v1.0.0 // indirect
	github.com/juju/os/v2 v2.2.5 // indirect
	github.com/juju/packaging/v3 v3.1.0 // indirect
	github.com/juju/persistent-cookiejar v1.0.0 // indirect
	github.com/juju/proxy v1.0.0 // indirect
	github.com/juju/pubsub/v2 v2.0.0 // indirect
	github.com/juju/ratelimit v1.0.2 // indirect
	github.com/juju/replicaset/v3 v3.0.1 // indirect
	github.com/juju/retry v1.0.0 // indirect
	github.com/juju/rfc/v2 v2.0.0 // indirect
	github.com/juju/romulus v1.0.0 // indirect
	github.com/juju/schema v1.2.0 // indirect
	github.com/juju/txn/v3 v3.0.2 // indirect
	github.com/juju/usso v1.0.1 // indirect
	github.com/juju/utils/v3 v3.1.1 // indirect
	github.com/juju/viddy v0.0.0-beta5 // indirect
	github.com/juju/webbrowser v1.0.0 // indirect
	github.com/juju/worker/v3 v3.5.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/httprc v1.0.5 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/lestrrat/go-jspointer v0.0.0-20160229021354-f4881e611bdb // indirect
	github.com/lestrrat/go-jsref v0.0.0-20160601013240-e452c7b5801d // indirect
	github.com/lestrrat/go-jsschema v0.0.0-20160903131957-b09d7650b822 // indirect
	github.com/lestrrat/go-jsval v0.0.0-20161012045717-b1258a10419f // indirect
	github.com/lestrrat/go-pdebug v0.0.0-20160817063333-2e6eaaa5717f // indirect
	github.com/lestrrat/go-structinfo v0.0.0-20160308131105-f74c056fe41f // indirect
	github.com/lib/pq v1.10.7 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mattn/go-sqlite3 v2.0.3+incompatible // indirect
	github.com/microsoft/kiota-abstractions-go v1.5.3 // indirect
	github.com/microsoft/kiota-authentication-azure-go v1.0.1 // indirect
	github.com/microsoft/kiota-http-go v1.1.1 // indirect
	github.com/microsoft/kiota-serialization-form-go v1.0.0 // indirect
	github.com/microsoft/kiota-serialization-json-go v1.0.4 // indirect
	github.com/microsoft/kiota-serialization-multipart-go v1.0.0 // indirect
	github.com/microsoft/kiota-serialization-text-go v1.0.0 // indirect
	github.com/microsoftgraph/msgraph-sdk-go v1.28.0 // indirect
	github.com/microsoftgraph/msgraph-sdk-go-core v1.0.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mittwald/vaultgo v0.1.4 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.7.1 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oapi-codegen/runtime v1.1.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/oracle/oci-go-sdk/v65 v65.55.0 // indirect
	github.com/packethost/packngo v0.28.1 // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/sftp v1.13.6 // indirect
	github.com/pkg/xattr v0.4.9 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rivo/tview v0.0.0-20220610163003-691f46d6f500 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rogpeppe/go-internal v1.12.0 // indirect
	github.com/rs/xid v1.5.0 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/sergi/go-diff v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/sony/gobreaker v0.5.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.17.0 // indirect
	github.com/std-uritemplate/std-uritemplate/go v0.0.47 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/vishvananda/netlink v1.2.1-beta.2 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/vmware/govmomi v0.34.1 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/zitadel/oidc/v2 v2.12.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.46.1 // indirect
	go.opentelemetry.io/otel v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.21.0 // indirect
	go.opentelemetry.io/otel/trace v1.21.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/mock v0.4.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.25.0 // indirect
	golang.org/x/exp v0.0.0-20231127185646-65229373498e // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	golang.org/x/term v0.22.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/api v0.154.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/gobwas/glob.v0 v0.2.3 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/juju/environschema.v1 v1.0.1 // indirect
	gopkg.in/retry.v1 v1.0.3 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637 // indirect
	k8s.io/api v0.29.0 // indirect
	k8s.io/apiextensions-apiserver v0.29.0 // indirect
	k8s.io/apimachinery v0.29.0 // indirect
	k8s.io/client-go v0.29.0 // indirect
	k8s.io/klog/v2 v2.110.1 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	k8s.io/utils v0.0.0-20231127182322-b307cd553661 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
)

replace (
	github.com/altoros/gosigma => github.com/juju/gosigma v0.0.0-20170523021020-a27b59fe2be9
	gopkg.in/yaml.v2 => github.com/juju/yaml v0.0.0-20200420012109-12a32b78de07
)

replace github.com/mattn/go-sqlite3 => github.com/mattn/go-sqlite3 v1.14.5
