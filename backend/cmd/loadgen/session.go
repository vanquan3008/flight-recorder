package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const accessTokenRenewal = 10 * time.Minute

// session holds the credentials one run uses: a console access token for
// reads and an ingest token for OTLP exports into a fresh project.
type session struct {
	client      *http.Client
	settings    options
	projectID   string
	ingestToken string

	mutex       sync.Mutex
	accessToken string
	issuedAt    time.Time
}

func newSession(ctx context.Context, client *http.Client, settings options) (*session, error) {
	current := &session{client: client, settings: settings}
	if err := current.login(ctx); err != nil {
		return nil, err
	}
	var project struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	slug := "loadgen-" + settings.runLabel
	body := map[string]string{"slug": slug, "name": "Load " + settings.runLabel, "environment": settings.environment}
	if err := current.call(ctx, http.MethodPost, "/api/v1/projects", "", body, &project); err != nil {
		return nil, fmt.Errorf("create project %s: %w", slug, err)
	}
	current.projectID = project.Project.ID
	var token struct {
		JWT string `json:"jwt"`
	}
	path := "/api/v1/projects/" + current.projectID + "/ingest-tokens"
	if err := current.call(ctx, http.MethodPost, path, current.projectID, map[string]string{"name": "loadgen-" + settings.runLabel}, &token); err != nil {
		return nil, fmt.Errorf("create ingest token: %w", err)
	}
	if current.projectID == "" || token.JWT == "" {
		return nil, fmt.Errorf("setup returned an empty project ID or ingest token")
	}
	current.ingestToken = token.JWT
	return current, nil
}

func (current *session) login(ctx context.Context) error {
	payload, _ := json.Marshal(map[string]string{"email": current.settings.email, "password": current.settings.password})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, current.settings.apiURL+"/api/v1/auth/login", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-TracePilot-Refresh", "1")
	var signedIn struct {
		AccessToken string `json:"access_token"`
	}
	if err := current.do(request, &signedIn); err != nil {
		return fmt.Errorf("sign in as %s: %w", current.settings.email, err)
	}
	current.mutex.Lock()
	current.accessToken, current.issuedAt = signedIn.AccessToken, time.Now()
	current.mutex.Unlock()
	return nil
}

// bearer returns a console access token, signing in again before the
// configured 15 minute lifetime runs out on long runs.
func (current *session) bearer(ctx context.Context) string {
	current.mutex.Lock()
	expired := time.Since(current.issuedAt) > accessTokenRenewal
	token := current.accessToken
	current.mutex.Unlock()
	if expired && current.login(ctx) == nil {
		current.mutex.Lock()
		token = current.accessToken
		current.mutex.Unlock()
	}
	return token
}

func (current *session) scopeHeaders(ctx context.Context, header http.Header) {
	header.Set("Authorization", "Bearer "+current.bearer(ctx))
	header.Set("X-Project-ID", current.projectID)
	header.Set("X-Environment", current.settings.environment)
}

func (current *session) call(ctx context.Context, method, path, projectID string, body, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, current.settings.apiURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+current.bearer(ctx))
	if projectID != "" {
		request.Header.Set("X-Project-ID", projectID)
	}
	return current.do(request, result)
}

func (current *session) do(request *http.Request, result any) error {
	response, err := current.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", request.Method, request.URL.Path, response.StatusCode, bytes.TrimSpace(payload))
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(payload, result)
}
