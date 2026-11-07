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

package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MonitorType enumerates the supported UptimeRobot monitor types.
// +kubebuilder:validation:Enum=HTTP;KEYWORD;PING;PORT;HEARTBEAT;DNS;API;UDP;VISUAL_COMPARISON;THIRD_PARTY_SERVICE
type MonitorType string

const (
	MonitorHTTP              MonitorType = "HTTP"
	MonitorKeyword           MonitorType = "KEYWORD"
	MonitorPing              MonitorType = "PING"
	MonitorPort              MonitorType = "PORT"
	MonitorHeartbeat         MonitorType = "HEARTBEAT"
	MonitorDNS               MonitorType = "DNS"
	MonitorAPI               MonitorType = "API"
	MonitorUDP               MonitorType = "UDP"
	MonitorVisualComparison  MonitorType = "VISUAL_COMPARISON"
	MonitorThirdPartyService MonitorType = "THIRD_PARTY_SERVICE"
)

// SecretKeySelector references a key within a Secret in the Monitor's namespace.
type SecretKeySelector struct {
	// name is the name of the Secret.
	// +required
	Name string `json:"name"`
	// key is the key within the Secret holding the value.
	// +required
	Key string `json:"key"`
}

// AlertContactAssignment assigns an UptimeRobot alert contact to the monitor.
// Threshold and recurrence are only honoured on paid plans.
type AlertContactAssignment struct {
	// alertContactId is the numeric ID of a pre-existing UptimeRobot alert contact.
	// +required
	AlertContactID int64 `json:"alertContactId"`
	// threshold is the delay in minutes after the monitor is down before notifying.
	// +optional
	Threshold int `json:"threshold,omitempty"`
	// recurrence repeats notifications every N minutes (0 = no repeat).
	// +optional
	Recurrence int `json:"recurrence,omitempty"`
}

// RegionThreshold holds per-region response-time thresholds in milliseconds.
type RegionThreshold struct {
	// +optional
	NA int `json:"na,omitempty"`
	// +optional
	EU int `json:"eu,omitempty"`
	// +optional
	AS int `json:"as,omitempty"`
	// +optional
	OC int `json:"oc,omitempty"`
}

// RegionData configures which region(s) a monitor runs from.
type RegionData struct {
	// regions is the list of region codes (na, eu, as, oc). At least one when set.
	// +kubebuilder:validation:MinItems=1
	// +optional
	Regions []string `json:"regions,omitempty"`
	// manualSelected must be true to change the region on update.
	// +optional
	ManualSelected *bool `json:"manualSelected,omitempty"`
	// threshold sets per-region response-time thresholds (ms).
	// +optional
	Threshold *RegionThreshold `json:"threshold,omitempty"`
}

