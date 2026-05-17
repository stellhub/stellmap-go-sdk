package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	routeRegistryRegister   = "/api/v1/registry/register"
	routeRegistryDeregister = "/api/v1/registry/deregister"
	routeRegistryHeartbeat  = "/api/v1/registry/heartbeat"
	routeRegistryInstances  = "/api/v1/registry/instances"
	routeRegistryWatch      = "/api/v1/registry/watch"
)

// Client 表示 StellMap Go SDK 客户端。
type Client struct {
	baseURL                *url.URL
	httpClient             *http.Client
	userAgent              string
	retryNotLeader         bool
	defaultHeaders         map[string]string
	watchReconnectInterval time.Duration
}

// RawRequest 表示一次底层 HTTP 请求。
type RawRequest struct {
	Method         string
	Path           string
	Query          url.Values
	Body           any
	Header         map[string]string
	BaseURL        *url.URL
	RetryNotLeader bool
}

// ClientOption 表示客户端可选项。
type ClientOption func(*Client)

// WithHTTPClient 自定义底层 HTTP Client。
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(client *Client) {
		if httpClient != nil {
			client.httpClient = httpClient
		}
	}
}

// WithUserAgent 设置 User-Agent。
func WithUserAgent(userAgent string) ClientOption {
	return func(client *Client) {
		client.userAgent = strings.TrimSpace(userAgent)
	}
}

// WithRetryNotLeader 设置是否自动跟随 leader 重试。
func WithRetryNotLeader(enabled bool) ClientOption {
	return func(client *Client) {
		client.retryNotLeader = enabled
	}
}

// WithDefaultHeader 设置默认请求头。
func WithDefaultHeader(key, value string) ClientOption {
	return func(client *Client) {
		if client.defaultHeaders == nil {
			client.defaultHeaders = make(map[string]string)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return
		}
		client.defaultHeaders[key] = value
	}
}

// WithWatchReconnectInterval 设置 watch 自动重连间隔。
func WithWatchReconnectInterval(interval time.Duration) ClientOption {
	return func(client *Client) {
		if interval > 0 {
			client.watchReconnectInterval = interval
		}
	}
}

// NewClient 创建 StellMap SDK 客户端。
func NewClient(baseURL string, options ...ClientOption) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse baseURL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("baseURL must include scheme and host")
	}

	client := &Client{
		baseURL:                trimTrailingSlashURL(parsed),
		httpClient:             &http.Client{Timeout: 10 * time.Second},
		userAgent:              "stellmap-go-sdk",
		retryNotLeader:         true,
		defaultHeaders:         make(map[string]string),
		watchReconnectInterval: time.Second,
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}

	return client, nil
}

// Register 注册实例。
func (c *Client) Register(ctx context.Context, request RegisterRequest) error {
	request, err := normalizeRegisterRequest(request)
	if err != nil {
		return err
	}

	_, err = c.doEnvelope(ctx, http.MethodPost, routeRegistryRegister, nil, request, nil, true)
	return err
}

// Deregister 注销实例。
func (c *Client) Deregister(ctx context.Context, request DeregisterRequest) error {
	request, err := normalizeDeregisterRequest(request)
	if err != nil {
		return err
	}

	_, err = c.doEnvelope(ctx, http.MethodPost, routeRegistryDeregister, nil, request, nil, true)
	return err
}

// Heartbeat 发送实例心跳。
func (c *Client) Heartbeat(ctx context.Context, request HeartbeatRequest) error {
	request, err := normalizeHeartbeatRequest(request)
	if err != nil {
		return err
	}

	_, err = c.doEnvelope(ctx, http.MethodPost, routeRegistryHeartbeat, nil, request, nil, true)
	return err
}

