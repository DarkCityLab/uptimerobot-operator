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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewHTTPClient(Options{
		BaseURL: srv.URL,
		Token:   StaticToken("test-token"),
	})
}

func TestCreateMonitorSendsBearerAndParsesID(t *testing.T) {
	var gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":12345,"friendlyName":"x","type":"HTTP","url":"https://e.com","status":"UP"}`)
	})

	m, err := c.CreateMonitor(context.Background(), &Monitor{Type: TypeHTTP, URL: "https://e.com", FriendlyName: "x", Interval: 300})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != 12345 {
		t.Errorf("want ID 12345, got %d", m.ID)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("want bearer header, got %q", gotAuth)
	}
}

// TestActionEndpointsSendJSONContentType guards against the HTTP 415 regression:
// the pause/start/reset endpoints POST with no caller-supplied body, but
// UptimeRobot v3 still requires Content-Type: application/json (and a body).
func TestActionEndpointsSendJSONContentType(t *testing.T) {
	cases := []struct {
		name string
		call func(c *HTTPClient) error
	}{
		{"pause", func(c *HTTPClient) error { return c.PauseMonitor(context.Background(), 1) }},
		{"start", func(c *HTTPClient) error { return c.StartMonitor(context.Background(), 1) }},
		{"reset", func(c *HTTPClient) error { return c.ResetStats(context.Background(), 1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotCT, gotBody string
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotCT = r.Header.Get("Content-Type")
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{}`)
			})
			if err := tc.call(c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotCT != "application/json" {
				t.Errorf("want Content-Type application/json, got %q", gotCT)
			}
			if gotBody == "" {
				t.Error("want a non-empty JSON body on a bodyless write, got empty")
			}
		})
	}
}

// TestConfigHashReflectsCredential guards the rotation mechanism: the hash must
// change when the resolved credential changes and be stable otherwise.
func TestConfigHashReflectsCredential(t *testing.T) {
	base := &Monitor{Type: TypeHTTP, URL: "https://e.com", FriendlyName: "h", Interval: 300, AuthType: "BEARER", HTTPPassword: "token-v1"}
	same := &Monitor{Type: TypeHTTP, URL: "https://e.com", FriendlyName: "h", Interval: 300, AuthType: "BEARER", HTTPPassword: "token-v1"}
	rotated := &Monitor{Type: TypeHTTP, URL: "https://e.com", FriendlyName: "h", Interval: 300, AuthType: "BEARER", HTTPPassword: "token-v2"}

	if ConfigHash(base) == "" {
		t.Fatal("ConfigHash returned empty string")
	}
	if ConfigHash(base) != ConfigHash(same) {
		t.Error("ConfigHash should be stable for identical config")
	}
	if ConfigHash(base) == ConfigHash(rotated) {
		t.Error("ConfigHash must change when the resolved credential changes (rotation)")
	}
}

func TestCreateMonitorBodyPerType(t *testing.T) {
	cases := []struct {
		name        string
		monitor     Monitor
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "HTTP",
			monitor:     Monitor{Type: TypeHTTP, URL: "https://e.com", FriendlyName: "h", Interval: 300, HTTPMethodType: "GET"},
			wantPresent: []string{"type", "url", "httpMethodType", "friendlyName", "interval"},
			wantAbsent:  []string{"keywordValue", "port", "gracePeriod"},
		},
		{
			name:        "KEYWORD",
			monitor:     Monitor{Type: TypeKeyword, URL: "https://e.com", FriendlyName: "k", Interval: 300, KeywordValue: "ok", KeywordType: "ALERT_EXISTS", KeywordCaseType: "CaseSensitive"},
			wantPresent: []string{"keywordValue", "keywordType", "keywordCaseType", "url"},
			wantAbsent:  []string{"port", "gracePeriod"},
		},
		{
			name:        "PORT",
			monitor:     Monitor{Type: TypePort, URL: "e.com", FriendlyName: "p", Interval: 300, Port: new(443), PortAlertCondition: "OPEN"},
			wantPresent: []string{"port", "portAlertCondition", "url"},
			wantAbsent:  []string{"keywordValue", "gracePeriod", "httpMethodType"},
		},
		{
			name:        "HEARTBEAT",
			monitor:     Monitor{Type: TypeHeartbeat, FriendlyName: "hb", Interval: 300, GracePeriod: new(30)},
			wantPresent: []string{"gracePeriod", "friendlyName"},
			wantAbsent:  []string{"url", "timeout", "port", "keywordValue"},
		},
		{
			name: "THIRD_PARTY_SERVICE",
			monitor: Monitor{Type: TypeThirdPartyService, FriendlyName: "tp", Interval: 300, Config: &Config{
				ThirdPartyService: &ThirdPartyService{ProviderID: 7},
			}},
			wantPresent: []string{"config", "friendlyName"},
			wantAbsent:  []string{"url", "timeout", "port"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				data, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(data, &body)
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":1}`)
			})
			if _, err := c.CreateMonitor(context.Background(), &tc.monitor); err != nil {
				t.Fatalf("create: %v", err)
			}
			for _, k := range tc.wantPresent {
				if _, ok := body[k]; !ok {
					t.Errorf("want key %q present in body %v", k, body)
				}
			}
			for _, k := range tc.wantAbsent {
				if _, ok := body[k]; ok {
					t.Errorf("want key %q absent in body %v", k, body)
				}
			}
		})
	}
}

func TestUpdateMonitorOmitsType(t *testing.T) {
	var body map[string]json.RawMessage
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("want PATCH, got %s", r.Method)
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		_, _ = io.WriteString(w, `{"id":1}`)
	})
	if _, err := c.UpdateMonitor(context.Background(), 1, &Monitor{Type: TypeHTTP, URL: "https://e.com", FriendlyName: "h", Interval: 300}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := body["type"]; ok {
		t.Errorf("update must not send type, got body %v", body)
	}
}

func TestErrorMappingAndRetryAfter(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		retryAfter string
		check      func(err error) bool
	}{
		{"not found", 404, `{"code":"000-004","message":"not found"}`, "", IsNotFound},
		{"duplicate", 409, `{"code":"009-011","message":"exists"}`, "", IsDuplicate},
		{"rate limited", 429, `{"code":"000-014","message":"slow down"}`, "30", IsRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.GetMonitor(context.Background(), 1)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !tc.check(err) {
				t.Errorf("error classification failed for %v", err)
			}
			if tc.name == "rate limited" && RetryAfter(err).Seconds() != 30 {
				t.Errorf("want RetryAfter 30s, got %v", RetryAfter(err))
			}
		})
	}
}

func TestErrorMessageListFlattening(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"code":"000-012","message":["a must be set","b invalid"]}`)
	})
	_, err := c.GetMonitor(context.Background(), 1)
	apiErr := asAPIError(err)
	if apiErr == nil {
		t.Fatalf("want APIError, got %T", err)
	}
	if apiErr.Message != "a must be set; b invalid" {
		t.Errorf("want flattened message, got %q", apiErr.Message)
	}
	if !apiErr.IsTerminal() {
		t.Error("400 should be terminal")
	}
}
