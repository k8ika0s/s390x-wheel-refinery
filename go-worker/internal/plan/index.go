package plan

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func basicAuth(user, pass string) string {
	auth := user + ":" + pass
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// IndexClient resolves package metadata from configured indexes.
// This is a minimal client used to fetch the latest version when pins are missing.
type IndexClient struct {
	BaseURL       string
	ExtraIndexURL string
	HTTPClient    *http.Client
	Username      string
	Password      string
}

// ResolveLatest returns a best-effort latest version string for the package.
func (c *IndexClient) ResolveLatest(name string) (string, error) {
	client := c.http()
	var errs []string
	for _, base := range c.indexes() {
		ver, err := c.fetchLatest(client, base, name)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", base, err))
			continue
		}
		if ver != "" {
			return ver, nil
		}
		errs = append(errs, fmt.Sprintf("%s: empty response", base))
	}
	return "", fmt.Errorf("version not found for %s (%s)", name, strings.Join(errs, "; "))
}

func (c *IndexClient) indexes() []string {
	out := []string{}
	if c.BaseURL != "" {
		out = append(out, c.BaseURL)
	}
	if c.ExtraIndexURL != "" {
		out = append(out, c.ExtraIndexURL)
	}
	if len(out) == 0 {
		out = append(out, "https://pypi.org/simple")
	}
	return out
}

func (c *IndexClient) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *IndexClient) fetchLatest(client *http.Client, base, name string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	headers := http.Header{}
	if c.Username != "" && c.Password != "" {
		headers.Set("Authorization", "Basic "+basicAuth(c.Username, c.Password))
	}
	// Try JSON API if PyPI
	if strings.Contains(u.Host, "pypi.org") {
		// Normalize to the canonical PyPI JSON endpoint regardless of the provided path
		apiBase := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		api := fmt.Sprintf("%s/pypi/%s/json", strings.TrimRight(apiBase, "/"), name)
		req, _ := http.NewRequest(http.MethodGet, api, nil)
		req.Header = headers
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("get %s: %w", api, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("get %s: status %d", api, resp.StatusCode)
		}
		var payload struct {
			Info struct {
				Version string `json:"version"`
			} `json:"info"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return "", err
		}
		return payload.Info.Version, nil
	}
	return "", fmt.Errorf("unsupported index host %s", u.Host)
}
