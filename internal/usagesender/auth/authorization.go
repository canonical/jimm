// Copyright 2018 Canonical Ltd.

// auth contains the client for retrieving usage authorizations.
package auth

import (
	"context"

	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
)

type getCredentialsRequest struct {
	httprequest.Route `httprequest:"POST /v4/jimm/authorization"`
	Body              credentialsRequest `httprequest:",body"`
}

type credentialsResponse struct {
	Credentials []byte `json:"credentials"`
}

type credentialsRequest struct {
	Tags map[string]string `json:"tags"`
}

// NewAuthorizationClient creates a new client for retrieving user authorizations.
func NewAuthorizationClient(baseURL string, client *httpbakery.Client) *authorizationClient {
	return &authorizationClient{
		client: &httprequest.Client{
			BaseURL: baseURL,
			Doer:    client,
		},
	}
}

type authorizationClient struct {
	client *httprequest.Client
}

// GetCredentials issues an API call to acquire the credentials for the specified user.
func (c authorizationClient) GetCredentials(ctx context.Context, user string) ([]byte, error) {
	var resp credentialsResponse

	if err := c.client.Call(
		ctx,
		&getCredentialsRequest{
			Body: credentialsRequest{
				Tags: map[string]string{"user": user},
			}},
		&resp,
	); err != nil {
		return nil, errgo.Mask(err)
	}
	return resp.Credentials, nil
}
