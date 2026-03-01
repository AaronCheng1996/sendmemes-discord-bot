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
	mu         sync.RWMutex
	token      string // current token value
	tokenParam string // "access_token" or "auth"
	username   string
	password   string
	apiEndpoint string
	httpClient  *http.Client
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
	}
	if accessToken != "" {
		c.token = accessToken
		c.tokenParam = "access_token"
	}
	return c
}

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

	if c.username == "" || c.password == "" {
		return fmt.Errorf("PCloudClient - Login: no access_token and no username/password configured")
	}

	apiURL := fmt.Sprintf("%s/userinfo?getauth=1&logout=1&username=%s&password=%s",
		c.apiEndpoint,
		url.QueryEscape(c.username),
		url.QueryEscape(c.password),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("PCloudClient - Login - NewRequest: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PCloudClient - Login - Do: %w", err)
	}
	defer resp.Body.Close()

	var result pcloudUserInfoResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("PCloudClient - Login - Decode: %w", err)
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

// authQuery returns the correct query parameter for the current token type.
func (c *PCloudClient) authQuery() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tokenParam + "=" + url.QueryEscape(c.token)
}

// ListFolder recursively lists all image files under folderID.
// Each returned PCloudEntry carries the immediate parent folder name (album name).
// Files directly inside folderID (root-level, no album subfolder) are skipped.
func (c *PCloudClient) ListFolder(ctx context.Context, folderID int64) ([]repo.PCloudEntry, error) {
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

// GetFileLink returns a temporary download URL for a pCloud file.
func (c *PCloudClient) GetFileLink(ctx context.Context, fileID int64) (string, error) {
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
