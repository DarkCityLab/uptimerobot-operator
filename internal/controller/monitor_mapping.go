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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	ur "github.com/DarkCityLab/uptimerobot-operator/internal/uptimerobot"
)

// buildClientMonitor maps a Monitor CR spec into the uptimerobot domain model,
// applying operator-wide defaults (friendly name, alert contacts) and resolving
// the HTTP auth password from its Secret when referenced.
func (r *MonitorReconciler) buildClientMonitor(ctx context.Context, mon *monitoringv1alpha1.Monitor) (*ur.Monitor, error) {
	spec := mon.Spec

	friendly := spec.FriendlyName
	if friendly == "" {
		friendly = mon.Namespace + "/" + mon.Name
	}

	m := &ur.Monitor{
		FriendlyName:          friendly,
		Type:                  ur.MonitorType(spec.Type),
		URL:                   spec.URL,
		Interval:              spec.Interval,
		Timeout:               spec.Timeout,
		Paused:                spec.Paused,
		TagNames:              spec.TagNames,
		MaintenanceWindowIDs:  spec.MaintenanceWindowIDs,
		GroupID:               spec.GroupID,
		CustomFields:          spec.CustomFields,
		ResponseTimeThreshold: spec.ResponseTimeThreshold,
		Port:                  spec.Port,
		PortAlertCondition:    spec.PortAlertCondition,
		GracePeriod:           spec.GracePeriod,
	}

	// Alert contacts: per-CR override, else operator-wide default.
	contacts := spec.AlertContacts
	if len(contacts) == 0 {
		contacts = r.DefaultAlertContacts
	}
	for _, c := range contacts {
		m.AlertContacts = append(m.AlertContacts, ur.AlertContact{
			AlertContactID: c.AlertContactID,
			Threshold:      c.Threshold,
			Recurrence:     c.Recurrence,
		})
	}

	if spec.Region != nil {
		m.Region = &ur.RegionData{
			Regions:        spec.Region.Regions,
			ManualSelected: spec.Region.ManualSelected,
		}
		if spec.Region.Threshold != nil {
			m.Region.Threshold = map[string]int{}
			if spec.Region.Threshold.NA != 0 {
				m.Region.Threshold["na"] = spec.Region.Threshold.NA
			}
			if spec.Region.Threshold.EU != 0 {
				m.Region.Threshold["eu"] = spec.Region.Threshold.EU
			}
			if spec.Region.Threshold.AS != 0 {
				m.Region.Threshold["as"] = spec.Region.Threshold.AS
			}
			if spec.Region.Threshold.OC != 0 {
				m.Region.Threshold["oc"] = spec.Region.Threshold.OC
			}
		}
	}

	if h := spec.HTTP; h != nil {
		m.HTTPMethodType = h.Method
		m.AuthType = h.AuthType
		m.HTTPUsername = h.Username
		m.CustomHTTPHeaders = h.CustomHeaders
		m.SuccessHTTPResponseCodes = h.SuccessHTTPResponseCodes
		m.PostValueData = h.PostValueData
		m.PostValueType = h.PostValueType
		m.FollowRedirections = h.FollowRedirections
		m.CheckSSLErrors = h.CheckSSLErrors
		m.SSLExpirationReminder = h.SSLExpirationReminder
		m.DomainExpirationReminder = h.DomainExpirationReminder
		if h.PasswordSecretRef != nil {
			pw, err := r.resolveSecretKey(ctx, mon.Namespace, h.PasswordSecretRef)
			if err != nil {
				return nil, err
			}
			m.HTTPPassword = pw
		}
	}

	if k := spec.Keyword; k != nil {
		m.KeywordValue = k.Value
		m.KeywordType = k.AlertWhen
		if k.CaseSensitive {
			m.KeywordCaseType = "CaseSensitive"
		} else {
			m.KeywordCaseType = "CaseInsensitive"
		}
	}

	if spec.Config != nil {
		m.Config = buildClientConfig(spec.Config)
	}

	return m, nil
}

