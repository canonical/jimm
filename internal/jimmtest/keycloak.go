// Copyright 2024 Canonical.

package jimmtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/errors"
)

// These constants are based on the `docker-compose.yaml` and `local/keycloak/jimm-realm.json` content.
const (
	// HardcodedSafeUsername is a hardcoded test keycloak user that pre-exists
	// but is safe for use in a Juju UserTag when the email is retrieved.
	HardcodedSafeUsername = "jimm-test"
	HardcodedSafePassword = "password"
	// HardcodedUnsafeUsername is a hardcoded test keycloak user that pre-exists
	// but is unsafe for use in a Juju UserTag when the email is retrieved.
	HardcodedUnsafeUsername = "jimm_test"
	HardcodedUnsafePassword = "password"

	keycloakHost             = "localhost:8082"
	keycloakJIMMRealmPath    = "/admin/realms/jimm"
	keycloakAdminUsername    = "jimm"
	keycloakAdminPassword    = "jimm"
	keycloakAdminCLIUsername = "admin-cli"
	//nolint:gosec // Thinks credentials exposed. Only used for test.
	keycloakAdminCLISecret = "DOLcuE5Cd7IxuR7JE4hpAUxaLF7RlAWh"
)

// KeycloakUser represents a basic user created in Keycloak.
type KeycloakUser struct {
	Id       string
	Email    string
	Username string
	Password string
}

// CreateRandomKeycloakUser creates a Keycloak user with random username and
// returns the created user details.
func CreateRandomKeycloakUser() (*KeycloakUser, error) {
	username := "random-user-" + uuid.New().String()[0:8]
	email := username + "@canonical.com"
	password := "jimm"

	adminCLIToken, err := getAdminCLIAccessToken()
	if err != nil {
		zapctx.Error(context.Background(), "failed to authenticate admin CLI user", zap.Error(err))
		return nil, errors.E(err, "failed to authenticate admin CLI user")
	}

	if err := addKeycloakUser(adminCLIToken, email, username); err != nil {
		zapctx.Error(context.Background(), "failed to add keycloak user", zap.Error(err))
		return nil, errors.E(err, fmt.Sprintf("failed to add keycloak user (%q, %q)", email, username))
	}

	id, err := getKeycloakUserId(adminCLIToken, username)
	if err != nil {
		zapctx.Error(context.Background(), "failed to get keycloak user ID", zap.Error(err))
		return nil, errors.E(err, fmt.Sprintf("failed to retrieve ID for newly added keycloak user (%q, %q)", email, username))
	}

	if err := setKeycloakUserPassword(adminCLIToken, id, password); err != nil {
		zapctx.Error(context.Background(), "failed to set keycloak user password", zap.Error(err))
		return nil, errors.E(err, fmt.Sprintf("failed to set password for newly added keycloak user (%q, %q, %q)", email, username, password))
	}
	return &KeycloakUser{
		Id:       id,
		Email:    email,
		Username: username,
		Password: password,
	}, nil
}

