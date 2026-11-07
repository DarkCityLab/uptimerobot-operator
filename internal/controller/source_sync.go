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
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/annotations"
)

const (
	// labelSourceUID marks Monitors generated from a source object, keyed by the
	// source's UID, so the owning watcher can list and prune them.
	labelSourceUID = "monitoring.homelab.io/source-uid"
	// labelSourceKind records the source kind for observability.
	labelSourceKind = "monitoring.homelab.io/source-kind"
)

// sourceSyncer contains the shared logic for watchers that translate an
// annotated source object (HTTPRoute, Ingress) into owned Monitor CRs.
type sourceSyncer struct {
	client.Client
	Scheme               *runtime.Scheme
	DefaultAlertContacts []monitoringv1alpha1.AlertContactAssignment
}

// hostTarget pairs a hostname with the derived monitor URL.
type hostTarget struct {
	host string
	url  string
}

// sync reconciles the set of Monitors owned by the source to exactly match
// targets (when enabled). When enabled is false it prunes all owned Monitors.
func (s *sourceSyncer) sync(ctx context.Context, owner client.Object, kind string, enabled bool, ann map[string]string, targets []hostTarget) error {
	log := logf.FromContext(ctx)

	// Desired monitor name -> target.
	desired := map[string]hostTarget{}
	if enabled {
		for _, t := range targets {
			desired[monitorName(owner.GetName(), t.host)] = t
		}
	}

	// Existing monitors owned by this source.
	var existing monitoringv1alpha1.MonitorList
	if err := s.List(ctx, &existing,
		client.InNamespace(owner.GetNamespace()),
		client.MatchingLabels{labelSourceUID: string(owner.GetUID())},
	); err != nil {
		return err
	}

	// Prune monitors no longer desired.
	keep := map[string]struct{}{}
	for name := range desired {
		keep[name] = struct{}{}
	}
	for i := range existing.Items {
		m := &existing.Items[i]
		if _, ok := keep[m.Name]; !ok {
			if err := s.Delete(ctx, m); err != nil && client.IgnoreNotFound(err) != nil {
				return err
			}
			log.Info("pruned generated monitor", "monitor", m.Name)
		}
	}

	// Create or update desired monitors.
	for name, t := range desired {
		mon := &monitoringv1alpha1.Monitor{}
		mon.Name = name
		mon.Namespace = owner.GetNamespace()

		_, err := controllerutil.CreateOrUpdate(ctx, s.Client, mon, func() error {
			if mon.Labels == nil {
				mon.Labels = map[string]string{}
			}
			mon.Labels[labelSourceUID] = string(owner.GetUID())
			mon.Labels[labelSourceKind] = kind

			spec := annotations.BuildMonitorSpec(ann, t.url, s.DefaultAlertContacts)
			// Preserve fields the user may have hand-tuned that we do not derive.
			spec.CollectStats = mon.Spec.CollectStats
			spec.StatsRefreshInterval = mon.Spec.StatsRefreshInterval
			mon.Spec = spec

			return controllerutil.SetControllerReference(owner, mon, s.Scheme)
		})
		if err != nil {
			return fmt.Errorf("sync monitor %s: %w", name, err)
		}
		log.V(1).Info("synced generated monitor", "monitor", name, "url", t.url)
	}

	return nil
}

// monitorName derives a deterministic, DNS-1123-safe Monitor name from the
// source name and host.
func monitorName(sourceName, host string) string {
	sum := sha256.Sum256([]byte(host))
	suffix := hex.EncodeToString(sum[:])[:8]
	base := sourceName
	// Reserve room for "-" + 8-char suffix within the 253-char limit.
	const maxBase = 253 - 9
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + "-" + suffix
}