// HTTPSettings configures HTTP, KEYWORD and API monitors.
// +kubebuilder:validation:XValidation:rule="!(has(self.authType) && (self.authType == 'HTTP_BASIC' || self.authType == 'DIGEST' || self.authType == 'BEARER')) || has(self.passwordSecretRef)",message="passwordSecretRef is required when authType is HTTP_BASIC, DIGEST or BEARER (it holds the password for Basic/Digest, the token for Bearer)"
// +kubebuilder:validation:XValidation:rule="!(has(self.authType) && (self.authType == 'HTTP_BASIC' || self.authType == 'DIGEST')) || (has(self.username) && size(self.username) > 0)",message="username is required when authType is HTTP_BASIC or DIGEST"
type HTTPSettings struct {
	// method is the HTTP method used for the check.
	// +kubebuilder:validation:Enum=HEAD;GET;POST;PUT;PATCH;DELETE;OPTIONS;QUERY
	// +optional
	Method string `json:"method,omitempty"`
	// authType selects the HTTP authentication scheme.
	// +kubebuilder:validation:Enum=NONE;HTTP_BASIC;DIGEST;BEARER
	// +optional
	AuthType string `json:"authType,omitempty"`
	// username for HTTP authentication.
	// +optional
	Username string `json:"username,omitempty"`
	// passwordSecretRef references a Secret key holding the HTTP auth credential
	// (so it never lives in the Monitor spec in plaintext): the password for
	// HTTP_BASIC/DIGEST, or the token for BEARER.
	// +optional
	PasswordSecretRef *SecretKeySelector `json:"passwordSecretRef,omitempty"`
	// customHeaders are sent with the request (serialized JSON must be <3000 chars).
	// +optional
	CustomHeaders map[string]string `json:"customHeaders,omitempty"`
	// successHttpResponseCodes lists codes/ranges treated as up, e.g. ["2xx","3xx"].
	// +optional
	SuccessHTTPResponseCodes []string `json:"successHttpResponseCodes,omitempty"`
	// postValueData is the request payload (for non-HEAD methods).
	// +optional
	PostValueData string `json:"postValueData,omitempty"`
	// postValueType selects the payload encoding.
	// +kubebuilder:validation:Enum=KEY_VALUE;RAW_JSON
	// +optional
	PostValueType string `json:"postValueType,omitempty"`
	// followRedirections toggles following HTTP redirects.
	// +optional
	FollowRedirections *bool `json:"followRedirections,omitempty"`
	// checkSSLErrors enables SSL/domain expiration error checks.
	// +optional
	CheckSSLErrors *bool `json:"checkSSLErrors,omitempty"`
	// sslExpirationReminder enables SSL-expiry notifications.
	// +optional
	SSLExpirationReminder *bool `json:"sslExpirationReminder,omitempty"`
	// domainExpirationReminder enables domain-expiry notifications.
	// +optional
	DomainExpirationReminder *bool `json:"domainExpirationReminder,omitempty"`
}

// KeywordSettings configures KEYWORD monitors.
type KeywordSettings struct {
	// value is the keyword to search for in the response body.
	// +required
	Value string `json:"value"`
	// alertWhen controls whether to alert when the keyword exists or is absent.
	// +kubebuilder:validation:Enum=ALERT_EXISTS;ALERT_NOT_EXISTS
	// +required
	AlertWhen string `json:"alertWhen"`
	// caseSensitive toggles case-sensitive matching.
	// +optional
	CaseSensitive bool `json:"caseSensitive,omitempty"`
}

// APIAssertionCheck is a single assertion for an API monitor.
type APIAssertionCheck struct {
	// property is the assertion source (JSONPath, headers., status_code, or body).
	// +required
	Property string `json:"property"`
	// comparison is the operator applied to the property.
	// +required
	Comparison string `json:"comparison"`
	// target is the expected value (JSON). Omit for existence/null/empty checks.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Target *apiextensionsv1.JSON `json:"target,omitempty"`
}

// APIAssertions is the assertion configuration for an API monitor.
type APIAssertions struct {
	// logic combines multiple checks.
	// +kubebuilder:validation:Enum=AND;OR
	// +required
	Logic string `json:"logic"`
	// checks is the list of assertion checks (1-5).
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=5
	// +required
	Checks []APIAssertionCheck `json:"checks"`
}

// DNSRecords configures the records a DNS monitor resolves and matches.
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

// UDPConfig configures UDP monitors.
type UDPConfig struct {
	// payload to send in the UDP packet.
	// +optional
	Payload string `json:"payload,omitempty"`
	// packetLossThreshold is the packet-loss percentage that triggers an alert.
	// +required
	PacketLossThreshold int `json:"packetLossThreshold"`
}

// AreaCoordinates is an optional region (percent-based) for visual comparison.
type AreaCoordinates struct {
	PercentX      int `json:"percentX"`
	PercentY      int `json:"percentY"`
	PercentWidth  int `json:"percentWidth"`
	PercentHeight int `json:"percentHeight"`
}

// VisualComparison configures VISUAL_COMPARISON monitors.
type VisualComparison struct {
	// sensitivityThreshold is the difference threshold (0-100).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +required
	SensitivityThreshold int `json:"sensitivityThreshold"`
	// viewport selects the capture viewport.
	// +kubebuilder:validation:Enum=desktop;mobile
	// +required
	Viewport string `json:"viewport"`
	// areaCoordinates optionally limits comparison to a sub-region.
	// +optional
	AreaCoordinates *AreaCoordinates `json:"areaCoordinates,omitempty"`
}

