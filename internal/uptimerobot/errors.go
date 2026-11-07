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

// Package uptimerobot is a transport-only client for the UptimeRobot v3 API.
//
// It deliberately has no dependency on controller-runtime or Kubernetes types so
// that it can be unit-tested in isolation and reused outside the operator. The
// Client interface is the seam controllers depend on; HTTPClient is the real
// implementation and fake.Client is an in-memory double for tests.
package uptimerobot

import (
	"errors"
	"fmt"
	"time"
)

// Stable API error codes worth branching on. The full table lives in the API
// docs; a code never changes meaning, but new codes may appear — always fall
// back to the HTTP status for a code we do not recognise.
const (
	CodeFeatureNotInPlan  = "000-003" // 403
	CodeNotFound          = "000-004" // 404
	CodeInvalidKey        = "000-005" // 401
	CodeForbidden         = "000-006" // 403
	CodeUnprocessable     = "000-009" // 422
	CodeBadRequest        = "000-012" // 400
	CodeServerError       = "000-013" // 500/413/415
	CodeRateLimited       = "000-014" // 429
	CodeConflict          = "000-016" // 409
	CodeReadOnlyKey       = "003-001" // 401
	CodeKeyMissing        = "003-005" // 401
	CodeMonitorURLBlocked = "009-001" // 403
	CodeIntervalTooShort  = "009-002" // 403
	CodeMonitorLimit      = "009-003" // 403
	CodeInvalidURLForType = "009-010" // 400
	CodeDuplicateMonitor  = "009-011" // 409
	CodeTypeImmutable     = "009-015" // 400
)

// APIError is a structured error returned by the UptimeRobot API. Callers should
// branch on Code (and fall back to HTTPStatus) rather than on Message text.
type APIError struct {
	// Code is the stable error code, e.g. "009-011". May be empty for transport
	// errors or responses without a code, in which case branch on HTTPStatus.
	Code string
	// HTTPStatus is the HTTP status code of the response.
	HTTPStatus int
	// Message is the human-readable message (may be a single string or joined list).
	Message string
	// RetryAfter is populated for 429 responses from the Retry-After header.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("uptimerobot: %s (code=%s, http=%d)", e.Message, e.Code, e.HTTPStatus)
	}
	return fmt.Sprintf("uptimerobot: %s (http=%d)", e.Message, e.HTTPStatus)
}

// Sentinel errors for ergonomic errors.Is checks. The concrete error is always
// an *APIError; these allow `errors.Is(err, ErrNotFound)` style branching.
var (
	ErrNotFound     = errors.New("uptimerobot: resource not found")
	ErrDuplicate    = errors.New("uptimerobot: monitor already exists")
	ErrRateLimited  = errors.New("uptimerobot: rate limited")
	ErrUnauthorized = errors.New("uptimerobot: unauthorized")
	ErrForbidden    = errors.New("uptimerobot: forbidden")
)

// Is lets *APIError satisfy errors.Is against the sentinels above based on the
// stable code (preferred) or the HTTP status (fallback for unknown codes).
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Code == CodeNotFound || (e.Code == "" && e.HTTPStatus == 404)
	case ErrDuplicate:
		return e.Code == CodeDuplicateMonitor
	case ErrRateLimited:
		return e.Code == CodeRateLimited || (e.Code == "" && e.HTTPStatus == 429)
	case ErrUnauthorized:
		return e.Code == CodeInvalidKey || e.Code == CodeKeyMissing || e.Code == CodeReadOnlyKey ||
			(e.Code == "" && e.HTTPStatus == 401)
	case ErrForbidden:
		return e.Code == CodeForbidden || e.Code == CodeFeatureNotInPlan ||
			(e.Code == "" && e.HTTPStatus == 403)
	}
	return false
}

// IsTerminal reports whether the error represents a condition that will not be
// resolved by retrying the same request unchanged (e.g. validation errors,
// plan limits). Such errors should surface as a Ready=False condition rather
// than being requeued indefinitely. Transient errors (5xx, 429) return false.
func (e *APIError) IsTerminal() bool {
	switch e.Code {
	case CodeServerError: // 5xx family — may succeed on retry
		return false
	case CodeRateLimited:
		return false
	}
	switch e.HTTPStatus {
	case 429, 500, 502, 503, 504:
		return false
	}
	// 4xx other than 429 are client errors that won't fix themselves.
	return e.HTTPStatus >= 400 && e.HTTPStatus < 500
}

// asAPIError extracts an *APIError from err, or returns nil.
func asAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}

// IsNotFound reports whether err is an UptimeRobot not-found error.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsDuplicate reports whether err indicates a duplicate monitor (009-011).
func IsDuplicate(err error) bool { return errors.Is(err, ErrDuplicate) }

// IsRateLimited reports whether err is a rate-limit error (429).
func IsRateLimited(err error) bool { return errors.Is(err, ErrRateLimited) }

// RetryAfter returns the Retry-After delay for a rate-limit error, or 0.
func RetryAfter(err error) time.Duration {
	if apiErr := asAPIError(err); apiErr != nil {
		return apiErr.RetryAfter
	}
	return 0
}

// IsTerminal reports whether err is a terminal (non-retryable) API error.
func IsTerminal(err error) bool {
	if apiErr := asAPIError(err); apiErr != nil {
		return apiErr.IsTerminal()
	}
	return false
}
