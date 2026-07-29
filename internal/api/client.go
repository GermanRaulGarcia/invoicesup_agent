// Package api is the HTTP client for the InvoicesUp connector endpoints.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Batch is one business's pending export, from GET /connector/pending.
type Batch struct {
	BusinessCode string `json:"business_code"`
	Filename     string `json:"filename"`
	Content      string `json:"content"`
	BatchToken   string `json:"batch_token"`
}

// Client talks to the connector API with a bearer connector token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client. baseURL may include or omit a trailing slash.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Pending returns the office's currently pending batches (one per business).
func (c *Client) Pending(ctx context.Context) ([]Batch, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/connector/pending", nil)
	if err != nil {
		return nil, err
	}
	c.auth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, apiError(resp)
	}

	var body struct {
		Data []Batch `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Data, nil
}

// Confirm records the batch as delivered and returns how many invoices were
// newly marked (0 on an idempotent replay).
func (c *Client) Confirm(ctx context.Context, batchToken string) (int, error) {
	payload, _ := json.Marshal(map[string]string{"batch_token": batchToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/connector/confirm", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	c.auth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, apiError(resp)
	}

	// Accept any 2xx; a body-less success (e.g. 204) counts as 0 newly delivered.
	var body struct {
		Delivered int `json:"delivered"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil && err != io.EOF {
		return 0, err
	}
	return body.Delivered, nil
}

func (c *Client) auth(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
}

func apiError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("connector API %s: %d %s", resp.Request.URL.Path, resp.StatusCode, strings.TrimSpace(string(b)))
}
