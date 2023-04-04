package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofga "github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	jimmnames "github.com/CanonicalLtd/jimm/pkg/names"
)

var now = (time.Time{}).UTC().Round(time.Millisecond)

const crossModelQueryEnv = `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@external
  name: cred-1
  cloud: test-cloud
users:
- username: alice@external
  controller-access: superuser
controllers:
- name: controller-1
  uuid: 10000000-0000-0000-0000-000000000000
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  type: iaas
  uuid: 10000000-0000-0000-0000-000000000000
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
- name: model-2
  type: iaas
  uuid: 20000000-0000-0000-0000-000000000000
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
- name: model-3
  type: iaas
  uuid: 30000000-0000-0000-0000-000000000000
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@external
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@external
    access: admin
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func getFullStatus(
	modelName string,
	applications map[string]jujuparams.ApplicationStatus,
	remoteApps map[string]jujuparams.RemoteApplicationStatus,
) jujuparams.FullStatus {
	return jujuparams.FullStatus{
		Model: jujuparams.ModelStatusInfo{
			Name:             modelName,
			Type:             "caas",
			CloudTag:         "cloud-microk8s",
			CloudRegion:      "localhost",
			Version:          "2.9.37",
			AvailableVersion: "",
			ModelStatus: jujuparams.DetailedStatus{
				Status: "available",
				Info:   "",
				Since:  &now,
				Data:   map[string]interface{}{},
			},
			SLA: "unsupported",
		},
		Machines:           map[string]jujuparams.MachineStatus{},
		Applications:       applications,
		RemoteApplications: remoteApps,
		Offers:             map[string]jujuparams.ApplicationOfferStatus{},
		Relations:          []jujuparams.RelationStatus(nil),
		Branches:           map[string]jujuparams.BranchStatus{},
	}
}

// Model1 holds a model that is running against a K8S controller and has a single app
// waiting for a db relation to a cross-model endpoint via offer.
var model1 = getFullStatus("model-1", map[string]jujuparams.ApplicationStatus{
	"myapp": {
		Charm:  "myapp",
		Series: "kubernetes",
		// handle charm-origin/name/rev/channel alex,
		Scale:         1,
		ProviderId:    "10000000-0000-0000-0000-000000000000",
		PublicAddress: "10.152.183.177",
		Exposed:       false,
		Status: jujuparams.DetailedStatus{ // todo handle this alex
			Status: "idle",
			Info:   "",
			Since:  &now,
		},
		Relations: map[string][]string{
			"db": {"myapp"},
		},
		Units: map[string]jujuparams.UnitStatus{
			"myapp/0": {
				AgentStatus: jujuparams.DetailedStatus{
					Status:  "idle",
					Version: "2.9.37",
					Since:   &now,
				},
				WorkloadStatus: jujuparams.DetailedStatus{
					Status: "blocked",
					Info:   "waiting for db relation",
					Since:  &now,
				},
				Leader:     true,
				Address:    "10.1.160.61",
				ProviderId: "myapp-0",
			},
		},
		EndpointBindings: map[string]string{
			"":        "alpha",
			"db":      "alpha",
			"ingress": "alpha",
			"myapp":   "alpha",
		},
	},
},
	map[string]jujuparams.RemoteApplicationStatus{
		"postgresql": {
			OfferURL: "lxdcloud:admin/db.postgresql",
			Endpoints: []jujuparams.RemoteEndpoint{
				{
					Name:      "db",
					Interface: "pgsql",
					Role:      "provider",
				},
			},
			Status: jujuparams.DetailedStatus{
				Status: "active",
				Info:   "Live master (12.14)",
				Since:  &now,
			},
		},
	},
)

// Model2 holds a model that is running against a K8S controller and has two apps.
// One that is currently deploying. It's unit status' are waiting on the agent installing.
// Another which is the ingress for this app.
// These apps have not been related.
//
// TODO(ale8k): How do we simulate storage? As this is just status... See newstatusformatter line 62, so we can test the below.
// Additionally, it has persistent volumes (filesystem).
// See: https://warthogs.atlassian.net/browse/CSS-3478
var model2 = getFullStatus("model-2", map[string]jujuparams.ApplicationStatus{
	"hello-kubecon": {
		Charm:         "hello-kubecon",
		Series:        "kubernetes",
		Scale:         1,
		CharmVersion:  "17",
		CharmChannel:  "idk",
		CharmProfile:  "idk",
		ProviderId:    "20000000-0000-0000-0000-000000000000",
		PublicAddress: "10.152.183.177",
		Exposed:       false,
		Status: jujuparams.DetailedStatus{ // todo handle this alex
			Status: "waiting",
			Info:   "installing agent",
			Since:  &now,
		},
		Relations: map[string][]string{},
		Units: map[string]jujuparams.UnitStatus{
			"hello-kubecon/0": {
				AgentStatus: jujuparams.DetailedStatus{
					Status:  "allocating",
					Version: "2.9.37",
					Since:   &now,
				},
				WorkloadStatus: jujuparams.DetailedStatus{
					Status: "waiting",
					Info:   "installing agent",
					Since:  &now,
				},
				Leader:     true,
				Address:    "10.1.160.62",
				ProviderId: "hello-kubecon-0",
			},
		},
		EndpointBindings: map[string]string{
			"":        "alpha",
			"ingress": "alpha",
		},
	},
	"nginx-ingress-integrator": {
		Charm:         "nginx-ingress-integrator",
		Series:        "kubernetes",
		Scale:         1,
		CharmVersion:  "54",
		CharmChannel:  "idk",
		CharmProfile:  "idk",
		ProviderId:    "20000000-0000-0000-0000-000000000000",
		PublicAddress: "10.152.183.167",
		Exposed:       true,
		Status: jujuparams.DetailedStatus{ // todo handle this alex
			Status: "active",
			Since:  &now,
		},
		Relations: map[string][]string{},
		Units: map[string]jujuparams.UnitStatus{
			"nginx-ingress-integrator/0": {
				AgentStatus: jujuparams.DetailedStatus{
					Status:  "idle",
					Version: "2.9.37",
					Since:   &now,
				},
				WorkloadStatus: jujuparams.DetailedStatus{
					Status: "active",
					Since:  &now,
				},
				Leader:     true,
				Address:    "10.1.160.63",
				ProviderId: "nginx-ingress-integrator-0",
			},
		},
		EndpointBindings: map[string]string{
			"":        "alpha",
			"ingress": "alpha",
		},
	},
},
	nil,
)

// Model3 holds an empty model
var model3 = getFullStatus("model-3", map[string]jujuparams.ApplicationStatus{},
	nil,
)

func TestQueryModelsJq(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Test setup
	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: client,
		Dialer: jimmtest.ModelDialerMap{
			"10000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model1, nil
					},
				},
			},
			"20000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model2, nil
					},
				},
			},
			"30000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model3, nil
					},
				},
			},
		},
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, crossModelQueryEnv)
	env.PopulateDB(c, j.Database, nil)

	modelUUIDs := []string{
		"10000000-0000-0000-0000-000000000000",
		"20000000-0000-0000-0000-000000000000",
		"30000000-0000-0000-0000-000000000000",
		"40000000-0000-0000-0000-000000000000", // Erroneous model (doesn't exist).
	}

	c.Assert(j.OpenFGAClient.AddRelations(ctx,
		[]ofga.Tuple{
			// Reader to model via direct relation
			{
				Object:   ofganames.FromTag(names.NewUserTag("alice@external")),
				Relation: ofganames.ReaderRelation,
				Target:   ofganames.FromTag(names.NewModelTag(modelUUIDs[0])),
			},
			// Reader to model via group
			{
				Object:   ofganames.FromTag(names.NewUserTag("alice@external")),
				Relation: ofganames.MemberRelation,
				Target:   ofganames.FromTag(jimmnames.NewGroupTag("1")),
			},
			{
				Object:   ofganames.FromTagWithRelation(jimmnames.NewGroupTag("1"), ofganames.MemberRelation),
				Relation: ofganames.ReaderRelation,
				Target:   ofganames.FromTag(names.NewModelTag(modelUUIDs[1])),
			},
			// Reader to model via administrator of controller
			{
				Object:   ofganames.FromTag(names.NewUserTag("alice@external")),
				Relation: ofganames.AdministratorRelation,
				Target:   ofganames.FromTag(names.NewControllerTag("00000000-0000-0000-0000-000000000000")),
			},
			{
				Object:   ofganames.FromTag(names.NewControllerTag("00000000-0000-0000-0000-000000000000")),
				Relation: ofganames.ControllerRelation,
				Target:   ofganames.FromTag(names.NewModelTag(modelUUIDs[2])),
			},
			// Reader to model via direct relation that does NOT exist
			{
				Object:   ofganames.FromTag(names.NewUserTag("alice@external")),
				Relation: ofganames.ReaderRelation,
				Target:   ofganames.FromTag(names.NewModelTag(modelUUIDs[3])),
			},
		}...,
	), qt.Equals, nil)

	alice := openfga.NewUser(
		&dbmodel.User{
			Username: "alice@external",
		},
		client,
	)

	// Tests:

	// Query for all models only.
	res, err := j.QueryModelsJq(ctx, alice, ".model")
	c.Assert(err, qt.IsNil)
	c.Assert(`
	{
		"results": {
			"10000000-0000-0000-0000-000000000000": [
			{
				"cloud": "microk8s",
				"controller": "controller-1",
				"model-status": {
				"current": "available",
				"since": "0001-01-01 00:00:00Z"
				},
				"name": "model-1",
				"region": "localhost",
				"sla": "unsupported",
				"type": "caas",
				"version": "2.9.37"
			}
			],
			"20000000-0000-0000-0000-000000000000": [
			{
				"cloud": "microk8s",
				"controller": "controller-1",
				"model-status": {
				"current": "available",
				"since": "0001-01-01 00:00:00Z"
				},
				"name": "model-2",
				"region": "localhost",
				"sla": "unsupported",
				"type": "caas",
				"version": "2.9.37"
			}
			],
			"30000000-0000-0000-0000-000000000000": [
			{
				"cloud": "microk8s",
				"controller": "controller-1",
				"model-status": {
				"current": "available",
				"since": "0001-01-01 00:00:00Z"
				},
				"name": "model-3",
				"region": "localhost",
				"sla": "unsupported",
				"type": "caas",
				"version": "2.9.37"
			}
			]
		},
		"errors": {
			"40000000-0000-0000-0000-000000000000": [
				"model not found"
			]
		}
	}	
	`, qt.JSONEquals, res)

	// Query all applications across all models.
	res, err = j.QueryModelsJq(ctx, alice, ".applications")
	c.Assert(err, qt.IsNil)
	c.Assert(`
	{
		"results": {
		  "10000000-0000-0000-0000-000000000000": [
			{
			  "myapp": {
				"address": "10.152.183.177",
				"application-status": {
				  "current": "idle",
				  "since": "0001-01-01 00:00:00Z"
				},
				"charm": "myapp",
				"charm-name": "myapp",
				"charm-origin": "charmhub",
				"charm-rev": -1,
				"endpoint-bindings": {
				  "": "alpha",
				  "db": "alpha",
				  "ingress": "alpha",
				  "myapp": "alpha"
				},
				"exposed": false,
				"os": "kubernetes",
				"provider-id": "10000000-0000-0000-0000-000000000000",
				"relations": {
				  "db": [
					"myapp"
				  ]
				},
				"scale": 1,
				"series": "kubernetes",
				"units": {
				  "myapp/0": {
					"address": "10.1.160.61",
					"juju-status": {
					  "current": "idle",
					  "since": "0001-01-01 00:00:00Z",
					  "version": "2.9.37"
					},
					"leader": true,
					"provider-id": "myapp-0",
					"workload-status": {
					  "current": "blocked",
					  "message": "waiting for db relation",
					  "since": "0001-01-01 00:00:00Z"
					}
				  }
				}
			  }
			}
		  ],
		  "20000000-0000-0000-0000-000000000000": [
			{
			  "hello-kubecon": {
				"address": "10.152.183.177",
				"application-status": {
				  "current": "waiting",
				  "message": "installing agent",
				  "since": "0001-01-01 00:00:00Z"
				},
				"charm": "hello-kubecon",
				"charm-channel": "idk",
				"charm-name": "hello-kubecon",
				"charm-origin": "charmhub",
				"charm-profile": "idk",
				"charm-rev": -1,
				"charm-version": "17",
				"endpoint-bindings": {
				  "": "alpha",
				  "ingress": "alpha"
				},
				"exposed": false,
				"os": "kubernetes",
				"provider-id": "20000000-0000-0000-0000-000000000000",
				"scale": 1,
				"series": "kubernetes",
				"units": {
				  "hello-kubecon/0": {
					"address": "10.1.160.62",
					"juju-status": {
					  "current": "allocating",
					  "since": "0001-01-01 00:00:00Z",
					  "version": "2.9.37"
					},
					"leader": true,
					"provider-id": "hello-kubecon-0",
					"workload-status": {
					  "current": "waiting",
					  "message": "installing agent",
					  "since": "0001-01-01 00:00:00Z"
					}
				  }
				}
			  },
			  "nginx-ingress-integrator": {
				"address": "10.152.183.167",
				"application-status": {
				  "current": "active",
				  "since": "0001-01-01 00:00:00Z"
				},
				"charm": "nginx-ingress-integrator",
				"charm-channel": "idk",
				"charm-name": "nginx-ingress-integrator",
				"charm-origin": "charmhub",
				"charm-profile": "idk",
				"charm-rev": -1,
				"charm-version": "54",
				"endpoint-bindings": {
				  "": "alpha",
				  "ingress": "alpha"
				},
				"exposed": true,
				"os": "kubernetes",
				"provider-id": "20000000-0000-0000-0000-000000000000",
				"scale": 1,
				"series": "kubernetes",
				"units": {
				  "nginx-ingress-integrator/0": {
					"address": "10.1.160.63",
					"juju-status": {
					  "current": "idle",
					  "since": "0001-01-01 00:00:00Z",
					  "version": "2.9.37"
					},
					"leader": true,
					"provider-id": "nginx-ingress-integrator-0",
					"workload-status": {
					  "current": "active",
					  "since": "0001-01-01 00:00:00Z"
					}
				  }
				}
			  }
			}
		  ],
		  "30000000-0000-0000-0000-000000000000": [
			{}
		  ]
		},
		"errors": {
			"40000000-0000-0000-0000-000000000000": [
				"model not found"
			]
		}
	}
	`, qt.JSONEquals, res)

	// Query specifically for models including the app "nginx-ingress-integrator"
	res, err = j.QueryModelsJq(ctx, alice, ".applications | with_entries(select(.key==\"nginx-ingress-integrator\"))")
	c.Assert(err, qt.IsNil)
	c.Assert(`
	{
		"results": {
		  "10000000-0000-0000-0000-000000000000": [
			{}
		  ],
		  "20000000-0000-0000-0000-000000000000": [
			{
			  "nginx-ingress-integrator": {
				"address": "10.152.183.167",
				"application-status": {
				  "current": "active",
				  "since": "0001-01-01 00:00:00Z"
				},
				"charm": "nginx-ingress-integrator",
				"charm-channel": "idk",
				"charm-name": "nginx-ingress-integrator",
				"charm-origin": "charmhub",
				"charm-profile": "idk",
				"charm-rev": -1,
				"charm-version": "54",
				"endpoint-bindings": {
				  "": "alpha",
				  "ingress": "alpha"
				},
				"exposed": true,
				"os": "kubernetes",
				"provider-id": "20000000-0000-0000-0000-000000000000",
				"scale": 1,
				"series": "kubernetes",
				"units": {
				  "nginx-ingress-integrator/0": {
					"address": "10.1.160.63",
					"juju-status": {
					  "current": "idle",
					  "since": "0001-01-01 00:00:00Z",
					  "version": "2.9.37"
					},
					"leader": true,
					"provider-id": "nginx-ingress-integrator-0",
					"workload-status": {
					  "current": "active",
					  "since": "0001-01-01 00:00:00Z"
					}
				  }
				}
			  }
			}
		  ],
		  "30000000-0000-0000-0000-000000000000": [
			{}
		  ]
		},
		"errors": {
			"40000000-0000-0000-0000-000000000000": [
				"model not found"
			]
		}
	}
	`, qt.JSONEquals, res)

	// Query specifically for storage on this model.
	res, err = j.QueryModelsJq(ctx, alice, ".storage")
	c.Assert(err, qt.IsNil)
	c.Assert(`
	{
		"results": {
		  "10000000-0000-0000-0000-000000000000": [
			{}
		  ],
		  "20000000-0000-0000-0000-000000000000": [
			{}
		  ],
		  "30000000-0000-0000-0000-000000000000": [
			{}
		  ]
		},
		"errors": {
			"40000000-0000-0000-0000-000000000000": [
				"model not found"
			]
		}
	}
	`, qt.JSONEquals, res)
}
