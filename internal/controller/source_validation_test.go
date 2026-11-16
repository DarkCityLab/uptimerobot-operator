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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/annotations"
)

// An enabled HTTPRoute carrying an invalid annotation must surface a Warning
// event and create no Monitor (instead of silently dropping the bad value).
func TestHTTPRouteRejectsInvalidAnnotations(t *testing.T) {
	scheme := newScheme(t)
	route := newRoute()
	route.Annotations[annotations.Interval] = "abc" // not an integer

	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	rec := record.NewFakeRecorder(10)
	r := &HTTPRouteReconciler{Client: cl, Scheme: scheme, Recorder: rec}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: route.Name, Namespace: route.Namespace},
	})
	if err != nil {
		t.Fatalf("Reconcile returned error (want nil, no requeue): %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Errorf("expected no requeue on user error, got %+v", res)
	}

	var monitors monitoringv1alpha1.MonitorList
	if err := cl.List(context.Background(), &monitors); err != nil {
		t.Fatalf("list monitors: %v", err)
	}
	if len(monitors.Items) != 0 {
		t.Errorf("expected 0 monitors from invalid annotations, got %d", len(monitors.Items))
	}

	select {
	case ev := <-rec.Events:
		if !strings.Contains(ev, "InvalidAnnotation") || !strings.Contains(ev, annotations.Interval) {
			t.Errorf("event %q should be an InvalidAnnotation naming %s", ev, annotations.Interval)
		}
	default:
		t.Error("expected an InvalidAnnotation Warning event, got none")
	}
}

// A source being deleted must NOT run annotation validation: deletion proceeds
// (and owned Monitors are cleaned up) even if the annotations are now invalid.
func TestHTTPRouteDeletionSkipsValidation(t *testing.T) {
	scheme := newScheme(t)
	route := newRoute()
	route.Annotations[annotations.Interval] = "abc" // invalid — but we're deleting
	now := metav1.Now()
	route.DeletionTimestamp = &now
	route.Finalizers = []string{"test/hold"} // fake client retains a deleting object only with a finalizer

	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(route).Build()
	rec := record.NewFakeRecorder(10)
	r := &HTTPRouteReconciler{Client: cl, Scheme: scheme, Recorder: rec}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: route.Name, Namespace: route.Namespace},
	}); err != nil {
		t.Fatalf("Reconcile on a deleting route returned error: %v", err)
	}

	select {
	case ev := <-rec.Events:
		t.Errorf("deletion must skip annotation validation, but got event: %q", ev)
	default:
		// expected: no InvalidAnnotation event on the delete path
	}
}
