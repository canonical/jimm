# Description
The following is a set of commands that can be used to deploy a local jimm-k8s 

# Commands
Deploy stateful applications (Vault and Postgresql) and tls-certificates operator to enable TLS for Postgres (which is required for Vault to work)
```
juju bootstrap localhost test
juju add-model db
juju add-cloud microk8s --controller test
juju add-model jimm microk8s
juju switch db
juju deploy postgresql --channel=14/edge
juju deploy tls-certificates-operator --config generate-self-signed-certificates=true --config ca-common-name="TestCA"
juju deploy vault --series=focal
juju relate tls-certificates-operator postgresql
juju relate vault:db postgresql:db
```
Deploy JIMM
```
juju switch jimm
//From root directory
make push-microk8s
//Switch to jimm-k8s charm directory
charmcraft pack
juju deploy ./juju-jimm-k8s_ubuntu-20.04-amd64.charm --resource jimm-image="localhost:32000/jimm:latest" --config uuid=ff77dbd0-ab87-444e-b9c7-768c675bf59d --config dns-name=juju-jimm-k8s-0.juju-jimm-k8s-endpoints.jimm.svc.cluster.local --config 
candid-url="https://api.staging.jujucharms.com/identity"
// The following commands can be skipped but will prevent
// JIMM from communicating with Candid.
juju config juju-jimm-k8s private-key=<removed> 
juju config juju-jimm-k8s public-key=<removed>
juju config juju-jimm-k8s candid-public-key=<removed>
juju config juju-jimm-k8s candid-agent-username=<removed>
juju config juju-jimm-k8s candid-agent-public-key=<removed>
juju config juju-jimm-k8s candid-agent-private-key=<removed>
```
Deploy OPNEFGA, make relations and run setup actions
```
juju deploy openfga-k8s --series=jammy --channel=latest/edge --revision=5
juju relate juju-jimm-k8s openfga-k8s
juju relate juju-jimm-k8s vault
juju relate juju-jimm-k8s postgresql
juju run-action openfga-k8s/0 schema-upgrade --wait
// Create a file called auth_model.yaml that looks like the following
//model: >
//  <json_here>
juju run-action juju-jimm-k8s/leader create-authorization-model --params auth_model.yaml --wait
lxc config device add <vault-lxc-unit>  myproxy proxy listen=tcp:127.0.0.1:8200 connect=tcp:0.0.0.0:8200 bind=host
```
