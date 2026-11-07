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

package annotations

import (
	"testing"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
)

func TestIsEnabled(t *testing.T) {
	cases := map[string]bool{"true": true, "TRUE": true, " true ": true, "false": false, "": false, "yes": false}
	for val, want := range cases {
		if got := IsEnabled(map[string]string{Enabled: val}); got != want {
			t.Errorf("IsEnabled(%q) = %v, want %v", val, got, want)
		}
	}
}

func TestBuildURL(t *testing.T) {
	cases := []struct {
		name          string
		ann           map[string]string
		host          string
		defaultScheme string
		want          string
	}{
		{"default https", nil, "x.example.com", "https", "https://x.example.com"},
		{"default http", nil, "x.example.com", "http", "http://x.example.com"},
		{"scheme override", map[string]string{Scheme: "http"}, "x.example.com", "https", "http://x.example.com"},
		{"path with slash", map[string]string{Path: "/healthz"}, "x.example.com", "https", "https://x.example.com/healthz"},
		{"path without slash", map[string]string{Path: "healthz"}, "x.example.com", "https", "https://x.example.com/healthz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BuildURL(tc.ann, tc.host, tc.defaultScheme); got != tc.want {
				t.Errorf("BuildURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAlertContacts(t *testing.T) {
	got := ParseAlertContacts("123, 456 ,bad,789")
	want := []int64{123, 456, 789}
	if len(got) != len(want) {
		t.Fatalf("want %d contacts, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i].AlertContactID != want[i] {
			t.Errorf("contact[%d] = %d, want %d", i, got[i].AlertContactID, want[i])
		}
	}
	if ParseAlertContacts("") != nil {
		t.Error("empty input should yield nil")
	}
}

func TestBuildMonitorSpec(t *testing.T) {
	defaults := []monitoringv1alpha1.AlertContactAssignment{{AlertContactID: 99}}

	t.Run("defaults and type override", func(t *testing.T) {
		spec := BuildMonitorSpec(map[string]string{
			Type:     "keyword",
			Interval: "60",
			Timeout:  "20",
		}, "https://x.example.com", defaults)

		if spec.Type != monitoringv1alpha1.MonitorKeyword {
			t.Errorf("type = %q, want KEYWORD", spec.Type)
		}
		if spec.Interval != 60 {
			t.Errorf("interval = %d, want 60", spec.Interval)
		}
		if spec.Timeout == nil || *spec.Timeout != 20 {
			t.Errorf("timeout = %v, want 20", spec.Timeout)
		}
		if len(spec.AlertContacts) != 1 || spec.AlertContacts[0].AlertContactID != 99 {
			t.Errorf("want default alert contact, got %v", spec.AlertContacts)
		}
	})

	t.Run("per-source alert contacts override default", func(t *testing.T) {
		spec := BuildMonitorSpec(map[string]string{AlertContacts: "7"}, "https://x", defaults)
		if len(spec.AlertContacts) != 1 || spec.AlertContacts[0].AlertContactID != 7 {
			t.Errorf("want override contact 7, got %v", spec.AlertContacts)
		}
	})

	t.Run("paused annotation", func(t *testing.T) {
		if spec := BuildMonitorSpec(map[string]string{Paused: "true"}, "https://x", nil); !spec.Paused {
			t.Error("Paused annotation true should set spec.Paused")
		}
		if spec := BuildMonitorSpec(map[string]string{}, "https://x", nil); spec.Paused {
			t.Error("absent Paused annotation should leave spec.Paused false")
		}
	})

	t.Run("http config from annotations", func(t *testing.T) {
		spec := BuildMonitorSpec(map[string]string{
			HTTPMethod:      "post",
			AuthType:        "bearer",
			AuthSecret:      "endpoint-token",
			HTTPBody:        `{"ping":true}`,
			HTTPBodyType:    "raw_json",
			HTTPHeaders:     `{"X-Env":"prod"}`,
			SuccessCodes:    "2xx, 3xx",
			FollowRedirects: "true",
			CheckSSLErrors:  "false",
		}, "https://x", nil)

		if spec.HTTP == nil {
			t.Fatal("expected spec.HTTP to be populated from annotations")
		}
		h := spec.HTTP
		if h.Method != "POST" {
			t.Errorf("method = %q, want POST", h.Method)
		}
		if h.AuthType != "BEARER" {
			t.Errorf("authType = %q, want BEARER", h.AuthType)
		}
		if h.PasswordSecretRef == nil || h.PasswordSecretRef.Name != "endpoint-token" || h.PasswordSecretRef.Key != "password" {
			t.Errorf("passwordSecretRef = %+v, want {endpoint-token, password}", h.PasswordSecretRef)
		}
		if h.PostValueData != `{"ping":true}` || h.PostValueType != "RAW_JSON" {
			t.Errorf("body = %q/%q", h.PostValueData, h.PostValueType)
		}
		if len(h.CustomHeaders) != 1 || h.CustomHeaders["X-Env"] != "prod" {
			t.Errorf("headers = %v", h.CustomHeaders)
		}
		if len(h.SuccessHTTPResponseCodes) != 2 {
			t.Errorf("successCodes = %v", h.SuccessHTTPResponseCodes)
		}
		if h.FollowRedirections == nil || !*h.FollowRedirections {
			t.Error("followRedirections should be true")
		}
		if h.CheckSSLErrors == nil || *h.CheckSSLErrors {
			t.Error("checkSSLErrors should be false")
		}
	})

	t.Run("no http annotations leaves HTTP nil", func(t *testing.T) {
		if spec := BuildMonitorSpec(map[string]string{Type: "HTTP"}, "https://x", nil); spec.HTTP != nil {
			t.Errorf("expected nil HTTP, got %+v", spec.HTTP)
		}
	})
}