// getAdminCLIAccessToken authenticates with the `admin-cli` client and returns
// the access token to be used to communicate with Keycloak admin API.
func getAdminCLIAccessToken() (string, error) {
	httpClient := http.Client{}
	u := url.URL{
		Scheme: "http",
		Host:   keycloakHost,
		User:   url.UserPassword(keycloakAdminCLIUsername, keycloakAdminCLISecret),
		Path:   "/realms/master/protocol/openid-connect/token",
	}
	reqBody := url.Values{}
	reqBody.Set("username", keycloakAdminUsername)
	reqBody.Set("password", keycloakAdminPassword)
	reqBody.Set("grant_type", "password")
	resp, err := httpClient.Post(
		u.String(),
		"application/x-www-form-urlencoded",
		strings.NewReader(reqBody.Encode()),
	)
	if err != nil {
		return "", errors.E(err, "failed to login with keycloak admin CLI user")
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.E(err, fmt.Sprintf("failed to read keycloak response for admin CLI login (status-code: %d)", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.E(fmt.Sprintf("failed to login with keycloak admin CLI user (status-code: %d): %q", resp.StatusCode, string(body)))
	}

	m := map[string]any{}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", errors.E(err, fmt.Sprintf("failed to parse keycloak response for admin CLI login: %q", string(body)))
	}

	if _, ok := m["access_token"]; !ok {
		return "", errors.E(err, fmt.Sprintf("cannot find access token in keycloak response: %q", string(body)))
	}
	if token, ok := m["access_token"].(string); !ok {
		return "", errors.E(err, fmt.Sprintf("received token is not string: %v", m["access_token"]))
	} else {
		return token, nil
	}
}

// getKeycloakUsersMap returns a map of Keycloak users, associating usernames to IDs.
func getKeycloakUsersMap(adminCLIToken string) (map[string]string, error) {
	httpClient := http.Client{}
	u := url.URL{
		Scheme: "http",
		Host:   keycloakHost,
		Path:   keycloakJIMMRealmPath + "/users",
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+adminCLIToken)
	req.Header.Add("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.E(err, "failed to get users from keycloak")
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to read keycloak response for list of users (status-code: %d)", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.E(fmt.Sprintf("failed to get users from keycloak (status-code: %d): %q", resp.StatusCode, string(body)))
	}

	var raw []struct {
		Id       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to parse keycloak response for list of users: %q", string(body)))
	}

	result := map[string]string{}
	for _, entry := range raw {
		result[entry.Username] = entry.Id
	}
	return result, nil
}

// getKeycloakUserId returns the Keycloak user ID of a given username.
func getKeycloakUserId(adminCLIToken, username string) (string, error) {
	m, err := getKeycloakUsersMap(adminCLIToken)
	if err != nil {
		return "", err
	}

	if id, ok := m[username]; !ok {
		return "", errors.E(fmt.Sprintf("keycloak user not found: %q", username))
	} else {
		return id, nil
	}
}

// addKeycloakUser adds a user (username/email pair) to Keycloak.
func addKeycloakUser(adminCLIToken, email, username string) error {
	httpClient := http.Client{}
	u := url.URL{
		Scheme: "http",
		Host:   keycloakHost,
		Path:   keycloakJIMMRealmPath + "/users",
	}

	reqBody := map[string]any{
		"username":      username,
		"email":         email,
		"emailVerified": true,
		"enabled":       true,
		"realmRoles":    []string{"user", "offline_access"},
	}

	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(reqBodyJSON))
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+adminCLIToken)
	req.Header.Add("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.E(err, "failed to add user to keycloak")
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.E(err, fmt.Sprintf("failed to read keycloak response to add user (status-code: %d)", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return errors.E(fmt.Sprintf("failed to add user to keycloak (status-code: %d): %q", resp.StatusCode, string(body)))
	}
	return nil
}

// setKeycloakUserPassword sets the password for given Keycloak user (identified by its ID).
func setKeycloakUserPassword(adminCLIToken, id, password string) error {
	httpClient := http.Client{}
	u := url.URL{
		Scheme: "http",
		Host:   keycloakHost,
		Path:   fmt.Sprintf("admin/realms/jimm/users/%s/reset-password", id),
	}

	reqBody := map[string]any{
		"type":      "password",
		"temporary": false,
		"value":     password,
	}

	reqBodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, u.String(), bytes.NewReader(reqBodyJSON))
	if err != nil {
		return err
	}

	req.Header.Add("Authorization", "Bearer "+adminCLIToken)
	req.Header.Add("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.E(err, "failed to set keycloak user password")
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.E(err, fmt.Sprintf("failed to read keycloak response to set user password (status-code: %d)", resp.StatusCode))
	}
	if resp.StatusCode != http.StatusNoContent {
		return errors.E(fmt.Sprintf("failed to set keycloak user password (status-code: %d): %q", resp.StatusCode, string(body)))
	}
	return nil
}
