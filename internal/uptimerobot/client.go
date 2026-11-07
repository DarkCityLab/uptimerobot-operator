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

package uptimerobot

import "context"

// Client is the seam the operator depends on for talking to UptimeRobot. The
// real implementation is *HTTPClient; tests use fake.Client. Every method takes
// a context so callers control cancellation and timeouts.
//
// Errors are always *APIError (use the errors.Is helpers: IsNotFound,
// IsDuplicate, IsRateLimited, IsTerminal).
type Client interface {
	// CreateMonitor creates a monitor and returns the created object (with ID).
	// The request body shape is derived from m.Type.
	CreateMonitor(ctx context.Context, m *Monitor) (*Monitor, error)

	// GetMonitor fetches a monitor by ID.
	GetMonitor(ctx context.Context, id int64) (*Monitor, error)

	// ListMonitors lists monitors matching opts. Used to adopt an existing
	// monitor by URL/friendly name when a create returns a duplicate.
	ListMonitors(ctx context.Context, opts ListOptions) ([]Monitor, error)

	// UpdateMonitor patches the mutable fields of an existing monitor. The
	// monitor type is never sent (it is immutable server-side).
	UpdateMonitor(ctx context.Context, id int64, m *Monitor) (*Monitor, error)

	// DeleteMonitor deletes a monitor by ID. A not-found result is returned as
	// an error satisfying errors.Is(err, ErrNotFound); callers typically treat
	// that as success during finalization.
	DeleteMonitor(ctx context.Context, id int64) error

	// PauseMonitor pauses monitoring.
	PauseMonitor(ctx context.Context, id int64) error

	// StartMonitor resumes monitoring.
	StartMonitor(ctx context.Context, id int64) error

	// ResetStats clears the accumulated statistics for a monitor.
	ResetStats(ctx context.Context, id int64) error

	// UptimeStats returns uptime statistics for the monitor over [from, to]
	// (ISO 8601; empty strings use the API defaults — last 24h).
	UptimeStats(ctx context.Context, id int64, from, to string) (*UptimeStats, error)

	// ResponseTimeStats returns the response-time summary for the monitor over
	// [from, to] (ISO 8601; empty strings use the API defaults — last 24h).
	ResponseTimeStats(ctx context.Context, id int64, from, to string) (*ResponseTimeStats, error)
}
