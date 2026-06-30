package komari

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	rpcURL     string
	publicURL  string
	apiKey     string
	httpClient *http.Client
}

type Options struct {
	APIKey  string
	Timeout time.Duration
}

func NewClient(rawURL string, timeout time.Duration) (*Client, error) {
	return NewClientWithOptions(rawURL, Options{Timeout: timeout})
}

func NewClientWithOptions(rawURL string, opts Options) (*Client, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("empty Komari URL")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Komari URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	baseURL := parsed.String()

	return &Client{
		baseURL:    baseURL,
		rpcURL:     baseURL + "/api/rpc2",
		publicURL:  baseURL + "/api/public",
		apiKey:     strings.TrimSpace(opts.APIKey),
		httpClient: &http.Client{Timeout: opts.Timeout},
	}, nil
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) HasAPIKey() bool {
	return c.apiKey != ""
}

func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	public, publicErr := c.PublicInfo(ctx)
	nodes, nodesErr := c.Nodes(ctx)
	if nodesErr != nil {
		return Snapshot{}, nodesErr
	}
	var nodeDetailErr error
	if c.HasAPIKey() {
		detailedNodes, err := c.AdminClients(ctx)
		if err == nil {
			mergeNodeDetails(nodes, detailedNodes)
		} else {
			nodeDetailErr = err
		}
	}
	status, statusErr := c.LatestStatus(ctx)
	if statusErr != nil {
		return Snapshot{}, statusErr
	}
	snapshot := NewSnapshot(c.baseURL, public, nodes, status)
	if publicErr != nil {
		snapshot.LastErr = publicErr
	}
	if nodeDetailErr != nil {
		snapshot.NodeDetailErr = nodeDetailErr
	}
	if version, err := c.Version(ctx); err == nil {
		snapshot.Version = version
	}
	if rpcVersion, err := c.RPCVersion(ctx); err == nil {
		snapshot.RPCVersion = rpcVersion
	}
	if me, err := c.Me(ctx); err == nil {
		snapshot.Me = me
	}
	if methods, err := c.Methods(ctx, false); err == nil {
		snapshot.Methods = methods
	}
	return snapshot, nil
}

func (c *Client) PublicInfo(ctx context.Context) (PublicInfo, error) {
	var public PublicInfo
	if err := c.call(ctx, "common:getPublicInfo", map[string]any{}, &public); err == nil {
		return public, nil
	}

	var out struct {
		Status  string     `json:"status"`
		Message string     `json:"message"`
		Data    PublicInfo `json:"data"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.publicURL, nil)
	if err != nil {
		return PublicInfo{}, err
	}
	c.applyAuth(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PublicInfo{}, fmt.Errorf("GET /api/public: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return PublicInfo{}, fmt.Errorf("GET /api/public: HTTP %d", resp.StatusCode)
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return PublicInfo{}, fmt.Errorf("decode /api/public: %w", err)
	}
	if out.Status != "" && out.Status != "success" {
		return PublicInfo{}, fmt.Errorf("GET /api/public: %s", out.Message)
	}
	return out.Data, nil
}

func (c *Client) Version(ctx context.Context) (VersionInfo, error) {
	var version VersionInfo
	if err := c.call(ctx, "common:getVersion", map[string]any{}, &version); err != nil {
		return VersionInfo{}, err
	}
	return version, nil
}

func (c *Client) Me(ctx context.Context) (MeInfo, error) {
	var me MeInfo
	if err := c.call(ctx, "common:getMe", map[string]any{}, &me); err != nil {
		return MeInfo{}, err
	}
	return me, nil
}

func (c *Client) RPCVersion(ctx context.Context) (string, error) {
	var version string
	if err := c.call(ctx, "rpc.version", nil, &version); err != nil {
		return "", err
	}
	return version, nil
}

func (c *Client) Methods(ctx context.Context, internal bool) ([]string, error) {
	var methods []string
	if err := c.call(ctx, "rpc.methods", map[string]any{"internal": internal}, &methods); err != nil {
		return nil, err
	}
	return methods, nil
}

func (c *Client) Nodes(ctx context.Context) (map[string]Node, error) {
	var nodes NodeMap
	if err := c.call(ctx, "common:getNodes", map[string]any{}, &nodes); err != nil {
		return nil, err
	}
	return map[string]Node(nodes), nil
}

func (c *Client) AdminClients(ctx context.Context) (map[string]Node, error) {
	var nodes NodeMap
	if err := c.call(ctx, "admin:listClients", map[string]any{}, &nodes); err != nil {
		return nil, err
	}
	return map[string]Node(nodes), nil
}

func (c *Client) LatestStatus(ctx context.Context) (map[string]Status, error) {
	var status map[string]Status
	if err := c.call(ctx, "common:getNodesLatestStatus", map[string]any{}, &status); err != nil {
		return nil, err
	}
	for key, st := range status {
		if st.NetTotalUp == 0 && st.NetTotalUpAlias > 0 {
			st.NetTotalUp = st.NetTotalUpAlias
		}
		if st.NetTotalDown == 0 && st.NetDownAlias > 0 {
			st.NetTotalDown = st.NetDownAlias
		}
		status[key] = st
	}
	return status, nil
}

func (c *Client) RecentStatus(ctx context.Context, uuid string) (RecentStatusResp, error) {
	var resp RecentStatusResp
	if err := c.call(ctx, "common:getNodeRecentStatus", map[string]any{"uuid": uuid}, &resp); err != nil {
		return RecentStatusResp{}, err
	}
	return resp, nil
}

func (c *Client) LoadRecords(ctx context.Context, uuid string, hours int, loadType string, maxCount int) (LoadRecordsResp, error) {
	if hours <= 0 {
		hours = 1
	}
	if loadType == "" {
		loadType = "all"
	}
	params := map[string]any{
		"type":      "load",
		"uuid":      uuid,
		"hours":     hours,
		"load_type": loadType,
	}
	if maxCount != 0 {
		params["maxCount"] = maxCount
	}
	var resp LoadRecordsResp
	if err := c.call(ctx, "common:getRecords", params, &resp); err != nil {
		return LoadRecordsResp{}, err
	}
	return resp, nil
}

func (c *Client) PingRecords(ctx context.Context, uuid string, hours int, taskID int, maxCount int) (PingRecordsResp, error) {
	if hours <= 0 {
		hours = 1
	}
	params := map[string]any{
		"type":  "ping",
		"uuid":  uuid,
		"hours": hours,
	}
	if taskID != 0 {
		params["task_id"] = taskID
	}
	if maxCount != 0 {
		params["maxCount"] = maxCount
	}
	var resp PingRecordsResp
	if err := c.call(ctx, "common:getRecords", params, &resp); err != nil {
		return PingRecordsResp{}, err
	}
	return resp, nil
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	payload := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s: HTTP %d", method, resp.StatusCode)
	}

	var rpcResp rpcResponse
	if err := decodeJSON(resp.Body, &rpcResp); err != nil {
		return fmt.Errorf("%s: decode response: %w", method, err)
	}
	if rpcResp.Error != nil {
		return fmt.Errorf("%s: RPC %d: %s", method, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	if len(rpcResp.Result) == 0 {
		return fmt.Errorf("%s: empty result", method)
	}
	if err := json.Unmarshal(rpcResp.Result, result); err != nil {
		return fmt.Errorf("%s: decode result: %w", method, err)
	}
	return nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func decodeJSON(r io.Reader, out any) error {
	decoder := json.NewDecoder(r)
	return decoder.Decode(out)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
