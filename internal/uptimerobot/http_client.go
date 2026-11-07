/*
Copyright 2026 DarkCityLab.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package uptimerobot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// DefaultBaseURL is the UptimeRobot v3 API root.
const DefaultBaseURL = "https://api.uptimerobot.com/v3"

// TokenFunc returns the current bearer token. It is called before every request
// so the token can be sourced from a rotating mounted Secret file.
type TokenFunc func(ctx context.Context) (string, error)

// StaticToken returns a TokenFunc that always yields the same token (tests/dev).
func StaticToken(token string) TokenFunc {
	return func(context.Context) (string, error) { return token, nil }
}

// Options configures an HTTPClient.
type Options struct {
	// BaseURL overrides DefaultBaseURL (useful for tests against httptest).
	BaseURL string
	// Token supplies the bearer token (required).
	Token TokenFunc
	// HTTPClient overrides the default *http.Client.
	HTTPClient *http.Client
	// RequestsPerMinute caps the client-side request rate. 0 disables limiting.
	RequestsPerMinute int
	// UserAgent is sent on every request.
	UserAgent string
}

// HTTPClient is the real UptimeRobot Client backed by net/http. It is safe for
// concurrent use.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
	token      TokenFunc
	userAgent  string
}

// compile-time assertion that HTTPClient implements Client.
var _ Client = (*HTTPClient)(nil)

// NewHTTPClient builds an HTTPClient from opts.
func NewHTTPClient(opts Options) *HTTPClient {
	base := opts.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	base = strings.TrimRight(base, "/")

	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	var limiter *rate.Limiter
	if opts.RequestsPerMinute > 0 {
		// Convert per-minute to per-second; allow a small burst.
		perSec := rate.Limit(float64(opts.RequestsPerMinute) / 60.0)
		burst := max(opts.RequestsPerMinute/4, 1)
		limiter = rate.NewLimiter(perSec, burst)
	}

	ua := opts.UserAgent
	if ua == "" {
		ua = "uptimerobot-operator"
	}

	return &HTTPClient{
		baseURL:    base,
		httpClient: hc,
		limiter:    limiter,
		token:      opts.Token,
		userAgent:  ua,
	}
}

// ---- request plumbing ------------------------------------------------------

// doJSON performs an HTTP request, honouring the rate limiter, and decodes a
// successful JSON body into out (which may be nil to discard the body). Non-2xx
// responses are converted to *APIError.
func (c *HTTPClient) doJSON(ctx context.Context, method, path string, query url.Values, body, out any) error {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
	}

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if c.token == nil {
		return &APIError{Code: CodeKeyMissing, HTTPStatus: 0, Message: "no token source configured"}
	}
	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("resolve token: %w", err)
	}
	if token == "" {
		return &APIError{Code: CodeKeyMissing, Message: "empty API token"}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp, data)
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// errorBody is the common error envelope returned by the API. message may be a
// string or an array of strings, so it is captured raw and flattened.
type errorBody struct {
	Message    json.RawMessage `json:"message"`
	Error      string          `json:"error"`
	StatusCode int             `json:"statusCode"`
	Code       string          `json:"code"`
}

func parseAPIError(resp *http.Response, data []byte) *APIError {
	apiErr := &APIError{HTTPStatus: resp.StatusCode}

	var eb errorBody
	if err := json.Unmarshal(data, &eb); err == nil {
		apiErr.Code = eb.Code
		apiErr.Message = flattenMessage(eb.Message)
		if apiErr.Message == "" && eb.Error != "" {
			apiErr.Message = eb.Error
		}
	}
	if apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(data))
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(resp.StatusCode)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil {
				apiErr.RetryAfter = time.Duration(secs) * time.Second
			}
		}
		if apiErr.RetryAfter == 0 {
			apiErr.RetryAfter = 60 * time.Second // sensible default for the free plan
		}
	}
	return apiErr
}

// flattenMessage turns a string or []string message into a single string.
func flattenMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return strings.Join(list, "; ")
	}
	return string(raw)
}

// ---- Client implementation -------------------------------------------------

func (c *HTTPClient) CreateMonitor(ctx context.Context, m *Monitor) (*Monitor, error) {
	body := buildRequestBody(m, true)
	var resp monitorResponse
	if err := c.doJSON(ctx, http.MethodPost, "/monitors", nil, body, &resp); err != nil {
		return nil, err
	}
	return resp.toMonitor(), nil
}

func (c *HTTPClient) GetMonitor(ctx context.Context, id int64) (*Monitor, error) {
	var resp monitorResponse
	if err := c.doJSON(ctx, http.MethodGet, "/monitors/"+strconv.FormatInt(id, 10), nil, nil, &resp); err != nil {
		return nil, err
	}
	return resp.toMonitor(), nil
}

func (c *HTTPClient) ListMonitors(ctx context.Context, opts ListOptions) ([]Monitor, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	if opts.Search != "" {
		q.Set("search", opts.Search)
	}
	var resp monitorListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/monitors", q, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Monitor, 0, len(resp.Data))
	for i := range resp.Data {
		out = append(out, *resp.Data[i].toMonitor())
	}
	return out, nil
}

func (c *HTTPClient) UpdateMonitor(ctx context.Context, id int64, m *Monitor) (*Monitor, error) {
	// type is immutable server-side and must not be sent on update.
	body := buildRequestBody(m, false)
	var resp monitorResponse
	if err := c.doJSON(ctx, http.MethodPatch, "/monitors/"+strconv.FormatInt(id, 10), nil, body, &resp); err != nil {
		return nil, err
	}
	return resp.toMonitor(), nil
}

func (c *HTTPClient) DeleteMonitor(ctx context.Context, id int64) error {
	return c.doJSON(ctx, http.MethodDelete, "/monitors/"+strconv.FormatInt(id, 10), nil, nil, nil)
}

func (c *HTTPClient) PauseMonitor(ctx context.Context, id int64) error {
	return c.doJSON(ctx, http.MethodPost, "/monitors/"+strconv.FormatInt(id, 10)+"/pause", nil, nil, nil)
}

func (c *HTTPClient) StartMonitor(ctx context.Context, id int64) error {
	return c.doJSON(ctx, http.MethodPost, "/monitors/"+strconv.FormatInt(id, 10)+"/start", nil, nil, nil)
}

func (c *HTTPClient) ResetStats(ctx context.Context, id int64) error {
	return c.doJSON(ctx, http.MethodPost, "/monitors/"+strconv.FormatInt(id, 10)+"/reset", nil, nil, nil)
}

func (c *HTTPClient) UptimeStats(ctx context.Context, id int64, from, to string) (*UptimeStats, error) {
	q := url.Values{}
	if from != "" {
		q.Set("from", from)
	}
	if to != "" {
		q.Set("to", to)
	}
	var resp monitorUptimeStatsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/monitors/"+strconv.FormatInt(id, 10)+"/stats/uptime", q, nil, &resp); err != nil {
		return nil, err
	}
	return &UptimeStats{
		UptimePercentage: resp.Uptime,
		DowntimeSeconds:  resp.TotalDowntimeSeconds,
		IncidentCount:    resp.IncidentCount,
		MTBFSeconds:      resp.MTBF,
		From:             resp.From,
		To:               resp.To,
	}, nil
}

func (c *HTTPClient) ResponseTimeStats(ctx context.Context, id int64, from, to string) (*ResponseTimeStats, error) {
	q := url.Values{}
	if from != "" {
		q.Set("from", from)
	}
	if to != "" {
		q.Set("to", to)
	}
	var resp monitorResponseTimeStatsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/monitors/"+strconv.FormatInt(id, 10)+"/stats/response-time", q, nil, &resp); err != nil {
		return nil, err
	}
	return &ResponseTimeStats{
		MinMS:      resp.Summary.Min,
		MaxMS:      resp.Summary.Max,
		AvgMS:      resp.Summary.Avg,
		DataPoints: resp.DataPoints,
		From:       resp.From,
		To:         resp.To,
	}, nil
}
