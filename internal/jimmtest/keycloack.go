// Copyright 2024 Canonical Ltd.

package jimmtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/canonical/jimm/internal/errors"
	"github.com/google/uuid"
)

// These constants are based on the `docker-compose.yaml` and `local/keycloak/jimm-realm.json` content.
const (
	keycloakHost             = "localhost:8082"
	keycloakJIMMRealmPath    = "/admin/realms/jimm"
	keycloakAdminUsername    = "jimm"
	keycloakAdminPassword    = "jimm"
	keycloakAdminCLIUsername = "admin-cli"
	keycloakAdminCLISecret   = "DOLcuE5Cd7IxuR7JE4hpAUxaLF7RlAWh"
)

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
	username := "random_user_" + uuid.New().String()[0:8]
	email := username + "@canonical.com"
	password := "jimm"

	adminCLIToken, err := getAdminCLIAccessToken()
	if err != nil {
		return nil, errors.E(err, "failed to authenticate admin CLI user")
	}

	if err := addKeycloakUser(adminCLIToken, email, username); err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to add keycloak user (%q, %q)", email, username))
	}

	id, err := getKeycloakUserId(adminCLIToken, username)
	if err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to retrieve ID for newly added keycloak user (%q, %q)", email, username))
	}

	if err := setKeycloakUserPassword(adminCLIToken, id, password); err != nil {
		return nil, errors.E(err, fmt.Sprintf("failed to set password for newly added keycloak user (%q, %q, %q)", email, username, password))
	}
	return &KeycloakUser{
		Id:       id,
		Email:    email,
		Username: username,
		Password: password,
	}, nil
}
