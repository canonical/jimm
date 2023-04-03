// Copyright 2023 Canonical Ltd.

package jimm_test

// TODO(Kian): We could use a test as below to unit test the
// auth function returned by JwtGenerator and ensure it correctly
// generates a JWT. The JWTGenerator requires the JIMM server to have
// a JWT service and cache setup, so we could either turn this into
// an interface and mock it or have the test start a full JIMM server.
// Also required an interface for authentication, mocked or Candid.
// This is already tested in an integration test in jujuapi/websocket_test.go
// func TestJwtGenerator(t *testing.T) {
// 	c := qt.New(t)

// 	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
// 	c.Assert(err, qt.IsNil)

// 	j := &jimm.JIMM{
// 		UUID: uuid.NewString(),
// 		Database: db.Database{
// 			DB: jimmtest.MemoryDB(c, nil),
// 		},
// 		OpenFGAClient: client,
// 		JWTService:    jimmjwx.NewJWTService(,),
// 	}
// 	ctx := context.Background()
// 	m := &dbmodel.Model{}
// 	authFunc := j.JwtGenerator(ctx, m)
// 	loginReq := new(jujuparams.LoginRequest)
// 	desiredPerms := map[string]interface{}{"model-123": "writer", "applicationoffer-123": "consumer"}
// 	token, err := authFunc(loginReq, desiredPerms)
// 	c.Assert(err, qt.IsNil)
// 	//Check token is valid and has correct assertions.
// }
