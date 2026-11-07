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

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/annotations"
	"github.com/DarkCityLab/uptimerobot-operator/internal/resolver"
)

// IngressReconciler watches annotated Ingresses and generates Monitor CRs.
type IngressReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	DefaultAlertContacts []monitoringv1alpha1.AlertContactAssignment

	resolver resolver.Ingress
}

// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.homelab.io,resources=monitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile translates an annotated Ingress into Monitor CRs (one per host).
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var ing networkingv1.Ingress
	if err := r.Get(ctx, req.NamespacedName, &ing); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	enabled := ing.DeletionTimestamp.IsZero() && annotations.IsEnabled(ing.Annotations)

	var targets []hostTarget
	if enabled {
		hosts, err := r.resolver.Hosts(&ing)
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, h := range hosts {
			// TLS hosts default to https, others to http.
			defaultScheme := "http"
			if resolver.IsTLSHost(&ing, h) {
				defaultScheme = "https"
			}
			targets = append(targets, hostTarget{host: h, url: annotations.BuildURL(ing.Annotations, h, defaultScheme)})
		}
		if len(hosts) == 0 {
			log.V(1).Info("Ingress enabled but exposes no hostnames", "ingress", req.NamespacedName)
		}
	}

	syncer := &sourceSyncer{Client: r.Client, Scheme: r.Scheme, DefaultAlertContacts: r.DefaultAlertContacts}
	if err := syncer.sync(ctx, &ing, "Ingress", enabled, ing.Annotations, targets); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager wires the controller, watching owned Monitors for cleanup.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Owns(&monitoringv1alpha1.Monitor{}).
		Named("ingress").
		Complete(r)
}