// ThirdPartyNotify tunes notifications for THIRD_PARTY_SERVICE monitors.
type ThirdPartyNotify struct {
	// +kubebuilder:validation:Enum=all;major;none
	// +optional
	Severity string `json:"severity,omitempty"`
	// +optional
	IncrementalUpdates bool `json:"incrementalUpdates,omitempty"`
}

// ThirdPartyService configures THIRD_PARTY_SERVICE monitors.
type ThirdPartyService struct {
	// providerId is the isdown provider ID to track.
	// +required
	ProviderID int64 `json:"providerId"`
	// trackedComponentIds scopes tracking to specific components.
	// +optional
	TrackedComponentIDs []int64 `json:"trackedComponentIds,omitempty"`
	// notify tunes notifications.
	// +optional
	Notify *ThirdPartyNotify `json:"notify,omitempty"`
}

// MonitorConfig holds the per-type nested configuration. Only the block matching
// the monitor's type is used; the others are ignored (and validated out by CEL).
type MonitorConfig struct {
	// ipVersion selects the IP version preference.
	// +kubebuilder:validation:Enum=ipv4Only;ipv6Only
	// +optional
	IPVersion string `json:"ipVersion,omitempty"`
	// sslExpirationPeriodDays sets reminder windows in days (0-365, max 10).
	// +kubebuilder:validation:MaxItems=10
	// +optional
	SSLExpirationPeriodDays []int `json:"sslExpirationPeriodDays,omitempty"`
	// applicationErrorRetries is the number of retries on application errors (0-3).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3
	// +optional
	ApplicationErrorRetries *int `json:"applicationErrorRetries,omitempty"`
	// dnsRecords configures a DNS monitor.
	// +optional
	DNSRecords *DNSRecords `json:"dnsRecords,omitempty"`
	// apiAssertions configures an API monitor.
	// +optional
	APIAssertions *APIAssertions `json:"apiAssertions,omitempty"`
	// udp configures a UDP monitor.
	// +optional
	UDP *UDPConfig `json:"udp,omitempty"`
	// visualComparison configures a VISUAL_COMPARISON monitor.
	// +optional
	VisualComparison *VisualComparison `json:"visualComparison,omitempty"`
	// thirdPartyService configures a THIRD_PARTY_SERVICE monitor.
	// +optional
	ThirdPartyService *ThirdPartyService `json:"thirdPartyService,omitempty"`
}

