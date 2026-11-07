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

// MonitorType enumerates the monitor types the v3 API supports. Each type
// requires a different subset of fields on create; see buildCreateBody.
type MonitorType string

const (
	TypeHTTP              MonitorType = "HTTP"
	TypeKeyword           MonitorType = "KEYWORD"
	TypePing              MonitorType = "PING"
	TypePort              MonitorType = "PORT"
	TypeHeartbeat         MonitorType = "HEARTBEAT"
	TypeDNS               MonitorType = "DNS"
	TypeAPI               MonitorType = "API"
	TypeUDP               MonitorType = "UDP"
	TypeVisualComparison  MonitorType = "VISUAL_COMPARISON"
	TypeThirdPartyService MonitorType = "THIRD_PARTY_SERVICE"
)

// AlertContact is an alert-contact assignment on a monitor. Threshold and
// recurrence are only honoured on paid plans (always 0 on Free).
type AlertContact struct {
	AlertContactID int64 `json:"alertContactId"`
	Threshold      int   `json:"threshold"`
	Recurrence     int   `json:"recurrence"`
}

// RegionData is the structured regional config (create uses `regionData`).
type RegionData struct {
	Regions        []string       `json:"REGION,omitempty"`
	ManualSelected *bool          `json:"MANUAL_SELECTED,omitempty"`
	Threshold      map[string]int `json:"THRESHOLD,omitempty"`
}

// APIAssertionCheck is one assertion within an API monitor's config.
type APIAssertionCheck struct {
	Property   string `json:"property"`
	Comparison string `json:"comparison"`
	Target     any    `json:"target,omitempty"`
}

// APIAssertions is the API-monitor assertion configuration.
type APIAssertions struct {
	Logic  string              `json:"logic"`
	Checks []APIAssertionCheck `json:"checks"`
}

// DNSRecords maps DNS record types to the values a DNS monitor should match.
type DNSRecords struct {
	A      []string `json:"A,omitempty"`
	AAAA   []string `json:"AAAA,omitempty"`
	CNAME  []string `json:"CNAME,omitempty"`
	MX     []string `json:"MX,omitempty"`
	NS     []string `json:"NS,omitempty"`
	TXT    []string `json:"TXT,omitempty"`
	SRV    []string `json:"SRV,omitempty"`
	PTR    []string `json:"PTR,omitempty"`
	SOA    []string `json:"SOA,omitempty"`
	SPF    []string `json:"SPF,omitempty"`
	DNSKEY []string `json:"DNSKEY,omitempty"`
	DS     []string `json:"DS,omitempty"`
	NSEC   []string `json:"NSEC,omitempty"`
	NSEC3  []string `json:"NSEC3,omitempty"`
}

// UDPConfig is the UDP-monitor configuration.
type UDPConfig struct {
	Payload             string `json:"payload,omitempty"`
	PacketLossThreshold int    `json:"packetLossThreshold"`
}

// AreaCoordinates is an optional region for visual-comparison monitors.
type AreaCoordinates struct {
	PercentX      float64 `json:"percentX"`
	PercentY      float64 `json:"percentY"`
	PercentWidth  float64 `json:"percentWidth"`
	PercentHeight float64 `json:"percentHeight"`
}

// VisualComparison is the visual-comparison monitor configuration.
type VisualComparison struct {
	SensitivityThreshold int              `json:"sensitivityThreshold"`
	Viewport             string           `json:"viewport"`
	AreaCoordinates      *AreaCoordinates `json:"areaCoordinates,omitempty"`
}

// ThirdPartyNotify tunes third-party-service notifications.
type ThirdPartyNotify struct {
	Severity           string `json:"severity"`
	IncrementalUpdates bool   `json:"incrementalUpdates"`
}

// ThirdPartyService is the third-party-service monitor configuration.
type ThirdPartyService struct {
	ProviderID          int64             `json:"providerId"`
	TrackedComponentIDs []int64           `json:"trackedComponentIds,omitempty"`
	Notify              *ThirdPartyNotify `json:"notify,omitempty"`
}

