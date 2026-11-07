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

// Package annotations defines the operator's annotation contract and helpers to
// translate an annotated source object (HTTPRoute/Ingress) into a Monitor spec.
package annotations

import (
	"encoding/json"
	"strconv"
	"strings"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
)

// Annotation keys recognised on source objects (HTTPRoute, Ingress) and on the
// Monitor itself (ResetStats).
const (
	// Prefix namespaces all operator annotations.
	Prefix = "monitoring.homelab.io/"

	// Enabled opts a source object in to monitoring ("true").
	Enabled = Prefix + "enabled"
	// Scheme overrides the URL scheme (default https, or http for non-TLS Ingress).
	Scheme = Prefix + "scheme"
	// Path appends a path to the derived URL (e.g. "/healthz").
	Path = Prefix + "path"
	// Type overrides the monitor type (default HTTP).
	Type = Prefix + "type"
	// Interval overrides the check interval in seconds.
	Interval = Prefix + "interval"
	// Timeout overrides the check timeout in seconds.
	Timeout = Prefix + "timeout"
	// AlertContacts is a comma-separated list of alert-contact IDs overriding
	// the operator-wide default for monitors derived from this source.
	AlertContacts = Prefix + "alert-contacts"
	// Paused pauses ("true") or resumes the monitor derived from this source.
	Paused = Prefix + "paused"

	// --- HTTP-monitor configuration (source parity with a standalone CR) ---
	// HTTPMethod sets the request method (HEAD/GET/POST/PUT/PATCH/DELETE/OPTIONS/QUERY).
	HTTPMethod = Prefix + "http-method"
	// AuthType selects the HTTP auth scheme (NONE/HTTP_BASIC/DIGEST/BEARER).
	AuthType = Prefix + "auth-type"
	// AuthUsername is the username for HTTP_BASIC/DIGEST.
	AuthUsername = Prefix + "auth-username"
	// AuthSecret names a Secret (same namespace) holding the password/token.
	AuthSecret = Prefix + "auth-secret"
	// AuthSecretKey is the key within AuthSecret (default "password").
	AuthSecretKey = Prefix + "auth-secret-key"
	// HTTPHeaders is a JSON object of custom request headers.
	HTTPHeaders = Prefix + "http-headers"
	// HTTPBody is the request payload (non-HEAD methods).
	HTTPBody = Prefix + "http-body"
	// HTTPBodyType selects the payload encoding (KEY_VALUE/RAW_JSON).
	HTTPBodyType = Prefix + "http-body-type"
	// SuccessCodes is a comma-separated list of success codes/ranges (e.g. "2xx,3xx").
	SuccessCodes = Prefix + "success-codes"
	// FollowRedirects toggles following redirects ("true"/"false").
	FollowRedirects = Prefix + "follow-redirects"
	// CheckSSLErrors toggles SSL/domain expiration error checks ("true"/"false").
	CheckSSLErrors = Prefix + "check-ssl-errors"
	// SSLExpiryReminder toggles SSL-expiry notifications ("true"/"false").
	SSLExpiryReminder = Prefix + "ssl-expiry-reminder"
	// DomainExpiryReminder toggles domain-expiry notifications ("true"/"false").
	DomainExpiryReminder = Prefix + "domain-expiry-reminder"

	// ResetStats is a nonce; changing its value triggers a one-shot reset-stats
	// call. Honoured on a Monitor directly and on a source object (the source
	// watcher propagates it to the generated Monitor).
	ResetStats = Prefix + "reset-stats"
)

// IsEnabled reports whether ann opts in to monitoring.
func IsEnabled(ann map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(ann[Enabled]), "true")
}

// IsPaused reports whether ann requests the monitor be paused.
func IsPaused(ann map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(ann[Paused]), "true")
}

// BuildURL derives a monitor URL from host and the source's annotations.
// defaultScheme is used when no scheme annotation is present.
func BuildURL(ann map[string]string, host, defaultScheme string) string {
	scheme := defaultScheme
	if s := strings.TrimSpace(ann[Scheme]); s != "" {
		scheme = s
	}
	path := strings.TrimSpace(ann[Path])
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return scheme + "://" + host + path
}

// ParseAlertContacts parses a comma-separated list of alert-contact IDs.
// Invalid entries are skipped.
func ParseAlertContacts(raw string) []monitoringv1alpha1.AlertContactAssignment {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []monitoringv1alpha1.AlertContactAssignment
	for part := range strings.SplitSeq(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			continue
		}
		out = append(out, monitoringv1alpha1.AlertContactAssignment{AlertContactID: id})
	}
	return out
}

