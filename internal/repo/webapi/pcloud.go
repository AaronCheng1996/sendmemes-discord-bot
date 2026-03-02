// Package webapi implements external API clients.
package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

const (
	// pcloudMaxConcurrent caps how many pCloud API calls run in parallel.
	// pCloud will close connections with EOF when too many arrive at once.
	pcloudMaxConcurrent = 3

	// pcloudMaxRetries is the maximum number of attempts for each API call
	// (including the first).  Retries apply only to transient errors (EOF,
	// auth codes that warrant a re-login).
	pcloudMaxRetries = 3

	// pcloudRetryBase is the initial backoff duration; it doubles each retry.
	pcloudRetryBase = 500 * time.Millisecond
)

// isTokenErr returns true when err signals that the pCloud session token
// is no longer valid and a re-login should be attempted.
//
// pCloud closes the TCP connection with EOF when a session token expires
// (instead of returning a JSON error body), so EOF is the primary signal.
// We also catch the JSON auth-error codes for completeness.
func isTokenErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "EOF") ||
		strings.Contains(s, "API error 1000") || // Log in required
		strings.Contains(s, "API error 2000") || // Log in failed
		strings.Contains(s, "API error 2001") || // Invalid auth token
		strings.Contains(s, "API error 2094") // Invalid access_token
}

// imageExtensions is the set of file extensions treated as images.
var imageExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {}, ".webp": {},
}

// pcloudMeta mirrors the JSON structure returned by pCloud's listfolder API.
type pcloudMeta struct {
	Name     string       `json:"name"`
	IsFolder bool         `json:"isfolder"`
	FileID   int64        `json:"fileid"`
	Contents []pcloudMeta `json:"contents"`
}

type pcloudListFolderResponse struct {
	Result   int        `json:"result"`
	Error    string     `json:"error"`
	Metadata pcloudMeta `json:"metadata"`
}

type pcloudFileLinkResponse struct {
	Result int      `json:"result"`
	Error  string   `json:"error"`
	Hosts  []string `json:"hosts"`
	Path   string   `json:"path"`
}

type pcloudUserInfoResponse struct {
	Result int    `json:"result"`
	Error  string `json:"error"`
	Auth   string `json:"auth"`
}

// PCloudClient calls the pCloud REST API.
// Auth priority: AccessToken field > Login(username, password).
//
// pCloud uses two different query parameter names depending on how the token was obtained:
//   - OAuth2 token  →  access_token=TOKEN
//   - Session token (from username/password login)  →  auth=TOKEN
type PCloudClient struct {
	mu          sync.RWMutex
	token       string // current token value
	tokenParam  string // "access_token" or "auth"
	username    string
	password    string
	apiEndpoint string
	httpClient  *http.Client

	// sem limits the number of concurrent pCloud API calls to avoid
	// triggering connection-level rate limiting (EOF responses).
	sem chan struct{}

	// loginMu serialises re-login so that when many goroutines detect a
	// token error at the same time only one issues the userinfo request.
	loginMu sync.Mutex
}

// NewPCloudClient creates a new pCloud API client.
// If accessToken is non-empty it is used directly as an OAuth token (access_token param).
// Otherwise call Login(ctx) once at startup to exchange username/password for a session token (auth param).
func NewPCloudClient(accessToken, username, password, apiEndpoint string) *PCloudClient {
	c := &PCloudClient{
		username:    username,
		password:    password,
		apiEndpoint: strings.TrimSuffix(apiEndpoint, "/"),
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		sem:         make(chan struct{}, pcloudMaxConcurrent),
	}
	if accessToken != "" {
		c.token = accessToken
		c.tokenParam = "access_token"
	}
	return c
}

// acquire blocks until a slot in the concurrency semaphore is available or
// ctx is cancelled.
func (c *PCloudClient) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *PCloudClient) release() { <-c.sem }

