package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/logger"
	petname "github.com/dustinkirkland/golang-petname"
	"github.com/google/uuid"
	"github.com/juju/juju/core/crossmodel"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// A simple script to seed a local database for schema testing.

func main() {
	ctx := context.Background()

	gdb, err := gorm.Open(postgres.Open("postgresql://jimm:jimm@localhost/jimm"), &gorm.Config{
		Logger: logger.GormLogger{},
	})

	if err != nil {
		fmt.Println("failed to connect to db ", err)
		os.Exit(1)
	}

	db := db.Database{
		DB: gdb,
	}

	db.Migrate(ctx, false)
	if err != nil {
		fmt.Println("failed to migrate to db ", err)
		os.Exit(1)
	}

	if err = db.AddGroup(ctx, "test-group"); err != nil {
		fmt.Println("failed to add group to db ", err)
		os.Exit(1)
	}

	u := dbmodel.User{
		Username: petname.Generate(2, "-") + "@external",
	}
	if err = db.DB.Create(&u).Error; err != nil {
		fmt.Println("failed to add user to db ", err)
		os.Exit(1)
	}

	cloud := dbmodel.Cloud{
		Name: petname.Generate(2, "-"),
		Type: "aws",
		Regions: []dbmodel.CloudRegion{{
			Name: petname.Generate(2, "-"),
		}},
	}
	if err = db.DB.Create(&cloud).Error; err != nil {
		fmt.Println("failed to add cloud to db ", err)
		os.Exit(1)
	}

	id, _ := uuid.NewRandom()
	controller := dbmodel.Controller{
		Name:        petname.Generate(2, "-"),
		UUID:        id.String(),
		CloudName:   cloud.Name,
		CloudRegion: cloud.Regions[0].Name,
		CloudRegions: []dbmodel.CloudRegionControllerPriority{{
			Priority:      0,
			CloudRegionID: cloud.Regions[0].ID,
		}},
	}
	if err = db.AddController(ctx, &controller); err != nil {
		fmt.Println("failed to add controller to db ", err)
		os.Exit(1)
	}

	cred := dbmodel.CloudCredential{
		Name:          petname.Generate(2, "-"),
		CloudName:     cloud.Name,
		OwnerUsername: u.Username,
		AuthType:      "empty",
	}
	if err = db.SetCloudCredential(ctx, &cred); err != nil {
		fmt.Println("failed to add cloud credential to db ", err)
		os.Exit(1)
	}

	model := dbmodel.Model{
		Name: petname.Generate(2, "-"),
		UUID: sql.NullString{
			String: id.String(),
			Valid:  true,
		},
		OwnerUsername:     u.Username,
		ControllerID:      controller.ID,
		CloudRegionID:     cloud.Regions[0].ID,
		CloudCredentialID: cred.ID,
		Life:              "alive",
		Status: dbmodel.Status{
			Status: "available",
			Since: sql.NullTime{
				Time:  time.Now().UTC().Truncate(time.Millisecond),
				Valid: true,
			},
		},
	}
	if err = db.AddModel(ctx, &model); err != nil {
		fmt.Println("failed to add model to db ", err)
		os.Exit(1)
	}

	offerName := petname.Generate(2, "-")
	offerURL, _ := crossmodel.ParseOfferURL(controller.Name + ":" + u.Username + "/" + model.Name + "." + offerName)
	offer := dbmodel.ApplicationOffer{
		UUID:            id.String(),
		Name:            offerName,
		ModelID:         model.ID,
		ApplicationName: petname.Generate(2, "-"),
		URL:             offerURL.String(),
	}
	if err = db.AddApplicationOffer(context.Background(), &offer); err != nil {
		fmt.Println("failed to add application offer to db ", err)
		os.Exit(1)
	}

	fmt.Println("DB seeded.")
}
