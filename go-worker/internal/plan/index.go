package plan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IndexClient resolves package metadata from configured indexes.
// This is a minimal client used to fetch the latest version when pins are missing.
type IndexClient struct {
	BaseURL       string
	ExtraIndexURL string
	HTTPClient    *http.Client
}

// ResolveLatest returns a best-effort latest version string for the package.
func (c *IndexClient) ResolveLatest(name string) (string, error) {
	client := c.http()
	for _, base := range c.indexes() {
		ver, _ := c.fetchLatest(client, base, name)
		if ver != "" {
			return ver, nil
		}
	}
	return "", fmt.Errorf("version not found for %s", name)
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
	// Try JSON API if PyPI
	if strings.Contains(u.Host, "pypi.org") {
		api := fmt.Sprintf("%s/pypi/%s/json", strings.TrimRight(base, "/"), name)
		resp, err := client.Get(api)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
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
