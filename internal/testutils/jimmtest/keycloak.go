// Copyright 2025 Canonical.

package jimmtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
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
	keycloakAdminCLIClientID = "admin-cli"
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
		return nil, fmt.Errorf("failed to authenticate admin CLI user: %w", err)
	}

	if err := addKeycloakUser(adminCLIToken, email, username); err != nil {
		return nil, fmt.Errorf("failed to add keycloak user (%q, %q): %w", email, username, err)
	}

	id, err := getKeycloakUserId(adminCLIToken, username)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ID for newly added keycloak user (%q, %q): %w", email, username, err)
	}

	if err := setKeycloakUserPassword(adminCLIToken, id, password); err != nil {
		return nil, fmt.Errorf("failed to set password for newly added keycloak user (%q, %q, %q): %w", email, username, password, err)
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
		Path:   "/realms/master/protocol/openid-connect/token",
	}
	reqBody := url.Values{}
	reqBody.Set("client_id", keycloakAdminCLIClientID)
	reqBody.Set("username", keycloakAdminUsername)
	reqBody.Set("password", keycloakAdminPassword)
	reqBody.Set("grant_type", "password")
	resp, err := httpClient.Post(
		u.String(),
		"application/x-www-form-urlencoded",
		strings.NewReader(reqBody.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("failed to login with keycloak admin CLI user: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read keycloak response for admin CLI login (status-code: %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to login with keycloak admin CLI user (status-code: %d): %q", resp.StatusCode, string(body))
	}

	m := map[string]any{}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", fmt.Errorf("failed to parse keycloak response for admin CLI login: %q: %w", string(body), err)
	}

	if _, ok := m["access_token"]; !ok {
		return "", fmt.Errorf("cannot find access token in keycloak response: %q: %w", string(body), err)
	}
	if token, ok := m["access_token"].(string); !ok {
		return "", fmt.Errorf("received token is not string: %v: %w", m["access_token"], err)
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
		return nil, fmt.Errorf("failed to get users from keycloak: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read keycloak response for list of users (status-code: %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get users from keycloak (status-code: %d): %q", resp.StatusCode, string(body))
	}

	var raw []struct {
		Id       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse keycloak response for list of users: %q: %w", string(body), err)
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
		return "", fmt.Errorf("keycloak user not found: %q", username)
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
		return fmt.Errorf("failed to add user to keycloak: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read keycloak response to add user (status-code: %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to add user to keycloak (status-code: %d): %q", resp.StatusCode, string(body))
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
		return fmt.Errorf("failed to set keycloak user password: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read keycloak response to set user password (status-code: %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to set keycloak user password (status-code: %d): %q", resp.StatusCode, string(body))
	}
	return nil
}