// MonitorSpec defines the desired state of a Monitor.
//
// The schema is type-discriminated: each monitor type requires a different set
// of fields. The CEL rules below enforce the per-type requirements that the
// UptimeRobot API would otherwise reject, and keep `type` immutable.
//
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="type is immutable"
// +kubebuilder:validation:XValidation:rule="self.type in ['HEARTBEAT','THIRD_PARTY_SERVICE'] || (has(self.url) && size(self.url) > 0)",message="url is required for this monitor type"
// +kubebuilder:validation:XValidation:rule="self.type != 'KEYWORD' || has(self.keyword)",message="keyword settings are required for KEYWORD monitors"
// +kubebuilder:validation:XValidation:rule="!(self.type in ['PORT','UDP']) || has(self.port)",message="port is required for PORT and UDP monitors"
// +kubebuilder:validation:XValidation:rule="self.type != 'API' || (has(self.config) && has(self.config.apiAssertions))",message="config.apiAssertions is required for API monitors"
// +kubebuilder:validation:XValidation:rule="self.type != 'DNS' || (has(self.config) && has(self.config.dnsRecords))",message="config.dnsRecords is required for DNS monitors"
// +kubebuilder:validation:XValidation:rule="self.type != 'UDP' || (has(self.config) && has(self.config.udp))",message="config.udp is required for UDP monitors"
// +kubebuilder:validation:XValidation:rule="self.type != 'VISUAL_COMPARISON' || (has(self.config) && has(self.config.visualComparison))",message="config.visualComparison is required for VISUAL_COMPARISON monitors"
// +kubebuilder:validation:XValidation:rule="self.type != 'THIRD_PARTY_SERVICE' || (has(self.config) && has(self.config.thirdPartyService))",message="config.thirdPartyService is required for THIRD_PARTY_SERVICE monitors"
type MonitorSpec struct {
	// type is the UptimeRobot monitor type. Immutable after creation.
	// +kubebuilder:default=HTTP
	// +optional
	Type MonitorType `json:"type,omitempty"`

	// friendlyName is the display name in UptimeRobot. Defaults to
	// "<namespace>/<name>" when empty.
	// +kubebuilder:validation:MaxLength=250
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`

	// url is the target to monitor. Required for all types except HEARTBEAT and
	// THIRD_PARTY_SERVICE.
	// +kubebuilder:validation:MaxLength=10000
	// +optional
	URL string `json:"url,omitempty"`

	// interval is the check interval in seconds (minimum 15).
	// +kubebuilder:validation:Minimum=15
	// +kubebuilder:default=300
	// +optional
	Interval int `json:"interval,omitempty"`

	// timeout is the check timeout in seconds (0-60). Required by the API for
	// all check types (ignored for HEARTBEAT / THIRD_PARTY_SERVICE).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=60
	// +kubebuilder:default=30
	// +optional
	Timeout *int `json:"timeout,omitempty"`

	// paused, when true, pauses monitoring (maps to the Pause API); false resumes it.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// tagNames assigns tags to the monitor.
	// +optional
	TagNames []string `json:"tagNames,omitempty"`

	// maintenanceWindowIds assigns maintenance windows by ID.
	// +optional
	MaintenanceWindowIDs []int64 `json:"maintenanceWindowIds,omitempty"`

	// groupId assigns the monitor to a monitor group (0 = none).
	// +optional
	GroupID int64 `json:"groupId,omitempty"`

	// customFields are key-value metadata (paid plans only; max 20 keys).
	// +optional
	CustomFields map[string]string `json:"customFields,omitempty"`

	// responseTimeThreshold applies a response-time threshold (ms) to all regions.
	// +optional
	ResponseTimeThreshold *int `json:"responseTimeThreshold,omitempty"`

	// region configures which region(s) the monitor runs from.
	// +optional
	Region *RegionData `json:"region,omitempty"`

	// alertContacts overrides the operator-wide default alert contacts for this
	// monitor. When empty, the operator injects its configured global default.
	// +optional
	AlertContacts []AlertContactAssignment `json:"alertContacts,omitempty"`

	// http configures HTTP, KEYWORD and API monitors.
	// +optional
	HTTP *HTTPSettings `json:"http,omitempty"`

	// keyword configures KEYWORD monitors.
	// +optional
	Keyword *KeywordSettings `json:"keyword,omitempty"`

	// port is the port to check (required for PORT and UDP monitors).
	// +optional
	Port *int `json:"port,omitempty"`

	// portAlertCondition controls when a PORT monitor alerts.
	// +kubebuilder:validation:Enum=CLOSED;OPEN
	// +optional
	PortAlertCondition string `json:"portAlertCondition,omitempty"`

	// gracePeriod is the HEARTBEAT grace period in seconds before alerting.
	// +optional
	GracePeriod *int `json:"gracePeriod,omitempty"`

	// config holds type-specific nested configuration.
	// +optional
	Config *MonitorConfig `json:"config,omitempty"`

	// collectStats enables periodic polling of uptime/response-time statistics
	// into .status. Off by default to conserve the API rate limit.
	// +optional
	CollectStats bool `json:"collectStats,omitempty"`

	// statsRefreshInterval controls how often stats are polled when
	// collectStats is enabled. Defaults to 15m.
	// +optional
	StatsRefreshInterval *metav1.Duration `json:"statsRefreshInterval,omitempty"`
}

// IncidentStatus is the last incident summary reflected from UptimeRobot.
type IncidentStatus struct {
	ID        string `json:"id,omitempty"`
	Status    string `json:"status,omitempty"`
	Cause     int    `json:"cause,omitempty"`
	Reason    string `json:"reason,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	Duration  int64  `json:"duration,omitempty"`
}