// QueryInstances 查询实例列表。
func (c *Client) QueryInstances(ctx context.Context, options QueryOptions) ([]Instance, error) {
	values, err := buildQueryValues(options)
	if err != nil {
		return nil, err
	}

	var items []Instance
	_, err = c.doEnvelope(ctx, http.MethodGet, routeRegistryInstances, values, nil, &items, false)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// WatchInstances 建立带自动重连能力的 watch 流。
func (c *Client) WatchInstances(ctx context.Context, options WatchOptions) *Watcher {
	return newWatcher(c, ctx, options)
}

// DoRaw 发起底层 HTTP 请求，供上层子客户端复用。
func (c *Client) DoRaw(ctx context.Context, request RawRequest) (*http.Response, error) {
	return c.doRaw(ctx, requestSpec{
		Method:  request.Method,
		Path:    request.Path,
		Query:   request.Query,
		Body:    request.Body,
		Header:  request.Header,
		BaseURL: request.BaseURL,
	}, request.RetryNotLeader)
}

type requestSpec struct {
	Method  string
	Path    string
	Query   url.Values
	Body    any
	Header  map[string]string
	BaseURL *url.URL
}

func (c *Client) doEnvelope(ctx context.Context, method, path string, query url.Values, requestBody any, dataTarget any, retryNotLeader bool) (successEnvelopeRaw, error) {
	response, err := c.doRaw(ctx, requestSpec{
		Method: method,
		Path:   path,
		Query:  query,
		Body:   requestBody,
	}, retryNotLeader)
	if err != nil {
		return successEnvelopeRaw{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Code      string          `json:"code"`
		Message   string          `json:"message,omitempty"`
		Data      json.RawMessage `json:"data,omitempty"`
		RequestID string          `json:"requestId,omitempty"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return successEnvelopeRaw{}, fmt.Errorf("decode response envelope: %w", err)
	}

	if dataTarget != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, dataTarget); err != nil {
			return successEnvelopeRaw{}, fmt.Errorf("decode response data: %w", err)
		}
	}

	return successEnvelopeRaw{
		Code:      envelope.Code,
		Message:   envelope.Message,
		Data:      envelope.Data,
		RequestID: envelope.RequestID,
	}, nil
}

func (c *Client) doRaw(ctx context.Context, spec requestSpec, retryNotLeader bool) (*http.Response, error) {
	return c.doRawWithHTTPClient(ctx, spec, retryNotLeader, c.httpClient)
}

func (c *Client) doRawWithHTTPClient(ctx context.Context, spec requestSpec, retryNotLeader bool, httpClient *http.Client) (*http.Response, error) {
	response, err := c.doRawOnceWithHTTPClient(ctx, spec, httpClient)
	if err == nil {
		return response, nil
	}

	var apiErr *APIError
	if !retryNotLeader || !c.retryNotLeader || !errors.As(err, &apiErr) || !apiErr.IsNotLeader() || strings.TrimSpace(apiErr.LeaderAddr) == "" {
		return nil, err
	}

	leaderBaseURL, buildErr := c.leaderBaseURL(apiErr.LeaderAddr, spec.BaseURL)
	if buildErr != nil {
		return nil, buildErr
	}
	spec.BaseURL = leaderBaseURL
	return c.doRawOnceWithHTTPClient(ctx, spec, httpClient)
}

func (c *Client) doRawOnce(ctx context.Context, spec requestSpec) (*http.Response, error) {
	return c.doRawOnceWithHTTPClient(ctx, spec, c.httpClient)
}

func (c *Client) doRawOnceWithHTTPClient(ctx context.Context, spec requestSpec, httpClient *http.Client) (*http.Response, error) {
	request, err := c.newRequest(ctx, spec)
	if err != nil {
		return nil, err
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}

	defer response.Body.Close()
	return nil, c.decodeAPIError(response)
}

func (c *Client) watchHTTPClient() *http.Client {
	if c.httpClient == nil {
		return http.DefaultClient
	}

	watchClient := *c.httpClient
	watchClient.Timeout = 0
	return &watchClient
}

func (c *Client) newRequest(ctx context.Context, spec requestSpec) (*http.Request, error) {
	base := c.baseURL
	if spec.BaseURL != nil {
		base = trimTrailingSlashURL(spec.BaseURL)
	}

	target := *base
	target.Path = joinURLPath(base.Path, spec.Path)
	if len(spec.Query) > 0 {
		target.RawQuery = spec.Query.Encode()
	}

	var bodyReader io.Reader
	if spec.Body != nil {
		payload, err := json.Marshal(spec.Body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, spec.Method, target.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	if spec.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(c.userAgent) != "" {
		request.Header.Set("User-Agent", c.userAgent)
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range c.defaultHeaders {
		request.Header.Set(key, value)
	}
	for key, value := range spec.Header {
		request.Header.Set(key, value)
	}
	return request, nil
}

func (c *Client) decodeAPIError(response *http.Response) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read error response body: %w", err)
	}

	var payload apiErrorResponse
	if len(body) > 0 {
		_ = json.Unmarshal(body, &payload)
	}
	if payload.Code == "" && strings.TrimSpace(string(body)) != "" {
		payload.Message = strings.TrimSpace(string(body))
	}

	return &APIError{
		HTTPStatus: response.StatusCode,
		Code:       payload.Code,
		Message:    payload.Message,
		RequestID:  payload.RequestID,
		LeaderID:   payload.LeaderID,
		LeaderAddr: payload.LeaderAddr,
	}
}

func (c *Client) leaderBaseURL(leaderAddr string, current *url.URL) (*url.URL, error) {
	leaderAddr = strings.TrimSpace(leaderAddr)
	if leaderAddr == "" {
		return nil, fmt.Errorf("leaderAddr is empty")
	}

	scheme := c.baseURL.Scheme
	if current != nil && current.Scheme != "" {
		scheme = current.Scheme
	}
	parsed, err := url.Parse(fmt.Sprintf("%s://%s", scheme, leaderAddr))
	if err != nil {
		return nil, fmt.Errorf("build leader baseURL: %w", err)
	}
	return parsed, nil
}

func trimTrailingSlashURL(value *url.URL) *url.URL {
	cloned := *value
	if cloned.Path == "/" {
		cloned.Path = ""
	}
	cloned.Path = strings.TrimRight(cloned.Path, "/")
	return &cloned
}

func joinURLPath(basePath, requestPath string) string {
	basePath = strings.TrimRight(basePath, "/")
	requestPath = "/" + strings.TrimLeft(requestPath, "/")
	if basePath == "" {
		return requestPath
	}
	return basePath + requestPath
}
