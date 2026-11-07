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
	"slices"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Ingress is the resolver for networking.k8s.io/v1 Ingress objects. It returns
// the hosts from spec.rules and spec.tls.
type Ingress struct{}

// GroupKind implements Resolver.
func (Ingress) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: networkingv1.GroupName, Kind: "Ingress"}
}

// Hosts implements Resolver.
func (Ingress) Hosts(obj client.Object) ([]string, error) {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil, typeErr("*networkingv1.Ingress", obj)
	}
	var hosts []string
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	for _, tls := range ing.Spec.TLS {
		hosts = append(hosts, tls.Hosts...)
	}
	return dedupeSorted(hosts), nil
}

// IsTLSHost reports whether host is covered by one of the Ingress's TLS entries.
// The source watcher uses this to default the URL scheme to https for TLS hosts.
func IsTLSHost(ing *networkingv1.Ingress, host string) bool {
	for _, tls := range ing.Spec.TLS {
		if slices.Contains(tls.Hosts, host) {
			return true
		}
	}
	return false
}
