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

// Package resolver extracts the public hostname(s) to monitor from an exposed
// Kubernetes resource (HTTPRoute, Ingress, ...). Each source kind has its own
// Resolver implementation (the strategy pattern): adding support for a new kind
// is a matter of implementing Resolver and registering it.
package resolver

import (
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Resolver extracts hostnames from a source object of a particular kind.
type Resolver interface {
	// GroupKind is the source kind this resolver handles.
	GroupKind() schema.GroupKind
	// Hosts returns the distinct hostnames exposed by obj, sorted for stable
	// output. It returns an error if obj is not of the expected type.
	Hosts(obj client.Object) ([]string, error)
}

// Registry maps a GroupKind to its Resolver.
type Registry struct {
	byKind map[schema.GroupKind]Resolver
}

// NewRegistry builds a Registry from the given resolvers.
func NewRegistry(resolvers ...Resolver) *Registry {
	r := &Registry{byKind: make(map[schema.GroupKind]Resolver, len(resolvers))}
	for _, res := range resolvers {
		r.byKind[res.GroupKind()] = res
	}
	return r
}

// For returns the resolver registered for gk, or false.
func (r *Registry) For(gk schema.GroupKind) (Resolver, bool) {
	res, ok := r.byKind[gk]
	return res, ok
}

// dedupeSorted returns the distinct non-empty entries of in, sorted.
func dedupeSorted(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// typeErr is returned when a resolver receives the wrong object type.
func typeErr(want string, got client.Object) error {
	return fmt.Errorf("resolver: expected %s, got %T", want, got)
}