// Login authenticates with username/password and caches the returned session token.
// If a token is already set (from NewPCloudClient) this is a no-op.
// Session tokens are passed to pCloud APIs as the "auth" query parameter.
func (c *PCloudClient) Login(ctx context.Context) error {
	c.mu.RLock()
	hasToken := c.token != ""
	c.mu.RUnlock()
	if hasToken {
		return nil
	}
	return c.doLogin(ctx)
}

// doLogin performs the actual userinfo API call (no token check).
// Retries up to pcloudMaxRetries times on transient errors.
func (c *PCloudClient) doLogin(ctx context.Context) error {
	if c.username == "" || c.password == "" {
		return fmt.Errorf("PCloudClient - Login: no access_token and no username/password configured")
	}

	apiURL := fmt.Sprintf("%s/userinfo?getauth=1&logout=1&username=%s&password=%s",
		c.apiEndpoint,
		url.QueryEscape(c.username),
		url.QueryEscape(c.password),
	)

	var lastErr error
	backoff := pcloudRetryBase
	for attempt := 1; attempt <= pcloudMaxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return fmt.Errorf("PCloudClient - Login - NewRequest: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("PCloudClient - Login - Do (attempt %d): %w", attempt, err)
			continue
		}

		var result pcloudUserInfoResponse
		decErr := json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if decErr != nil {
			lastErr = fmt.Errorf("PCloudClient - Login - Decode (attempt %d): %w", attempt, decErr)
			continue
		}
		if result.Result != 0 {
			return fmt.Errorf("PCloudClient - Login - API error %d: %s", result.Result, result.Error)
		}
		if result.Auth == "" {
			return fmt.Errorf("PCloudClient - Login - empty auth token in response")
		}

		c.mu.Lock()
		c.token = result.Auth
		c.tokenParam = "auth" // session tokens use "auth" param, NOT "access_token"
		c.mu.Unlock()
		return nil
	}

	return fmt.Errorf("PCloudClient - Login - all %d attempts failed: %w", pcloudMaxRetries, lastErr)
}

// relogin forces a fresh username/password authentication under the loginMu
// so that concurrent goroutines don't all issue userinfo requests at once.
// If another goroutine already re-logged in while this one was waiting, the
// cached token is used directly without making another API call.
func (c *PCloudClient) relogin(ctx context.Context) error {
	if c.username == "" {
		return fmt.Errorf("PCloudClient - relogin: no username/password configured for token refresh")
	}

	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	// If another goroutine finished re-login while we waited, reuse its token.
	c.mu.RLock()
	hasToken := c.token != ""
	c.mu.RUnlock()
	if hasToken {
		return nil
	}

	return c.doLogin(ctx)
}

// authQuery returns the correct query parameter for the current token type.
func (c *PCloudClient) authQuery() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tokenParam + "=" + url.QueryEscape(c.token)
}

// ---------------------------------------------------------------------------
// ListFolder
// ---------------------------------------------------------------------------

// ListFolder recursively lists all image files under folderID.
// Each returned PCloudEntry carries the immediate parent folder name (album name).
// Files directly inside folderID (root-level, no album subfolder) are skipped.
// Retries up to pcloudMaxRetries times; re-logs in if the token has expired.
func (c *PCloudClient) ListFolder(ctx context.Context, folderID int64) ([]repo.PCloudEntry, error) {
	var lastErr error
	backoff := pcloudRetryBase
	needRelogin := false

	for attempt := 1; attempt <= pcloudMaxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			backoff *= 2

			if needRelogin && c.username != "" {
				if rerr := c.relogin(ctx); rerr != nil {
					return nil, fmt.Errorf("PCloudClient - ListFolder - relogin: %w", rerr)
				}
				needRelogin = false
			}
		}

		entries, err := c.doListFolder(ctx, folderID)
		if err == nil {
			return entries, nil
		}
		lastErr = err
		if isTokenErr(err) {
			needRelogin = true
			// Clear cached token so relogin proceeds.
			c.mu.Lock()
			c.token = ""
			c.tokenParam = ""
			c.mu.Unlock()
			continue
		}
		return nil, err // non-retriable
	}
	return nil, fmt.Errorf("PCloudClient - ListFolder - all %d attempts failed: %w", pcloudMaxRetries, lastErr)
}

