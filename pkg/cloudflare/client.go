package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

// Client wraps the Cloudflare API for DNS record management.
type Client struct {
	apiToken string
	baseURL  string
	http     *http.Client
}

// NewClient creates a CF API client with the given API token.
func NewClient(apiToken string) *Client {
	return &Client{
		apiToken: apiToken,
		baseURL:  defaultBaseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	// Retry with backoff on 429.
	var resp *http.Response
	for attempt := 0; attempt < 3; attempt++ {
		resp, err = c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			break
		}
		resp.Body.Close()
		time.Sleep(time.Duration(1<<attempt) * time.Second)

		// Rebuild request for retry (body may have been consumed).
		if body != nil {
			b, _ := json.Marshal(body)
			reqBody = bytes.NewReader(b)
		}
		req, _ = http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		req.Header.Set("Content-Type", "application/json")
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiResp apiResponse[json.RawMessage]
		if json.Unmarshal(data, &apiResp) == nil && len(apiResp.Errors) > 0 {
			return nil, fmt.Errorf("cf api error %d: %s", apiResp.Errors[0].Code, apiResp.Errors[0].Message)
		}
		return nil, fmt.Errorf("cf api %s %s: status %d", method, path, resp.StatusCode)
	}

	return data, nil
}

// ListZones returns all zones accessible with the API token.
func (c *Client) ListZones(ctx context.Context) ([]Zone, error) {
	data, err := c.do(ctx, http.MethodGet, "/zones?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	var resp apiResponse[[]Zone]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// GetZone returns a single zone by ID.
func (c *Client) GetZone(ctx context.Context, zoneID string) (*Zone, error) {
	data, err := c.do(ctx, http.MethodGet, "/zones/"+zoneID, nil)
	if err != nil {
		return nil, err
	}
	var resp apiResponse[Zone]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// ListRecords returns all DNS records for a zone, handling pagination.
func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	var all []Record
	page := 1
	for {
		path := fmt.Sprintf("/zones/%s/dns_records?per_page=100&page=%d", zoneID, page)
		data, err := c.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var resp apiResponse[[]Record]
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Result...)
		if resp.ResultInfo == nil || page >= resp.ResultInfo.TotalPages {
			break
		}
		page++
	}
	return all, nil
}

// CreateRecord creates a DNS record in a zone.
func (c *Client) CreateRecord(ctx context.Context, zoneID string, rec Record) (*Record, error) {
	path := fmt.Sprintf("/zones/%s/dns_records", zoneID)
	data, err := c.do(ctx, http.MethodPost, path, rec)
	if err != nil {
		return nil, err
	}
	var resp apiResponse[Record]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// UpdateRecord updates an existing DNS record.
func (c *Client) UpdateRecord(ctx context.Context, zoneID, recordID string, rec Record) (*Record, error) {
	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID)
	data, err := c.do(ctx, http.MethodPatch, path, rec)
	if err != nil {
		return nil, err
	}
	var resp apiResponse[Record]
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// DeleteRecord deletes a DNS record.
func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID)
	_, err := c.do(ctx, http.MethodDelete, path, nil)
	return err
}