// Config is the per-type nested `config` object. Only the fields relevant to a
// monitor's type are serialized (buildCreateBody enforces this).
type Config struct {
	IPVersion               string             `json:"ipVersion,omitempty"`
	SSLExpirationPeriodDays []int              `json:"sslExpirationPeriodDays,omitempty"`
	ApplicationErrorRetries *int               `json:"applicationErrorRetries,omitempty"`
	DNSRecords              *DNSRecords        `json:"dnsRecords,omitempty"`
	APIAssertions           *APIAssertions     `json:"apiAssertions,omitempty"`
	UDP                     *UDPConfig         `json:"udp,omitempty"`
	VisualComparison        *VisualComparison  `json:"visualComparison,omitempty"`
	ThirdPartyService       *ThirdPartyService `json:"thirdPartyService,omitempty"`
}

// Incident is the last-incident summary returned on a monitor.
type Incident struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Cause     int    `json:"cause"`
	Reason    string `json:"reason"`
	StartedAt string `json:"startedAt"`
	Duration  int64  `json:"duration"`
}

// Monitor is the operator's domain representation of an UptimeRobot monitor.
// It is used by the Client interface in both directions: the HTTP client maps
// it to the correct per-type request body on write, and decodes API responses
// back into it on read.
type Monitor struct {
	// Identity / core.
	ID           int64
	FriendlyName string
	Type         MonitorType
	URL          string
	Interval     int
	Timeout      *int
	Paused       bool

	// HTTP family (HTTP, KEYWORD, API).
	HTTPMethodType           string
	AuthType                 string
	HTTPUsername             string
	HTTPPassword             string
	CustomHTTPHeaders        map[string]string
	SuccessHTTPResponseCodes []string
	PostValueData            string
	PostValueType            string
	FollowRedirections       *bool
	CheckSSLErrors           *bool
	SSLExpirationReminder    *bool
	DomainExpirationReminder *bool

	// Keyword.
	KeywordType     string
	KeywordValue    string
	KeywordCaseType string

	// Port / UDP.
	Port               *int
	PortAlertCondition string

	// Heartbeat.
	GracePeriod *int

	// Org / routing.
	TagNames              []string
	MaintenanceWindowIDs  []int64
	GroupID               int64
	CustomFields          map[string]string
	ResponseTimeThreshold *int
	Region                *RegionData
	AlertContacts         []AlertContact

	// Per-type nested config.
	Config *Config

	// Read-only fields populated from API responses.
	Status               string
	HeartbeatURL         string
	CurrentStateDuration int64
	CreateDateTime       string
	SSLBrand             string
	SSLExpiryDateTime    string
	DomainExpireDate     string
	LastIncident         *Incident
}

// ListOptions filters a ListMonitors call. Zero value lists the first page.
type ListOptions struct {
	// Limit caps the page size (API default applies when 0).
	Limit int
	// Cursor is the opaque pagination cursor from a previous page's nextLink.
	Cursor string
	// Search narrows results by friendly name or URL (server-side), when set.
	Search string
}

// UptimeStats is the per-monitor uptime statistics over a date range
// (GET /monitors/{id}/stats/uptime).
type UptimeStats struct {
	// UptimePercentage is in the range 0-100.
	UptimePercentage float64
	DowntimeSeconds  float64
	IncidentCount    int
	// MTBFSeconds is mean-time-between-failures; nil when no incidents occurred.
	MTBFSeconds *float64
	From        string
	To          string
}

// ResponseTimeStats is the per-monitor response-time summary
// (GET /monitors/{id}/stats/response-time).
type ResponseTimeStats struct {
	MinMS      *float64
	MaxMS      *float64
	AvgMS      *float64
	DataPoints int
	From       string
	To         string
}
