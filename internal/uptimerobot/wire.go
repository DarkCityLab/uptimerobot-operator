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

// This file isolates the wire format (JSON) from the domain Monitor type. The
// create/update body uses different field names than the GET response (e.g.
// create's `tagNames`/`regionData`/`assignedAlertContacts` vs GET's
// `tags`/`regionalData`), so request and response have separate structs.

// monitorRequestBody is the create/update request body. Every field is
// omitempty so buildRequestBody can emit exactly the per-type subset.
type monitorRequestBody struct {
	FriendlyName string `json:"friendlyName,omitempty"`
	Type         string `json:"type,omitempty"`
	URL          string `json:"url,omitempty"`
	Interval     int    `json:"interval,omitempty"`
	Timeout      *int   `json:"timeout,omitempty"`

	HTTPMethodType           string            `json:"httpMethodType,omitempty"`
	AuthType                 string            `json:"authType,omitempty"`
	HTTPUsername             string            `json:"httpUsername,omitempty"`
	HTTPPassword             string            `json:"httpPassword,omitempty"`
	CustomHTTPHeaders        map[string]string `json:"customHttpHeaders,omitempty"`
	SuccessHTTPResponseCodes []string          `json:"successHttpResponseCodes,omitempty"`
	PostValueData            string            `json:"postValueData,omitempty"`
	PostValueType            string            `json:"postValueType,omitempty"`
	FollowRedirections       *bool             `json:"followRedirections,omitempty"`
	CheckSSLErrors           *bool             `json:"checkSSLErrors,omitempty"`
	SSLExpirationReminder    *bool             `json:"sslExpirationReminder,omitempty"`
	DomainExpirationReminder *bool             `json:"domainExpirationReminder,omitempty"`

	KeywordType     string `json:"keywordType,omitempty"`
	KeywordValue    string `json:"keywordValue,omitempty"`
	KeywordCaseType string `json:"keywordCaseType,omitempty"`

	Port               *int   `json:"port,omitempty"`
	PortAlertCondition string `json:"portAlertCondition,omitempty"`

	GracePeriod *int `json:"gracePeriod,omitempty"`

	TagNames              []string          `json:"tagNames,omitempty"`
	MaintenanceWindowIDs  []int64           `json:"maintenanceWindowsIds,omitempty"`
	GroupID               int64             `json:"groupId,omitempty"`
	CustomFields          map[string]string `json:"customFields,omitempty"`
	ResponseTimeThreshold *int              `json:"responseTimeThreshold,omitempty"`
	RegionData            *RegionData       `json:"regionData,omitempty"`
	AssignedAlertContacts []AlertContact    `json:"assignedAlertContacts,omitempty"`

	Config *Config `json:"config,omitempty"`
}

// buildRequestBody assembles the exact oneOf shape for m.Type, placing only the
// fields that type's schema accepts. includeType controls whether the immutable
// `type` field is emitted (true for create, false for update/patch).
func buildRequestBody(m *Monitor, includeType bool) *monitorRequestBody {
	b := &monitorRequestBody{
		FriendlyName: m.FriendlyName,
		Interval:     m.Interval,
	}
	if includeType {
		b.Type = string(m.Type)
	}

	// Organisational fields accepted by every type.
	b.AssignedAlertContacts = m.AlertContacts
	b.TagNames = m.TagNames
	b.MaintenanceWindowIDs = m.MaintenanceWindowIDs
	b.GroupID = m.GroupID
	b.CustomFields = m.CustomFields
	b.RegionData = m.Region
	// Config is passed through; its own omitempty fields mean only the keys the
	// caller populated (per type) are serialized.
	b.Config = m.Config

	// httpFamily fills the HTTP/KEYWORD/API request fields.
	httpFamily := func() {
		b.HTTPMethodType = m.HTTPMethodType
		b.AuthType = m.AuthType
		b.HTTPUsername = m.HTTPUsername
		b.HTTPPassword = m.HTTPPassword
		b.CustomHTTPHeaders = m.CustomHTTPHeaders
		b.SuccessHTTPResponseCodes = m.SuccessHTTPResponseCodes
		b.PostValueData = m.PostValueData
		b.PostValueType = m.PostValueType
		b.FollowRedirections = m.FollowRedirections
		b.CheckSSLErrors = m.CheckSSLErrors
		b.SSLExpirationReminder = m.SSLExpirationReminder
		b.DomainExpirationReminder = m.DomainExpirationReminder
	}

	switch m.Type {
	case TypeHTTP:
		b.URL = m.URL
		b.Timeout = m.Timeout
		b.ResponseTimeThreshold = m.ResponseTimeThreshold
		httpFamily()
	case TypeKeyword:
		b.URL = m.URL
		b.Timeout = m.Timeout
		b.ResponseTimeThreshold = m.ResponseTimeThreshold
		httpFamily()
		b.KeywordType = m.KeywordType
		b.KeywordValue = m.KeywordValue
		b.KeywordCaseType = m.KeywordCaseType
	case TypeAPI:
		b.URL = m.URL
		b.Timeout = m.Timeout
		b.ResponseTimeThreshold = m.ResponseTimeThreshold
		httpFamily()
	case TypePing:
		b.URL = m.URL
		b.Timeout = m.Timeout
		b.ResponseTimeThreshold = m.ResponseTimeThreshold
	case TypePort:
		b.URL = m.URL
		b.Timeout = m.Timeout
		b.Port = m.Port
		b.PortAlertCondition = m.PortAlertCondition
		b.ResponseTimeThreshold = m.ResponseTimeThreshold
	case TypeUDP:
		b.URL = m.URL
		b.Timeout = m.Timeout
		b.Port = m.Port
	case TypeDNS:
		b.URL = m.URL
		b.Timeout = m.Timeout
	case TypeVisualComparison:
		b.URL = m.URL
	case TypeHeartbeat:
		b.GracePeriod = m.GracePeriod
	case TypeThirdPartyService:
		// Only type, config.thirdPartyService and alert contacts apply.
	default:
		// Unknown type: send a conservative body (url + timeout) and let the
		// server validate.
		b.URL = m.URL
		b.Timeout = m.Timeout
	}
	return b
}

