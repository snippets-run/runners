package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultURL = "https://registry.snippets.run"

type Resolution struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Type   string `json:"type"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
}

type Client struct {
	baseURL  string
	version  string
	resolve  *http.Client
	download *http.Client
}

func New(baseURL, version string) (*Client, error) {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid registry URL: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	return &Client{
		baseURL:  baseURL,
		version:  version,
		resolve:  &http.Client{Timeout: 10 * time.Second, Transport: transport},
		download: &http.Client{Timeout: 120 * time.Second, Transport: transport},
	}, nil
}

func (c *Client) Resolve(ctx context.Context, owner, repo, ref string) (Resolution, error) {
	path := "/api/resolve/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "@" + url.PathEscape(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return Resolution{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "run/"+c.version)
	response, err := c.resolve.Do(req)
	if err != nil {
		return Resolution{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Resolution{}, responseError(response)
	}
	var resolution Resolution
	if err := json.NewDecoder(response.Body).Decode(&resolution); err != nil {
		return Resolution{}, fmt.Errorf("decode resolve response: %w", err)
	}
	if resolution.Commit == "" {
		return Resolution{}, fmt.Errorf("resolve response has no commit")
	}
	if resolution.Type == "" {
		return Resolution{}, fmt.Errorf("resolve response has no snippet type")
	}
	return resolution, nil
}

func (c *Client) Download(ctx context.Context, owner, repo, commit string) (io.ReadCloser, error) {
	path := "/api/download/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "@" + url.PathEscape(commit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/gzip")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "run/"+c.version)
	response, err := c.download.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		return nil, responseError(response)
	}
	return response.Body, nil
}

func responseError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	message := strings.TrimSpace(string(body))
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error != "" {
		message = payload.Error
	}
	if message == "" {
		message = response.Status
	}
	return fmt.Errorf("registry returned %d: %s", response.StatusCode, message)
}
