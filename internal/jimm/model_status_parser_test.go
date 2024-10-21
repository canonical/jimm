// Copyright 2024 Canonical.
package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

var now = (time.Time{}).UTC().Round(time.Millisecond)

const crossModelQueryEnv = `
clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
users:
- username: alice@canonical.com
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
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
- name: model-2
  type: iaas
  uuid: 20000000-0000-0000-0000-000000000000
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
- name: model-3
  type: iaas
  uuid: 30000000-0000-0000-0000-000000000000
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
- name: model-5
  type: iaas
  uuid: 50000000-0000-0000-0000-000000000000
  controller: controller-1
  default-series: warty
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
  life: alive
  status:
    status: available
    info: "OK!"
    since: 2020-02-20T20:02:20Z
  users:
  - user: alice@canonical.com
    access: admin
  sla:
    level: unsupported
  agent-version: 1.2.3
`

func getFullStatus(
	modelName string,
	applications map[string]jujuparams.ApplicationStatus,
	remoteApps map[string]jujuparams.RemoteApplicationStatus,
	modelRelations []jujuparams.RelationStatus,

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
		Relations:          modelRelations,
		Branches:           map[string]jujuparams.BranchStatus{},
	}
}

// Model1 holds a model that is running against a K8S controller and has a single app
// waiting for a db relation to a cross-model endpoint via offer.
var model1 = getFullStatus("model-1", map[string]jujuparams.ApplicationStatus{
	"myapp": {
		Charm: "myapp",
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
	[]jujuparams.RelationStatus{
		{
			Id:        0,
			Key:       "myapp",
			Interface: "db",
			Scope:     "regular",
			Endpoints: []jujuparams.EndpointStatus{
				{
					ApplicationName: "myapp",
					Name:            "db",
					Role:            "myrole",
					Subordinate:     false,
				},
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
	nil,
)

// Model3 holds an empty model
var model3 = getFullStatus("model-3", map[string]jujuparams.ApplicationStatus{},
	nil,
	nil,
)

// Model5 holds an empty model, but it's API returns an error for storage
var model5 = getFullStatus("model-5", map[string]jujuparams.ApplicationStatus{},
	nil,
	nil,
)

func TestQueryModelsJq(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Test setup
	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.PostgresDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: client,
		Dialer: jimmtest.ModelDialerMap{
			"10000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model1, nil
					},
					ListFilesystems_: func(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
						return []jujuparams.FilesystemDetailsListResult{
							{
								Result: []jujuparams.FilesystemDetails{
									{
										FilesystemTag: "filesystem-myapp-0-0",
										VolumeTag:     "volume-myapp-0-0",
										Info: jujuparams.FilesystemInfo{
											Size:         4096,
											Pool:         "pool-1",
											FilesystemId: "da64ec3c-0cf7-42f2-9951-35a5a3eaadc1",
										},
										Life: life.Alive,
										Status: jujuparams.EntityStatus{
											Status: status.Active,
											Since:  &now,
										},
										UnitAttachments: map[string]jujuparams.FilesystemAttachmentDetails{
											"filesystem-myapp-0-1": {
												FilesystemAttachmentInfo: jujuparams.FilesystemAttachmentInfo{
													MountPoint: "/home/ubuntu/myapp/.data",
													ReadOnly:   false,
												},
												Life: life.Value(state.Alive.String()),
											},
										},
									},
								},
							},
						}, nil
					},
					ListVolumes_: func(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
						return []jujuparams.VolumeDetailsListResult{}, nil
					},
					ListStorageDetails_: func(ctx context.Context) ([]jujuparams.StorageDetails, error) {
						return []jujuparams.StorageDetails{}, nil
					},
				},
			},
			"20000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model2, nil
					},
					ListFilesystems_: func(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
						return []jujuparams.FilesystemDetailsListResult{}, nil
					},
					ListVolumes_: func(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
						return []jujuparams.VolumeDetailsListResult{}, nil
					},
					ListStorageDetails_: func(ctx context.Context) ([]jujuparams.StorageDetails, error) {
						return []jujuparams.StorageDetails{}, nil
					},
				},
			},
			"30000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model3, nil
					},
					ListFilesystems_: func(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
						return []jujuparams.FilesystemDetailsListResult{}, nil
					},
					ListVolumes_: func(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
						return []jujuparams.VolumeDetailsListResult{}, nil
					},
					ListStorageDetails_: func(ctx context.Context) ([]jujuparams.StorageDetails, error) {
						return []jujuparams.StorageDetails{}, nil
					},
				},
			},
			"50000000-0000-0000-0000-000000000000": &jimmtest.Dialer{
				API: &jimmtest.API{
					Status_: func(_ context.Context, _ []string) (*jujuparams.FullStatus, error) {
						return &model5, nil
					},
					ListFilesystems_: func(ctx context.Context, machines []string) ([]jujuparams.FilesystemDetailsListResult, error) {
						return []jujuparams.FilesystemDetailsListResult{}, errors.E("forcing an error on model 5")
					},
					ListVolumes_: func(ctx context.Context, machines []string) ([]jujuparams.VolumeDetailsListResult, error) {
						return []jujuparams.VolumeDetailsListResult{}, nil
					},
					ListStorageDetails_: func(ctx context.Context) ([]jujuparams.StorageDetails, error) {
						return []jujuparams.StorageDetails{}, nil
					},
				},
			},
		},
	}

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	env := jimmtest.ParseEnvironment(c, crossModelQueryEnv)
	env.PopulateDB(c, j.Database)

	modelUUIDs := []string{
		"10000000-0000-0000-0000-000000000000",
		"20000000-0000-0000-0000-000000000000",
		"30000000-0000-0000-0000-000000000000",
		"40000000-0000-0000-0000-000000000000", // Erroneous model (doesn't exist).
		"50000000-0000-0000-0000-000000000000", // Erroneous model (storage errors).
	}

	// Tests:

	// Query for all models only.
	res, err := j.QueryModelsJq(ctx, modelUUIDs, ".model")
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
			"50000000-0000-0000-0000-000000000000": [
				"forcing an error on model 5"
			]
		}
	}	
	`, qt.JSONEquals, res)

	// Query all applications across all models.
	res, err = j.QueryModelsJq(ctx, modelUUIDs, ".applications")
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
				"provider-id": "10000000-0000-0000-0000-000000000000",
				"relations": {
				  "db": [
					{
						"interface": "db",
						"related-application": "myapp",
						"scope": "regular"
					}
				  ]
				},
				"scale": 1,
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
				"provider-id": "20000000-0000-0000-0000-000000000000",
				"scale": 1,
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
				"provider-id": "20000000-0000-0000-0000-000000000000",
				"scale": 1,
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
			"50000000-0000-0000-0000-000000000000": [
				"forcing an error on model 5"
			]
		}
	}
	`, qt.JSONEquals, res)

	// Query specifically for models including the app "nginx-ingress-integrator"
	res, err = j.QueryModelsJq(ctx, modelUUIDs, ".applications | with_entries(select(.key==\"nginx-ingress-integrator\"))")
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
				"provider-id": "20000000-0000-0000-0000-000000000000",
				"scale": 1,
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
			"50000000-0000-0000-0000-000000000000": [
				"forcing an error on model 5"
			]
		}
	}
	`, qt.JSONEquals, res)

	// Query specifically for storage on this model.
	res, err = j.QueryModelsJq(ctx, modelUUIDs, ".storage")
	c.Assert(err, qt.IsNil)

	// Not the cleanest thing in the world, but this field needs ignoring,
	// and as our struct has a nested map, cmpopts.IgnoreMapFields won't do.
	res.
		Results[modelUUIDs[0]][0].(map[string]any)["filesystems"].(map[string]any)["myapp/0/0"].(map[string]any)["status"].(map[string]any)["since"] = "<ignored>"

	c.Assert(`
	{
		"results": {
		  "10000000-0000-0000-0000-000000000000": [
			{
			  "filesystems": {
				"myapp/0/0": {
				  "Attachments": {
					"containers": {
					  "myapp/0/1": {
						"life": "alive",
						"mount-point": "/home/ubuntu/myapp/.data",
						"read-only": false
					  }
					}
				  },
				  "life": "alive",
				  "pool": "pool-1",
				  "provider-id": "da64ec3c-0cf7-42f2-9951-35a5a3eaadc1",
				  "size": 4096,
				  "status": {
					"current": "active",
					"since": "<ignored>"
				  },
				  "volume": "myapp/0/0"
				}
			  }
			}
		  ],
		  "20000000-0000-0000-0000-000000000000": [
			{}
		  ],
		  "30000000-0000-0000-0000-000000000000": [
			{}
		  ]
		},
		"errors": {
		  "50000000-0000-0000-0000-000000000000": [
			"forcing an error on model 5"
		  ]
		}
	}
	`, qt.JSONEquals, res)
}