// BuildMonitorSpec produces a Monitor spec from a derived url and the source
// annotations. defaultAlertContacts is applied when the source does not specify
// its own via the AlertContacts annotation.
//
// The interval/timeout defaults MUST mirror the CRD field defaults, otherwise
// the apiserver re-defaults the stored object and the source watcher's
// CreateOrUpdate never converges (an update-churn loop).
func BuildMonitorSpec(ann map[string]string, url string, defaultAlertContacts []monitoringv1alpha1.AlertContactAssignment) monitoringv1alpha1.MonitorSpec {
	defaultTimeout := 30
	spec := monitoringv1alpha1.MonitorSpec{
		Type:     monitoringv1alpha1.MonitorHTTP,
		URL:      url,
		Interval: 300,
		Timeout:  &defaultTimeout,
	}

	if t := strings.TrimSpace(ann[Type]); t != "" {
		spec.Type = monitoringv1alpha1.MonitorType(strings.ToUpper(t))
	}
	if v := strings.TrimSpace(ann[Interval]); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			spec.Interval = n
		}
	}
	if v := strings.TrimSpace(ann[Timeout]); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			spec.Timeout = &n
		}
	}

	if contacts := ParseAlertContacts(ann[AlertContacts]); len(contacts) > 0 {
		spec.AlertContacts = contacts
	} else if len(defaultAlertContacts) > 0 {
		spec.AlertContacts = defaultAlertContacts
	}

	spec.Paused = IsPaused(ann)

	if h := buildHTTPSettings(ann); h != nil {
		spec.HTTP = h
	}

	return spec
}

// buildHTTPSettings assembles HTTPSettings from the HTTP-config annotations, or
// returns nil when none are present. A credential Secret is referenced by name
// (AuthSecret) + key (AuthSecretKey, default "password"); the Monitor controller
// resolves it — the annotation never carries the secret value.
func buildHTTPSettings(ann map[string]string) *monitoringv1alpha1.HTTPSettings {
	h := &monitoringv1alpha1.HTTPSettings{}
	set := false

	if v := strings.TrimSpace(ann[HTTPMethod]); v != "" {
		h.Method = strings.ToUpper(v)
		set = true
	}
	if v := strings.TrimSpace(ann[AuthType]); v != "" {
		h.AuthType = strings.ToUpper(v)
		set = true
	}
	if v := strings.TrimSpace(ann[AuthUsername]); v != "" {
		h.Username = v
		set = true
	}
	if v := strings.TrimSpace(ann[AuthSecret]); v != "" {
		key := strings.TrimSpace(ann[AuthSecretKey])
		if key == "" {
			key = "password"
		}
		h.PasswordSecretRef = &monitoringv1alpha1.SecretKeySelector{Name: v, Key: key}
		set = true
	}
	if v := strings.TrimSpace(ann[HTTPHeaders]); v != "" {
		headers := map[string]string{}
		if err := json.Unmarshal([]byte(v), &headers); err == nil && len(headers) > 0 {
			h.CustomHeaders = headers
			set = true
		}
	}
	if v := strings.TrimSpace(ann[HTTPBody]); v != "" {
		h.PostValueData = v
		set = true
	}
	if v := strings.TrimSpace(ann[HTTPBodyType]); v != "" {
		h.PostValueType = strings.ToUpper(v)
		set = true
	}
	if v := strings.TrimSpace(ann[SuccessCodes]); v != "" {
		var codes []string
		for part := range strings.SplitSeq(v, ",") {
			if c := strings.TrimSpace(part); c != "" {
				codes = append(codes, c)
			}
		}
		if len(codes) > 0 {
			h.SuccessHTTPResponseCodes = codes
			set = true
		}
	}
	if b, ok := parseBoolAnnotation(ann[FollowRedirects]); ok {
		h.FollowRedirections = &b
		set = true
	}
	if b, ok := parseBoolAnnotation(ann[CheckSSLErrors]); ok {
		h.CheckSSLErrors = &b
		set = true
	}
	if b, ok := parseBoolAnnotation(ann[SSLExpiryReminder]); ok {
		h.SSLExpirationReminder = &b
		set = true
	}
	if b, ok := parseBoolAnnotation(ann[DomainExpiryReminder]); ok {
		h.DomainExpirationReminder = &b
		set = true
	}

	if !set {
		return nil
	}
	return h
}

// parseBoolAnnotation parses "true"/"false" (case-insensitive); ok is false when
// the value is absent or unparseable, so the field is left at its default.
func parseBoolAnnotation(v string) (val bool, ok bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(strings.ToLower(v))
	if err != nil {
		return false, false
	}
	return b, true
}
