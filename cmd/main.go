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

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/apikey"
	"github.com/DarkCityLab/uptimerobot-operator/internal/controller"
	ur "github.com/DarkCityLab/uptimerobot-operator/internal/uptimerobot"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(monitoringv1alpha1.AddToScheme(scheme))
	// Install is the current Gateway API registration entrypoint (AddToScheme is
	// deprecated). Registering the scheme is always safe even when the Gateway
	// API CRDs are not installed in the cluster; only watching a missing type
	// would fail, which the RESTMapper guard below prevents.
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)

	// UptimeRobot operator flags.
	var apiKeyFile string
	var baseURL string
	var requestsPerMinute int
	var defaultAlertContactsRaw string
	var statusRefreshInterval time.Duration
	var watchSecrets bool
	var secretResyncInterval time.Duration
	flag.StringVar(&apiKeyFile, "api-key-file", "/etc/uptimerobot/apiKey",
		"Path to a file (mounted Secret) containing the UptimeRobot API token.")
	flag.BoolVar(&watchSecrets, "watch-secrets", false,
		"Watch credential Secrets (labelled monitoring.homelab.io/credential=true) so a "+
			"rotation is pushed to UptimeRobot promptly. Requires RBAC list;watch on secrets. "+
			"When false (default), rotation is picked up on --secret-resync-interval.")
	flag.DurationVar(&secretResyncInterval, "secret-resync-interval", time.Minute,
		"How often to re-reconcile a Monitor that references a credential Secret when "+
			"--watch-secrets is false, so a rotated Secret is re-read and re-pushed.")
	flag.StringVar(&baseURL, "uptimerobot-base-url", ur.DefaultBaseURL,
		"Base URL of the UptimeRobot v3 API.")
	flag.IntVar(&requestsPerMinute, "requests-per-minute", 10,
		"Client-side rate limit for UptimeRobot API calls (Free plan allows 10).")
	flag.StringVar(&defaultAlertContactsRaw, "default-alert-contacts", "",
		"Comma-separated UptimeRobot alert-contact IDs applied to monitors that do not set their own.")
	flag.DurationVar(&statusRefreshInterval, "status-refresh-interval", 5*time.Minute,
		"How often to re-read a monitor's live status from UptimeRobot.")

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	// Secret access strategy (see --watch-secrets):
	//  - default: do NOT cache Secrets; the client reads them directly with
	//    `get` (least privilege, no cluster list/watch). Rotation is picked up
	//    on --secret-resync-interval.
	//  - --watch-secrets: cache/watch ONLY Secrets labelled
	//    monitoring.homelab.io/credential=true so the controller can watch them
	//    for prompt rotation while keeping the informer small.
	clientOpts := client.Options{
		Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}},
	}
	cacheOpts := cache.Options{}
	if watchSecrets {
		clientOpts = client.Options{}
		cacheOpts.ByObject = map[client.Object]cache.ByObject{
			&corev1.Secret{}: {
				Label: labels.SelectorFromSet(labels.Set{"monitoring.homelab.io/credential": "true"}),
			},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "e60f050a.homelab.io",
		Client:                 clientOpts,
		Cache:                  cacheOpts,
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	// Build the UptimeRobot client backed by the mounted API-key file.
	// A missing/empty key is not fatal: the manager still starts (and stays
	// healthy) so it can be installed before the Secret exists; monitors simply
	// report a failing condition until a valid key is provided.
	keySource := apikey.NewFileSource(apiKeyFile)
	if err := keySource.Validate(context.Background()); err != nil {
		setupLog.Info("UptimeRobot API key not available at startup; monitors will not sync until it is provided",
			"path", apiKeyFile, "error", err.Error())
	}
	urClient := ur.NewHTTPClient(ur.Options{
		BaseURL:           baseURL,
		Token:             keySource.Token,
		RequestsPerMinute: requestsPerMinute,
		UserAgent:         "uptimerobot-operator",
	})

	defaultAlertContacts := parseDefaultAlertContacts(defaultAlertContactsRaw)
	if len(defaultAlertContacts) > 0 {
		setupLog.Info("Using default alert contacts", "count", len(defaultAlertContacts))
	}

	if err := (&controller.MonitorReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		UptimeRobot:           urClient,
		DefaultAlertContacts:  defaultAlertContacts,
		StatusRefreshInterval: statusRefreshInterval,
		WatchSecrets:          watchSecrets,
		SecretResyncInterval:  secretResyncInterval,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Monitor")
		os.Exit(1)
	}

	// The HTTPRoute watcher is only started when the Gateway API CRD is present,
	// so the manager does not crash on clusters without Gateway API installed.
	httpRouteGVK := schema.GroupVersionKind{Group: gatewayv1.GroupName, Version: "v1", Kind: "HTTPRoute"}
	if gvkInstalled(mgr, httpRouteGVK) {
		if err := (&controller.HTTPRouteReconciler{
			Client:               mgr.GetClient(),
			Scheme:               mgr.GetScheme(),
			DefaultAlertContacts: defaultAlertContacts,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create controller", "controller", "HTTPRoute")
			os.Exit(1)
		}
	} else {
		setupLog.Info("Gateway API not installed; HTTPRoute watch disabled (restart the operator after installing it)")
	}

	if err := (&controller.IngressReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		DefaultAlertContacts: defaultAlertContacts,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "Ingress")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

// gvkInstalled reports whether the cluster has a REST mapping for gvk. A
// NoMatch error means the CRD is not installed; other errors are treated as
// "not installed" and logged by the caller. Note: the RESTMapper is populated
// at manager start, so installing the CRD later requires an operator restart.
func gvkInstalled(mgr ctrl.Manager, gvk schema.GroupVersionKind) bool {
	_, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return false
		}
		setupLog.Error(err, "Failed to probe for CRD; treating as not installed", "gvk", gvk.String())
		return false
	}
	return true
}

// parseDefaultAlertContacts parses a comma-separated list of alert-contact IDs.
func parseDefaultAlertContacts(raw string) []monitoringv1alpha1.AlertContactAssignment {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []monitoringv1alpha1.AlertContactAssignment
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			setupLog.Info("Ignoring invalid alert-contact ID", "value", part)
			continue
		}
		out = append(out, monitoringv1alpha1.AlertContactAssignment{AlertContactID: id})
	}
	return out
}
