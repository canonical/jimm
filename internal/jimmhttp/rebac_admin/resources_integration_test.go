// Copyright 2024 Canonical.
package rebac_admin_test

import (
	"context"
	"strings"

	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
	"github.com/canonical/rebac-admin-ui-handlers/v1/resources"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/common/utils"
	"github.com/canonical/jimm/v3/internal/jimmhttp/rebac_admin"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type resourcesSuite struct {
	jimmtest.JIMMSuite
}

var _ = gc.Suite(&resourcesSuite{})

// resourcesTestEnv is used to create entries in JIMM's database.
// The rebacAdminSuite does not spin up a Juju controller so we cannot use
// regular JIMM methods to create resources.
const resourcesTestEnv = `clouds:
- name: test-cloud
  type: test-provider
  regions:
  - name: test-cloud-region
cloud-credentials:
- owner: alice@canonical.com
  name: cred-1
  cloud: test-cloud
controllers:
- name: controller-1
  uuid: 00000001-0000-0000-0000-000000000001
  cloud: test-cloud
  region: test-cloud-region
models:
- name: model-1
  uuid: 00000002-0000-0000-0000-000000000001
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
- name: model-2
  uuid: 00000002-0000-0000-0000-000000000002
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
- name: model-3
  uuid: 00000003-0000-0000-0000-000000000003
  controller: controller-1
  cloud: test-cloud
  region: test-cloud-region
  cloud-credential: cred-1
  owner: alice@canonical.com
`

func (s *resourcesSuite) TestListResources(c *gc.C) {
	ctx := context.Background()
	ctx = rebac_handlers.ContextWithIdentity(ctx, s.AdminUser)
	resourcesSvc := rebac_admin.NewResourcesService(s.JIMM)
	tester := jimmtest.GocheckTester{C: c}
	env := jimmtest.ParseEnvironment(tester, resourcesTestEnv)
	env.PopulateDB(tester, s.JIMM.Database)
	type testEntity struct {
		Id       string
		Name     string
		ParentId string
		Type     string
	}
	ids := make([]testEntity, 0)
	for _, c := range env.Clouds {
		ids = append(ids, testEntity{
			Id:       c.Name,
			Name:     c.Name,
			ParentId: "",
			Type:     "clouds",
		})
	}
	for _, c := range env.Controllers {
		ids = append(ids, testEntity{
			Id:       c.UUID,
			Name:     c.Name,
			ParentId: "",
			Type:     "controllers",
		})
	}
	for _, m := range env.Models {
		ids = append(ids, testEntity{
			Id:       m.UUID,
			Name:     m.Name,
			ParentId: env.Controller(m.Controller).UUID,
			Type:     "models",
		})
	}

	testCases := []struct {
		desc         string
		size         *int
		page         *int
		nameFilter   string
		typeFilter   string
		wantPage     int
		wantSize     int
		wantNextpage *int
		ids          []testEntity
	}{
		{
			desc:         "test with first page",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(0),
			wantPage:     0,
			wantSize:     2,
			wantNextpage: utils.IntToPointer(1),
			ids:          []testEntity{ids[0], ids[1]},
		},
		{
			desc:         "test with second page",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(1),
			wantPage:     1,
			wantSize:     2,
			wantNextpage: utils.IntToPointer(2),
			ids:          []testEntity{ids[2], ids[3]},
		},
		{
			desc:         "test with last page",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(2),
			wantPage:     2,
			wantSize:     1,
			wantNextpage: nil,
			ids:          []testEntity{ids[4]},
		},
		{
			desc:         "test first page with model name filter",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(0),
			nameFilter:   "model",
			wantPage:     0,
			wantSize:     2,
			wantNextpage: utils.IntToPointer(1),
			ids: func() []testEntity {
				filteredIds := make([]testEntity, 0)
				for _, id := range ids {
					if strings.HasPrefix(id.Name, "model") {
						filteredIds = append(filteredIds, id)
					}
				}
				return filteredIds
			}()[:2],
		},
		{
			desc:         "test first page with model name filter",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(1),
			nameFilter:   "model",
			wantPage:     1,
			wantSize:     1,
			wantNextpage: nil,
			ids: func() []testEntity {
				filteredIds := make([]testEntity, 0)
				for _, id := range ids {
					if strings.Contains(id.Name, "model") {
						filteredIds = append(filteredIds, id)
					}
				}
				return filteredIds
			}()[2:],
		},
		{
			desc:         "test first page with controller entity type",
			size:         utils.IntToPointer(2),
			page:         utils.IntToPointer(0),
			typeFilter:   rebac_admin.Controller,
			wantPage:     0,
			wantSize:     1,
			wantNextpage: nil,
			ids: func() []testEntity {
				filteredIds := make([]testEntity, 0)
				for _, id := range ids {
					if id.Type == "controllers" {
						filteredIds = append(filteredIds, id)
					}
				}
				return filteredIds
			}(),
		},
		{
			desc:         "test big page with model entity type",
			size:         utils.IntToPointer(10),
			page:         utils.IntToPointer(0),
			typeFilter:   rebac_admin.Model,
			wantPage:     0,
			wantSize:     3,
			wantNextpage: nil,
			ids:          []testEntity{},
		},
	}
	for _, t := range testCases {
		resources, err := resourcesSvc.ListResources(ctx, &resources.GetResourcesParams{
			Size:       t.size,
			Page:       t.page,
			EntityName: &t.nameFilter,
			EntityType: &t.typeFilter,
		})
		c.Assert(err, gc.IsNil)
		c.Assert(*resources.Meta.Page, gc.Equals, t.wantPage)
		c.Assert(resources.Meta.Size, gc.Equals, t.wantSize)
		if t.wantNextpage == nil {
			c.Assert(resources.Next.Page, gc.IsNil)
		} else {
			c.Assert(*resources.Next.Page, gc.Equals, *t.wantNextpage)
		}
		for i := range len(t.ids) {
			c.Assert(resources.Data[i].Entity.Id, gc.Equals, t.ids[i].Id)
			if t.ids[i].ParentId != "" {
				c.Assert(resources.Data[i].Parent.Id, gc.Equals, t.ids[i].ParentId)
			}
		}
	}
}
