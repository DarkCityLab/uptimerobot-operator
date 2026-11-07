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

package resolver

import (
	"reflect"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestHTTPRouteHosts(t *testing.T) {
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "httpbin", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"b.example.com", "a.example.com", "b.example.com"},
		},
	}
	got, err := HTTPRoute{}.Hosts(route)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a.example.com", "b.example.com"} // deduped + sorted
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
}

func TestHTTPRouteWrongType(t *testing.T) {
	if _, err := (HTTPRoute{}).Hosts(&networkingv1.Ingress{}); err == nil {
		t.Error("want type error, got nil")
	}
}

func TestIngressHostsAndTLS(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "shop.example.com"},
				{Host: "api.example.com"},
			},
			TLS: []networkingv1.IngressTLS{
				{Hosts: []string{"shop.example.com"}},
			},
		},
	}
	got, err := Ingress{}.Hosts(ing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"api.example.com", "shop.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}

	if !IsTLSHost(ing, "shop.example.com") {
		t.Error("shop.example.com should be a TLS host")
	}
	if IsTLSHost(ing, "api.example.com") {
		t.Error("api.example.com should not be a TLS host")
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry(HTTPRoute{}, Ingress{})
	if _, ok := reg.For((HTTPRoute{}).GroupKind()); !ok {
		t.Error("HTTPRoute resolver not found in registry")
	}
	if _, ok := reg.For((Ingress{}).GroupKind()); !ok {
		t.Error("Ingress resolver not found in registry")
	}
}
