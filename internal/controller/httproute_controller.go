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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/annotations"
	"github.com/DarkCityLab/uptimerobot-operator/internal/resolver"
)

// HTTPRouteReconciler watches annotated HTTPRoutes and generates Monitor CRs.
type HTTPRouteReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	DefaultAlertContacts []monitoringv1alpha1.AlertContactAssignment

	resolver resolver.HTTPRoute
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.homelab.io,resources=monitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile translates an annotated HTTPRoute into Monitor CRs (one per hostname).
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var route gatewayv1.HTTPRoute
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		// Deleted: owned Monitors are garbage-collected via owner references.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	enabled := route.DeletionTimestamp.IsZero() && annotations.IsEnabled(route.Annotations)

	var targets []hostTarget
	if enabled {
		hosts, err := r.resolver.Hosts(&route)
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, h := range hosts {
			targets = append(targets, hostTarget{host: h, url: annotations.BuildURL(route.Annotations, h, "https")})
		}
		if len(hosts) == 0 {
			log.V(1).Info("HTTPRoute enabled but exposes no hostnames", "route", req.NamespacedName)
		}
	}

	syncer := &sourceSyncer{Client: r.Client, Scheme: r.Scheme, DefaultAlertContacts: r.DefaultAlertContacts}
	if err := syncer.sync(ctx, &route, "HTTPRoute", enabled, route.Annotations, targets); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager wires the controller, watching owned Monitors for cleanup.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.HTTPRoute{}).
		Owns(&monitoringv1alpha1.Monitor{}).
		Named("httproute").
		Complete(r)
}
