// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/names/v4"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestGetCloud(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-1",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "alice@external",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username: "bob@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-2",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "everyone@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	alice := &dbmodel.User{Username: "alice@external"}
	bob := &dbmodel.User{Username: "bob@external"}
	charlie := &dbmodel.User{Username: "charlie@external"}
	daphne := &dbmodel.User{
		Username:         "daphne@external",
		ControllerAccess: "superuser",
	}

	_, err = j.GetCloud(ctx, alice, names.NewCloudTag("test-cloud-0"))
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	_, err = j.GetCloud(ctx, charlie, names.NewCloudTag("test-cloud-1"))
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)

	cld, err := j.GetCloud(ctx, alice, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, bob, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, daphne, names.NewCloudTag("test-cloud-1"))
	c.Assert(err, qt.IsNil)
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	})

	cld, err = j.GetCloud(ctx, charlie, names.NewCloudTag("test-cloud-2"))
	c.Check(cld, qt.DeepEquals, dbmodel.Cloud{
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User: dbmodel.User{
				Username: "charlie@external",
			},
			Access: "add-model",
		}},
	})
}

func TestForEachCloud(t *testing.T) {
	c := qt.New(t)

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-1",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "alice@external",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username: "bob@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-2",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "bob@external",
			},
			Access: "add-model",
		}, {
			User: dbmodel.User{
				Username: "everyone@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	err = j.Database.AddCloud(ctx, &dbmodel.Cloud{
		Name: "test-cloud-3",
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username: "everyone@external",
			},
			Access: "add-model",
		}},
	})
	c.Assert(err, qt.IsNil)

	alice := &dbmodel.User{Username: "alice@external"}
	bob := &dbmodel.User{Username: "bob@external"}
	charlie := &dbmodel.User{Username: "charlie@external"}
	daphne := &dbmodel.User{
		Username:         "daphne@external",
		ControllerAccess: "superuser",
	}

	var clds []dbmodel.Cloud
	err = j.ForEachCloud(ctx, alice, false, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	}, {
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User:     *alice,
			Access:   "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "alice@external",
			User:     *alice,
			Access:   "add-model",
		}},
	}})

	clds = clds[:0]
	err = j.ForEachCloud(ctx, bob, false, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}, {
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "bob@external",
			User:     *bob,
			Access:   "add-model",
		}},
	}})

	clds = clds[:0]
	err = j.ForEachCloud(ctx, charlie, false, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User:     *charlie,
			Access:   "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Username: "charlie@external",
			User:     *charlie,
			Access:   "add-model",
		}},
	}})

	clds = clds[:0]
	err = j.ForEachCloud(ctx, daphne, true, func(cld *dbmodel.Cloud) error {
		clds = append(clds, *cld)
		return nil
	})
	c.Assert(err, qt.IsNil)
	c.Check(clds, qt.DeepEquals, []dbmodel.Cloud{{
		ID:        1,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-1",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "alice@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "alice@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "admin",
		}, {
			Model: gorm.Model{
				ID:        2,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-1",
			Access:    "add-model",
		}},
	}, {
		ID:        2,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-2",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        3,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "bob@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        2,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "bob@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-2",
			Access:    "add-model",
		}, {
			Model: gorm.Model{
				ID:        4,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "everyone@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        3,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "everyone@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-2",
			Access:    "add-model",
		}},
	}, {
		ID:        3,
		CreatedAt: now,
		UpdatedAt: now,
		Name:      "test-cloud-3",
		Regions:   []dbmodel.CloudRegion{},
		Users: []dbmodel.UserCloudAccess{{
			Model: gorm.Model{
				ID:        5,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Username: "everyone@external",
			User: dbmodel.User{
				Model: gorm.Model{
					ID:        3,
					CreatedAt: now,
					UpdatedAt: now,
				},
				Username:         "everyone@external",
				ControllerAccess: "add-model",
			},
			CloudName: "test-cloud-3",
			Access:    "add-model",
		}},
	}})
}
