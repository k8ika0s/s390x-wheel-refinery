package reporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client posts artifacts to control-plane.
type Client struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (c *Client) post(path string, payload any) error {
	if c == nil || c.BaseURL == "" {
		return nil
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("X-Worker-Token", c.Token)
	}
	cli := c.Client
	if cli == nil {
		cli = http.DefaultClient
	}
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("post %s status %s", path, resp.Status)
	}
	return nil
}

func (c *Client) PostManifest(entries any) error { return c.post("/api/manifest", entries) }
func (c *Client) PostPlan(plan any) error       { return c.post("/api/plan", plan) }
func (c *Client) PostLog(log any) error         { return c.post("/api/logs", log) }
