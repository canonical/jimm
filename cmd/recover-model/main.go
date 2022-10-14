// Copyright 2019 Canonical Ltd.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	envconfig "github.com/juju/juju/environs/config"
	"github.com/juju/names/v4"
	errgo "gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon-bakery.v2/httpbakery/agent"
	mgo "gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jimm/config"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

var configFile = flag.String("config", "config.yaml", "configuration `file` path")

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: recover-model [options] <controller> <model>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if len(flag.Args()) != 2 {
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := config.Read(*configFile)
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "recover-model: cannot read config file: %s\n", err)
		os.Exit(1)
	}

	if err := recoverModel(context.Background(), cfg, flag.Arg(0), flag.Arg(1)); err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "recover-model: cannot recover model: %s\n", err)
		os.Exit(1)
	}
}

func recoverModel(ctx context.Context, cfg *config.Config, controller, model string) error {
	session, err := mgo.Dial(cfg.MongoAddr)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", cfg.MongoAddr)
	}
	defer session.Close()
	if cfg.DBName == "" {
		cfg.DBName = "jem"
	}
	db := session.DB(cfg.DBName)

	bclient := httpbakery.NewClient()
	err = agent.SetUpAuth(bclient, &agent.AuthInfo{
		Key: cfg.AgentKey,
		Agents: []agent.Agent{{
			URL:      cfg.IdentityLocation,
			Username: cfg.AgentUsername,
		}},
	})
	if err != nil {
		return errgo.Notef(err, "cannot initialize agent")
	}

	p, err := jem.NewPool(ctx, jem.Params{
		DB:              db,
		SessionPool:     mgosession.NewPool(ctx, session, 100),
		ControllerAdmin: cfg.ControllerAdmin,
	})
	if err != nil {
		return errgo.Notef(err, "cannot access JIMM database")
	}
	defer p.Close()

	j := p.JEM(ctx)
	defer j.Close()

	var cPath params.EntityPath
	if err := cPath.UnmarshalText([]byte(controller)); err != nil {
		return errgo.Notef(err, "invalid controller")
	}

	conn, err := j.OpenAPI(ctx, cPath)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller %q", controller)
	}
	defer conn.Close()

	mm := modelmanager.NewClient(conn)
	ms, err := mm.ListModels(conn.Info.Tag.(names.UserTag).Id())
	if err != nil {
		return errgo.Notef(err, "cannot list controller models")
	}
	for _, m := range ms {
		if m.Owner+"/"+m.Name != model {
			continue
		}
		mi, err := modelInfo(mm, m.UUID)
		if err != nil {
			return errgo.Notef(err, "cannot get model info for %q", model)
		}
		doc, err := mongodocFromModelInfo(ctx, mi)
		if err != nil {
			return errgo.Mask(err)
		}
		doc.Controller = cPath
		return errgo.Mask(j.DB.InsertModel(ctx, doc))
	}

	return errgo.Newf("cannot find model %q", model)
}

func modelInfo(mm *modelmanager.Client, uuid string) (*jujuparams.ModelInfo, error) {
	res, err := mm.ModelInfo([]names.ModelTag{names.NewModelTag(uuid)})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if len(res) != 1 {
		return nil, errgo.Newf("unexpected number of model-info responses (%d)", len(res))
	}
	if res[0].Error != nil {
		return nil, errgo.Mask(res[0].Error)
	}
	return res[0].Result, nil
}

func mongodocFromModelInfo(ctx context.Context, mi *jujuparams.ModelInfo) (*mongodoc.Model, error) {
	ot, err := names.ParseUserTag(mi.OwnerTag)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	owner := strings.TrimSuffix(ot.Id(), "@external")
	mPath := params.EntityPath{
		User: params.User(owner),
		Name: params.Name(mi.Name),
	}
	info := mongodoc.ModelInfo{
		Life: string(mi.Life),
		Status: mongodoc.ModelStatus{
			Status:  string(mi.Status.Status),
			Message: mi.Status.Info,
			Data:    mi.Status.Data,
		},
	}
	if mi.Status.Since != nil {
		info.Status.Since = *mi.Status.Since
	}
	if mi.AgentVersion != nil {
		info.Config = map[string]interface{}{
			envconfig.AgentVersionKey: mi.AgentVersion.String(),
		}
	}

	ct, err := names.ParseCloudTag(mi.CloudTag)
	if err != nil {
		return nil, errgo.Mask(err)
	}

	var acl params.ACL
	for _, u := range mi.Users {
		if strings.IndexByte(u.UserName, '@') == -1 {
			// local users do not interest us.
			continue
		}
		username := strings.TrimSuffix(u.UserName, "@external")
		switch u.Access {
		case jujuparams.ModelAdminAccess:
			acl.Admin = append(acl.Admin, username)
			fallthrough
		case jujuparams.ModelWriteAccess:
			acl.Write = append(acl.Write, username)
			fallthrough
		case jujuparams.ModelReadAccess:
			acl.Read = append(acl.Read, username)
		}
	}

	cct, err := names.ParseCloudCredentialTag(mi.CloudCredentialTag)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	cred := mongodoc.CredentialPath{
		Cloud: cct.Cloud().Id(),
		EntityPath: mongodoc.EntityPath{
			User: strings.TrimSuffix(cct.Owner().Id(), "@external"),
			Name: cct.Name(),
		},
	}

	return &mongodoc.Model{
		Path:          mPath,
		ACL:           acl,
		UUID:          mi.UUID,
		Info:          &info,
		Creator:       owner,
		Cloud:         params.Cloud(ct.Id()),
		CloudRegion:   mi.CloudRegion,
		DefaultSeries: mi.DefaultSeries,
		Type:          mi.Type,
		ProviderType:  mi.ProviderType,
		Credential:    cred,
	}, nil
}