func (c *PCloudClient) doListFolder(ctx context.Context, folderID int64) ([]repo.PCloudEntry, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	apiURL := fmt.Sprintf("%s/listfolder?folderid=%d&recursive=1&%s",
		c.apiEndpoint, folderID, c.authQuery())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("PCloudClient - ListFolder - NewRequest: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PCloudClient - ListFolder - Do: %w", err)
	}
	defer resp.Body.Close()

	var result pcloudListFolderResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("PCloudClient - ListFolder - Decode: %w", err)
	}
	if result.Result != 0 {
		return nil, fmt.Errorf("PCloudClient - ListFolder - API error %d: %s", result.Result, result.Error)
	}

	var entries []repo.PCloudEntry
	for _, child := range result.Metadata.Contents {
		if !child.IsFolder {
			continue
		}
		collectImages(child, child.Name, &entries)
	}
	return entries, nil
}

// collectImages recursively walks a pCloud folder tree node.
// albumName is always the leaf folder name containing the image file.
func collectImages(node pcloudMeta, albumName string, out *[]repo.PCloudEntry) {
	for _, child := range node.Contents {
		if child.IsFolder {
			collectImages(child, child.Name, out)
			continue
		}
		ext := strings.ToLower(filepath.Ext(child.Name))
		if _, ok := imageExtensions[ext]; !ok {
			continue
		}
		*out = append(*out, repo.PCloudEntry{
			FileID:           child.FileID,
			Name:             child.Name,
			ParentFolderName: albumName,
		})
	}
}

// ---------------------------------------------------------------------------
// GetFileLink
// ---------------------------------------------------------------------------

// GetFileLink returns a temporary download URL for a pCloud file.
// Retries up to pcloudMaxRetries times; re-logs in if the token has expired.
func (c *PCloudClient) GetFileLink(ctx context.Context, fileID int64) (string, error) {
	var lastErr error
	backoff := pcloudRetryBase
	needRelogin := false

	for attempt := 1; attempt <= pcloudMaxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			backoff *= 2

			if needRelogin && c.username != "" {
				if rerr := c.relogin(ctx); rerr != nil {
					return "", fmt.Errorf("PCloudClient - GetFileLink - relogin: %w", rerr)
				}
				needRelogin = false
			}
		}

		link, err := c.doGetFileLink(ctx, fileID)
		if err == nil {
			return link, nil
		}
		lastErr = err
		if isTokenErr(err) {
			needRelogin = true
			// Clear cached token so relogin proceeds.
			c.mu.Lock()
			c.token = ""
			c.tokenParam = ""
			c.mu.Unlock()
			continue
		}
		return "", err // non-retriable
	}
	return "", fmt.Errorf("PCloudClient - GetFileLink - all %d attempts failed: %w", pcloudMaxRetries, lastErr)
}

func (c *PCloudClient) doGetFileLink(ctx context.Context, fileID int64) (string, error) {
	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	apiURL := fmt.Sprintf("%s/getfilelink?fileid=%d&forcedownload=0&%s",
		c.apiEndpoint, fileID, c.authQuery())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("PCloudClient - GetFileLink - NewRequest: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("PCloudClient - GetFileLink - Do: %w", err)
	}
	defer resp.Body.Close()

	var result pcloudFileLinkResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("PCloudClient - GetFileLink - Decode: %w", err)
	}
	if result.Result != 0 {
		return "", fmt.Errorf("PCloudClient - GetFileLink - API error %d: %s", result.Result, result.Error)
	}
	if len(result.Hosts) == 0 {
		return "", fmt.Errorf("PCloudClient - GetFileLink - no hosts returned")
	}
	return "https://" + result.Hosts[0] + result.Path, nil
}
