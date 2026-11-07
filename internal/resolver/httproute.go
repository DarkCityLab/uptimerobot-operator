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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// HTTPRoute is the resolver for Gateway API HTTPRoute objects. It returns the
// route's spec.hostnames.
type HTTPRoute struct{}

// GroupKind implements Resolver.
func (HTTPRoute) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: gatewayv1.GroupName, Kind: "HTTPRoute"}
}

// Hosts implements Resolver.
func (HTTPRoute) Hosts(obj client.Object) ([]string, error) {
	route, ok := obj.(*gatewayv1.HTTPRoute)
	if !ok {
		return nil, typeErr("*gatewayv1.HTTPRoute", obj)
	}
	hosts := make([]string, 0, len(route.Spec.Hostnames))
	for _, h := range route.Spec.Hostnames {
		hosts = append(hosts, string(h))
	}
	return dedupeSorted(hosts), nil
}