// ---- response wire types ---------------------------------------------------

// monitorResponse mirrors the GET/POST monitor object. Nullable string fields
// decode to "" and nullable numbers to their zero value, which is fine for our
// status mapping.
type monitorResponse struct {
	ID                   int64     `json:"id"`
	FriendlyName         string    `json:"friendlyName"`
	Type                 string    `json:"type"`
	URL                  string    `json:"url"`
	Interval             int       `json:"interval"`
	Timeout              *int      `json:"timeout"`
	Status               string    `json:"status"`
	CurrentStateDuration int64     `json:"currentStateDuration"`
	CreateDateTime       string    `json:"createDateTime"`
	SSLBrand             string    `json:"sslBrand"`
	SSLExpiryDateTime    string    `json:"sslExpiryDateTime"`
	DomainExpireDate     string    `json:"domainExpireDate"`
	HeartbeatURL         string    `json:"heartbeatUrl"`
	HTTPMethodType       string    `json:"httpMethodType"`
	GroupID              int64     `json:"groupId"`
	LastIncident         *Incident `json:"lastIncident"`
}

func (r *monitorResponse) toMonitor() *Monitor {
	return &Monitor{
		ID:                   r.ID,
		FriendlyName:         r.FriendlyName,
		Type:                 MonitorType(r.Type),
		URL:                  r.URL,
		Interval:             r.Interval,
		Timeout:              r.Timeout,
		Status:               r.Status,
		CurrentStateDuration: r.CurrentStateDuration,
		CreateDateTime:       r.CreateDateTime,
		SSLBrand:             r.SSLBrand,
		SSLExpiryDateTime:    r.SSLExpiryDateTime,
		DomainExpireDate:     r.DomainExpireDate,
		HeartbeatURL:         r.HeartbeatURL,
		HTTPMethodType:       r.HTTPMethodType,
		GroupID:              r.GroupID,
		LastIncident:         r.LastIncident,
	}
}

type monitorListResponse struct {
	NextLink string            `json:"nextLink"`
	Data     []monitorResponse `json:"data"`
}

type monitorUptimeStatsResponse struct {
	Uptime               float64  `json:"uptime"`
	TotalDowntimeSeconds float64  `json:"total_downtime_seconds"`
	IncidentCount        int      `json:"incident_count"`
	MTBF                 *float64 `json:"mtbf"`
	From                 string   `json:"from"`
	To                   string   `json:"to"`
}

type monitorResponseTimeStatsResponse struct {
	Summary struct {
		Min *float64 `json:"min"`
		Max *float64 `json:"max"`
		Avg *float64 `json:"avg"`
	} `json:"summary"`
	DataPoints int    `json:"data_points"`
	From       string `json:"from"`
	To         string `json:"to"`
}
