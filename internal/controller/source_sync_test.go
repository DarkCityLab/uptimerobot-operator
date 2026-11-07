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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/annotations"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := monitoringv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := gatewayv1.Install(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newRoute() *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httpbin",
			Namespace: "default",
			UID:       types.UID("uid-123"),
			Annotations: map[string]string{
				annotations.Enabled: "true",
			},
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"httpbin.example.com"},
		},
	}
}

func TestSourceSyncerCreatesOwnedMonitor(t *testing.T) {
	scheme := newScheme(t)
	route := newRoute()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()

	syncer := &sourceSyncer{Client: cl, Scheme: scheme}
	targets := []hostTarget{{host: "httpbin.example.com", url: "https://httpbin.example.com"}}
	if err := syncer.sync(context.Background(), route, "HTTPRoute", true, route.Annotations, targets); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var monitors monitoringv1alpha1.MonitorList
	if err := cl.List(context.Background(), &monitors, client.InNamespace("default")); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(monitors.Items) != 1 {
		t.Fatalf("want 1 monitor, got %d", len(monitors.Items))
	}
	mon := monitors.Items[0]
	if mon.Spec.URL != "https://httpbin.example.com" {
		t.Errorf("url = %q", mon.Spec.URL)
	}
	if mon.Labels[labelSourceUID] != "uid-123" {
		t.Errorf("missing source-uid label: %v", mon.Labels)
	}
	if len(mon.OwnerReferences) != 1 || mon.OwnerReferences[0].UID != route.UID {
		t.Errorf("want controller owner ref to route, got %v", mon.OwnerReferences)
	}
	if mon.OwnerReferences[0].Controller == nil || !*mon.OwnerReferences[0].Controller {
		t.Error("owner ref should be a controller ref")
	}
}

func TestSourceSyncerPrunesWhenDisabled(t *testing.T) {
	scheme := newScheme(t)
	route := newRoute()
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	syncer := &sourceSyncer{Client: cl, Scheme: scheme}

	targets := []hostTarget{{host: "httpbin.example.com", url: "https://httpbin.example.com"}}
	if err := syncer.sync(context.Background(), route, "HTTPRoute", true, route.Annotations, targets); err != nil {
		t.Fatalf("sync enabled: %v", err)
	}

	// Disable: prune everything.
	if err := syncer.sync(context.Background(), route, "HTTPRoute", false, route.Annotations, nil); err != nil {
		t.Fatalf("sync disabled: %v", err)
	}

	var monitors monitoringv1alpha1.MonitorList
	if err := cl.List(context.Background(), &monitors, client.InNamespace("default")); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(monitors.Items) != 0 {
		t.Fatalf("want 0 monitors after prune, got %d", len(monitors.Items))
	}
}

func TestSourceSyncerPrunesStaleHost(t *testing.T) {
	scheme := newScheme(t)
	route := newRoute()
	route.Spec.Hostnames = []gatewayv1.Hostname{"a.example.com", "b.example.com"}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	syncer := &sourceSyncer{Client: cl, Scheme: scheme}

	two := []hostTarget{
		{host: "a.example.com", url: "https://a.example.com"},
		{host: "b.example.com", url: "https://b.example.com"},
	}
	if err := syncer.sync(context.Background(), route, "HTTPRoute", true, route.Annotations, two); err != nil {
		t.Fatalf("sync two: %v", err)
	}

	// One host removed: the stale monitor must be pruned.
	one := []hostTarget{{host: "a.example.com", url: "https://a.example.com"}}
	if err := syncer.sync(context.Background(), route, "HTTPRoute", true, route.Annotations, one); err != nil {
		t.Fatalf("sync one: %v", err)
	}

	var monitors monitoringv1alpha1.MonitorList
	_ = cl.List(context.Background(), &monitors, client.InNamespace("default"))
	if len(monitors.Items) != 1 {
		t.Fatalf("want 1 monitor after host removal, got %d", len(monitors.Items))
	}
	if monitors.Items[0].Spec.URL != "https://a.example.com" {
		t.Errorf("surviving monitor url = %q", monitors.Items[0].Spec.URL)
	}
}