// UptimeStatus holds polled uptime statistics.
type UptimeStatus struct {
	// ratio is the uptime percentage (0-100) over the polled window.
	// +optional
	Ratio string `json:"ratio,omitempty"`
	// downtimeSeconds is total downtime over the window.
	// +optional
	DowntimeSeconds int64 `json:"downtimeSeconds,omitempty"`
	// incidentCount is the number of incidents in the window.
	// +optional
	IncidentCount int `json:"incidentCount,omitempty"`
	// from/to describe the polled window (ISO 8601).
	// +optional
	From string `json:"from,omitempty"`
	// +optional
	To string `json:"to,omitempty"`
}

// ResponseTimeStatus holds polled response-time statistics (milliseconds).
type ResponseTimeStatus struct {
	// +optional
	AverageMS int `json:"averageMs,omitempty"`
	// +optional
	MinMS int `json:"minMs,omitempty"`
	// +optional
	MaxMS int `json:"maxMs,omitempty"`
}

// MonitorStatus defines the observed state of a Monitor.
type MonitorStatus struct {
	// monitorID is the UptimeRobot monitor ID once created.
	// +optional
	MonitorID string `json:"monitorID,omitempty"`

	// observedURL is the URL the operator last reconciled.
	// +optional
	ObservedURL string `json:"observedURL,omitempty"`

	// heartbeatURL is the push URL for HEARTBEAT monitors.
	// +optional
	HeartbeatURL string `json:"heartbeatUrl,omitempty"`

	// monitorStatus is the live status reported by UptimeRobot (UP/DOWN/PAUSED...).
	// +optional
	MonitorStatus string `json:"monitorStatus,omitempty"`

	// state is the reconcile state: Pending, Active or Error.
	// +optional
	State string `json:"state,omitempty"`

	// currentStateDuration is how long (seconds) the monitor has held its status.
	// +optional
	CurrentStateDuration int64 `json:"currentStateDuration,omitempty"`

	// createDateTime is when the monitor was created in UptimeRobot.
	// +optional
	CreateDateTime string `json:"createDateTime,omitempty"`

	// sslBrand / sslExpiryDateTime / domainExpireDate reflect SSL/domain metadata.
	// +optional
	SSLBrand string `json:"sslBrand,omitempty"`
	// +optional
	SSLExpiryDateTime string `json:"sslExpiryDateTime,omitempty"`
	// +optional
	DomainExpireDate string `json:"domainExpireDate,omitempty"`

	// lastIncident is the most recent incident summary.
	// +optional
	LastIncident *IncidentStatus `json:"lastIncident,omitempty"`

	// uptime holds polled uptime statistics (when collectStats is enabled).
	// +optional
	Uptime *UptimeStatus `json:"uptime,omitempty"`

	// responseTime holds polled response-time statistics (when collectStats is enabled).
	// +optional
	ResponseTime *ResponseTimeStatus `json:"responseTime,omitempty"`

	// lastStatsSync is when statistics were last polled.
	// +optional
	LastStatsSync *metav1.Time `json:"lastStatsSync,omitempty"`

	// lastResetStatsToken records the last applied reset-stats annotation value
	// so the one-shot reset action fires only when the annotation changes.
	// +optional
	LastResetStatsToken string `json:"lastResetStatsToken,omitempty"`

	// observedGeneration is the last reconciled .metadata.generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// observedConfigHash is a hash of the last config pushed to UptimeRobot,
	// including resolved Secret values. A credential rotation changes this hash
	// without bumping generation, which is what triggers a re-push.
	// +optional
	ObservedConfigHash string `json:"observedConfigHash,omitempty"`
	// conditions represent the current state of the Monitor resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=mon
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.observedURL`
// +kubebuilder:printcolumn:name="Monitor-ID",type=string,JSONPath=`.status.monitorID`
// +kubebuilder:printcolumn:name="Monitor-Status",type=string,JSONPath=`.status.monitorStatus`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Monitor is the Schema for the monitors API.
type Monitor struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of Monitor
	// +required
	Spec MonitorSpec `json:"spec"`

	// status defines the observed state of Monitor
	// +optional
	Status MonitorStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MonitorList contains a list of Monitor.
type MonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Monitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Monitor{}, &MonitorList{})
}