// buildClientConfig maps the CRD nested config to the client config.
func buildClientConfig(c *monitoringv1alpha1.MonitorConfig) *ur.Config {
	out := &ur.Config{
		IPVersion:               c.IPVersion,
		SSLExpirationPeriodDays: c.SSLExpirationPeriodDays,
		ApplicationErrorRetries: c.ApplicationErrorRetries,
	}
	if c.DNSRecords != nil {
		out.DNSRecords = &ur.DNSRecords{
			A: c.DNSRecords.A, AAAA: c.DNSRecords.AAAA, CNAME: c.DNSRecords.CNAME,
			MX: c.DNSRecords.MX, NS: c.DNSRecords.NS, TXT: c.DNSRecords.TXT,
			SRV: c.DNSRecords.SRV, PTR: c.DNSRecords.PTR, SOA: c.DNSRecords.SOA,
			SPF: c.DNSRecords.SPF, DNSKEY: c.DNSRecords.DNSKEY, DS: c.DNSRecords.DS,
			NSEC: c.DNSRecords.NSEC, NSEC3: c.DNSRecords.NSEC3,
		}
	}
	if c.APIAssertions != nil {
		checks := make([]ur.APIAssertionCheck, 0, len(c.APIAssertions.Checks))
		for _, chk := range c.APIAssertions.Checks {
			var target any
			if chk.Target != nil {
				target = chk.Target
			}
			checks = append(checks, ur.APIAssertionCheck{
				Property:   chk.Property,
				Comparison: chk.Comparison,
				Target:     target,
			})
		}
		out.APIAssertions = &ur.APIAssertions{Logic: c.APIAssertions.Logic, Checks: checks}
	}
	if c.UDP != nil {
		out.UDP = &ur.UDPConfig{Payload: c.UDP.Payload, PacketLossThreshold: c.UDP.PacketLossThreshold}
	}
	if c.VisualComparison != nil {
		vc := &ur.VisualComparison{
			SensitivityThreshold: c.VisualComparison.SensitivityThreshold,
			Viewport:             c.VisualComparison.Viewport,
		}
		if a := c.VisualComparison.AreaCoordinates; a != nil {
			vc.AreaCoordinates = &ur.AreaCoordinates{
				PercentX:      float64(a.PercentX),
				PercentY:      float64(a.PercentY),
				PercentWidth:  float64(a.PercentWidth),
				PercentHeight: float64(a.PercentHeight),
			}
		}
		out.VisualComparison = vc
	}
	if c.ThirdPartyService != nil {
		tps := &ur.ThirdPartyService{
			ProviderID:          c.ThirdPartyService.ProviderID,
			TrackedComponentIDs: c.ThirdPartyService.TrackedComponentIDs,
		}
		if n := c.ThirdPartyService.Notify; n != nil {
			tps.Notify = &ur.ThirdPartyNotify{Severity: n.Severity, IncrementalUpdates: n.IncrementalUpdates}
		}
		out.ThirdPartyService = tps
	}
	return out
}

// resolveSecretKey fetches a value from a Secret in the given namespace. A
// missing Secret or key returns a clear error so the Monitor controller can
// surface it on status (Ready=False, DependencyError) without deleting the
// already-created UptimeRobot monitor.
func (r *MonitorReconciler) resolveSecretKey(ctx context.Context, namespace string, ref *monitoringv1alpha1.SecretKeySelector) (string, error) {
	var secret corev1.Secret
	key := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := r.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("referenced Secret %s not found", key)
		}
		return "", fmt.Errorf("get Secret %s: %w", key, err)
	}
	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("secret %s has no key %q", key, ref.Key)
	}
	if len(val) == 0 {
		return "", fmt.Errorf("secret %s key %q is empty", key, ref.Key)
	}
	return string(val), nil
}
