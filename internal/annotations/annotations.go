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

	// ResetStats is a nonce on a Monitor; changing its value triggers a one-shot
	// reset-stats call.
	ResetStats = Prefix + "reset-stats"
)

// IsEnabled reports whether ann opts in to monitoring.
func IsEnabled(ann map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(ann[Enabled]), "true")
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

	return spec
}
