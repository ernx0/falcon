package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(base, token string) *Client {
	return &Client{
		BaseURL: base,
		Token:   token,
		HTTP: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-Worker-Token", c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api %s: %d %s", path, resp.StatusCode, string(b))
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) RunStart(ctx context.Context, runID int64) error {
	return c.post(ctx, fmt.Sprintf("/internal/runs/%d/start", runID), nil, nil)
}

type StepReq struct {
	Tool         string          `json:"tool"`
	Status       string          `json:"status"`
	ArtifactPath string          `json:"artifact_path"`
	Stats        json.RawMessage `json:"stats"`
	Error        string          `json:"error"`
}

func (c *Client) RunStep(ctx context.Context, runID int64, s StepReq) error {
	return c.post(ctx, fmt.Sprintf("/internal/runs/%d/steps", runID), s, nil)
}

type FinishReq struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

func (c *Client) RunFinish(ctx context.Context, runID int64, status, errMsg string) error {
	return c.post(ctx, fmt.Sprintf("/internal/runs/%d/finish", runID), FinishReq{Status: status, Error: errMsg}, nil)
}

type AssetItem struct {
	Kind   string          `json:"kind"`
	Value  string          `json:"value,omitempty"`
	Host   string          `json:"host,omitempty"`
	IP     string          `json:"ip,omitempty"`
	Port   int             `json:"port,omitempty"`
	Proto  string          `json:"proto,omitempty"`
	Scheme string          `json:"scheme,omitempty"`
	Status int             `json:"status,omitempty"`
	Title  string          `json:"title,omitempty"`
	Tech   []string        `json:"tech,omitempty"`
	URL    string          `json:"url,omitempty"`
	Method string          `json:"method,omitempty"`
	Source string          `json:"source,omitempty"`
	Meta   json.RawMessage `json:"meta,omitempty"`
}

type BulkAssetsReq struct {
	ScopeID *int64      `json:"scope_id"`
	Items    []AssetItem `json:"items"`
}

type BulkAssetsResp struct {
	Hosts     int `json:"hosts"`
	IPs       int `json:"ips"`
	Services  int `json:"services"`
	Endpoints int `json:"endpoints"`
	OOSCount  int `json:"oos_count"`
}

func (c *Client) BulkAssets(ctx context.Context, programID int64, req BulkAssetsReq) (BulkAssetsResp, error) {
	var out BulkAssetsResp
	err := c.post(ctx, fmt.Sprintf("/internal/programs/%d/assets:bulk", programID), req, &out)
	return out, err
}
