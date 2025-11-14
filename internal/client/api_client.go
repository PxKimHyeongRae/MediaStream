package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// AuthResponse represents the authentication response
type AuthResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

// CCTVInfo represents the CCTV information from the API
type CCTVInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SyncResponse represents the sync response
type SyncResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// APIClient handles communication with the external API
type APIClient struct {
	baseURL      string
	httpClient   *http.Client
	accessToken  string
	refreshToken string
	cookies      []*http.Cookie
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	jar, _ := cookiejar.New(nil)

	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}
}

// SignIn authenticates with the API and stores tokens/cookies
func (c *APIClient) SignIn(ctx context.Context, username, password string) error {
	authData := map[string]string{
		"username": username,
		"password": password,
	}

	jsonData, err := json.Marshal(authData)
	if err != nil {
		return fmt.Errorf("failed to marshal auth data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/auth/sign-in", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Store cookies for authentication
	c.cookies = resp.Cookies()

	// Try to read response body for JSON tokens
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// If body has content, try to parse as JSON
	if len(body) > 0 {
		var authResp AuthResponse
		if err := json.Unmarshal(body, &authResp); err != nil {
			// JSON parsing failed, will use cookies only
		} else {
			c.accessToken = authResp.AccessToken
			c.refreshToken = authResp.RefreshToken
		}
	}

	// Check if we have either cookies or tokens
	if len(c.cookies) == 0 && c.accessToken == "" {
		return fmt.Errorf("no authentication credentials received")
	}

	return nil
}

// SyncCCTVs triggers the CCTV sync process
func (c *APIClient) SyncCCTVs(ctx context.Context) error {
	if c.accessToken == "" && len(c.cookies) == 0 {
		return fmt.Errorf("not authenticated")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/cctvs/sync", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Use token if available, otherwise cookies will be sent automatically
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync failed with status %d: %s", resp.StatusCode, string(body))
	}

	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if syncResp.Status != "success" && syncResp.Status != "completed" {
		return fmt.Errorf("sync not completed: %s", syncResp.Message)
	}

	return nil
}

// GetCCTVs retrieves the list of CCTVs
func (c *APIClient) GetCCTVs(ctx context.Context) ([]CCTVInfo, error) {
	if c.accessToken == "" && len(c.cookies) == 0 {
		return nil, fmt.Errorf("not authenticated")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/cctvs", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use token if available, otherwise cookies will be sent automatically
	if c.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get CCTVs failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body first for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Try to parse as array first
	var cctvs []CCTVInfo
	if err := json.Unmarshal(body, &cctvs); err != nil {
		// If array parsing fails, try parsing as object with data field
		var response struct {
			Data []CCTVInfo `json:"data"`
		}
		if err2 := json.Unmarshal(body, &response); err2 != nil {
			return nil, fmt.Errorf("failed to decode response as array or object: %w, body: %s", err, string(body))
		}
		cctvs = response.Data
	}

	return cctvs, nil
}

// GetAccessToken returns the current access token
func (c *APIClient) GetAccessToken() string {
	return c.accessToken
}

// GetRefreshToken returns the current refresh token
func (c *APIClient) GetRefreshToken() string {
	return c.refreshToken
}
