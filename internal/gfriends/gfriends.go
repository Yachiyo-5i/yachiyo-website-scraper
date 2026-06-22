package gfriends

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultIndexURL = "https://raw.githubusercontent.com/gfriends/gfriends/master/Filetree.json"
	DefaultBaseURL  = "https://cdn.jsdelivr.net/gh/gfriends/gfriends@master"
	DefaultTTL      = 24 * time.Hour
)

type Options struct {
	CachePath  string
	IndexURL   string
	BaseURL    string
	TTL        time.Duration
	HTTPClient *http.Client
}

type Client struct {
	cachePath  string
	indexURL   string
	baseURL    string
	ttl        time.Duration
	httpClient *http.Client

	mu     sync.Mutex
	loaded bool
	index  map[string]string
}

func NewClient(opts Options) *Client {
	cachePath := strings.TrimSpace(opts.CachePath)
	if cachePath == "" {
		cachePath = DefaultCachePath()
	}
	indexURL := strings.TrimSpace(opts.IndexURL)
	if indexURL == "" {
		indexURL = DefaultIndexURL
	}
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	ttl := opts.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		cachePath:  cachePath,
		indexURL:   indexURL,
		baseURL:    baseURL,
		ttl:        ttl,
		httpClient: httpClient,
	}
}

func DefaultCachePath() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join("cache", "gfriends", "Filetree.json")
	}
	return filepath.Join(filepath.Dir(exe), "cache", "gfriends", "Filetree.json")
}

func (c *Client) Lookup(ctx context.Context, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}

	index, err := c.load(ctx)
	if err != nil {
		return "", false
	}
	imageURL, ok := index[name]
	return imageURL, ok
}

func (c *Client) load(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loaded {
		return c.index, nil
	}

	data, err := c.readFreshCache()
	if err != nil {
		return c.failLoad(err)
	}
	if data == nil {
		data, err = c.download(ctx)
		if err != nil {
			data, err = c.readAnyCache()
		}
		if err != nil {
			return c.failLoad(err)
		}
	}

	index, err := parseIndex(data, c.baseURL)
	if err != nil {
		return c.failLoad(err)
	}
	c.index = index
	c.loaded = true
	return index, nil
}

func (c *Client) failLoad(err error) (map[string]string, error) {
	c.index = map[string]string{}
	c.loaded = true
	return c.index, err
}

func (c *Client) readFreshCache() ([]byte, error) {
	if c.ttl < 0 {
		return nil, nil
	}
	info, err := os.Stat(c.cachePath)
	if err != nil {
		return nil, nil
	}
	if time.Since(info.ModTime()) > c.ttl {
		return nil, nil
	}
	return os.ReadFile(c.cachePath)
}

func (c *Client) readAnyCache() ([]byte, error) {
	return os.ReadFile(c.cachePath)
}

func (c *Client) download(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.indexURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download gfriends index: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0o755); err == nil {
		_ = os.WriteFile(c.cachePath, data, 0o644)
	}
	return data, nil
}

func parseIndex(data []byte, baseURL string) (map[string]string, error) {
	var tree struct {
		Content json.RawMessage `json:"Content"`
	}
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, err
	}
	index := map[string]string{}
	if len(tree.Content) == 0 || string(tree.Content) == "null" {
		return index, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(tree.Content))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("gfriends Content must be an object")
	}

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		company, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("gfriends company key must be a string")
		}
		if err := parseCompany(decoder, index, baseURL, company); err != nil {
			return nil, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return index, nil
}

func parseCompany(decoder *json.Decoder, index map[string]string, baseURL, company string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("gfriends company %q must be an object", company)
	}

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		nameFile, ok := token.(string)
		if !ok {
			return fmt.Errorf("gfriends actor key in %q must be a string", company)
		}
		var imageFile string
		if err := decoder.Decode(&imageFile); err != nil {
			return err
		}
		name := strings.TrimSuffix(nameFile, filepath.Ext(nameFile))
		if strings.TrimSpace(name) == "" || strings.TrimSpace(imageFile) == "" {
			continue
		}
		index[name] = imageURL(baseURL, company, imageFile)
	}
	_, err = decoder.Token()
	return err
}

func imageURL(baseURL, company, imageFile string) string {
	fileName, query, hasQuery := strings.Cut(imageFile, "?")
	escapedPath := path.Join("Content", url.PathEscape(company), url.PathEscape(fileName))
	imageURL := strings.TrimRight(baseURL, "/") + "/" + escapedPath
	if hasQuery && query != "" {
		imageURL += "?" + query
	}
	return imageURL
}
