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

package controller

import (
	"context"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	"github.com/DarkCityLab/uptimerobot-operator/internal/annotations"
	ur "github.com/DarkCityLab/uptimerobot-operator/internal/uptimerobot"
)

const (
	// finalizerName guards external monitor deletion.
	finalizerName = "monitoring.homelab.io/finalizer"

	// conditionReady is the primary condition type on a Monitor.
	conditionReady = "Ready"

	// defaultStatusRefresh is how often a steady-state monitor is re-read when
	// stats collection is disabled.
	defaultStatusRefresh = 5 * time.Minute
	// defaultStatsRefresh is the fallback stats polling interval.
	defaultStatsRefresh = 15 * time.Minute
)

// MonitorReconciler reconciles a Monitor object against the UptimeRobot API.
type MonitorReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// UptimeRobot is the API client (interface, so tests inject a fake).
	UptimeRobot ur.Client

	// DefaultAlertContacts is injected into monitors that do not set their own.
	DefaultAlertContacts []monitoringv1alpha1.AlertContactAssignment

	// StatusRefreshInterval overrides defaultStatusRefresh when non-zero.
	StatusRefreshInterval time.Duration
}

// +kubebuilder:rbac:groups=monitoring.homelab.io,resources=monitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.homelab.io,resources=monitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.homelab.io,resources=monitors/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile drives a Monitor toward its desired state in UptimeRobot.
func (r *MonitorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var mon monitoringv1alpha1.Monitor
	if err := r.Get(ctx, req.NamespacedName, &mon); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion.
	if !mon.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &mon)
	}

	// Ensure finalizer before creating any external resource.
	if !controllerutil.ContainsFinalizer(&mon, finalizerName) {
		controllerutil.AddFinalizer(&mon, finalizerName)
		if err := r.Update(ctx, &mon); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	desired, err := r.buildClientMonitor(ctx, &mon)
	if err != nil {
		// Spec references a missing Secret, etc. Surface and retry.
		return r.fail(ctx, &mon, "DependencyError", err)
	}

	// Create or adopt if we have no monitor ID yet.
	if mon.Status.MonitorID == "" {
		return r.reconcileCreate(ctx, &mon, desired)
	}

	id, err := strconv.ParseInt(mon.Status.MonitorID, 10, 64)
	if err != nil {
		// Corrupt ID: clear it and recreate next pass.
		mon.Status.MonitorID = ""
		if err := r.Status().Update(ctx, &mon); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Update on spec change (generation bump).
	live := (*ur.Monitor)(nil)
	if mon.Status.ObservedGeneration != mon.Generation {
		updated, err := r.UptimeRobot.UpdateMonitor(ctx, id, desired)
		if err != nil {
			if ur.IsNotFound(err) {
				// Monitor vanished server-side: recreate.
				mon.Status.MonitorID = ""
				if err := r.Status().Update(ctx, &mon); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{Requeue: true}, nil
			}
			return r.handleAPIError(ctx, &mon, "UpdateFailed", err)
		}
		live = updated
		r.event(&mon, "Updated", "Updated UptimeRobot monitor %d", id)
	} else {
		fetched, err := r.UptimeRobot.GetMonitor(ctx, id)
		if err != nil {
			if ur.IsNotFound(err) {
				mon.Status.MonitorID = ""
				if err := r.Status().Update(ctx, &mon); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{Requeue: true}, nil
			}
			return r.handleAPIError(ctx, &mon, "GetFailed", err)
		}
		live = fetched
	}

	// Reconcile paused state.
	if err := r.reconcilePaused(ctx, &mon, id, live); err != nil {
		return r.handleAPIError(ctx, &mon, "PauseFailed", err)
	}

	// One-shot reset-stats on annotation nonce change.
	if err := r.reconcileResetStats(ctx, &mon, id); err != nil {
		return r.handleAPIError(ctx, &mon, "ResetStatsFailed", err)
	}

	// Optional stats polling.
	requeueAfter := r.statusRefresh()
	if mon.Spec.CollectStats {
		interval := r.statsRefresh(&mon)
		if r.statsStale(&mon, interval) {
			if err := r.pollStats(ctx, &mon, id); err != nil {
				// Non-fatal: log and continue; stats are best-effort.
				log.V(1).Info("stats poll failed", "error", err)
			}
		}
		requeueAfter = interval
	}

	r.applyLiveStatus(&mon, live)
	mon.Status.ObservedGeneration = mon.Generation
	mon.Status.State = "Active"
	meta.SetStatusCondition(&mon.Status.Conditions, metav1.Condition{
		Type:    conditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  "Synced",
		Message: "Monitor is synced with UptimeRobot",
	})
	if err := r.Status().Update(ctx, &mon); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// reconcileCreate creates (or adopts an existing) monitor.
func (r *MonitorReconciler) reconcileCreate(ctx context.Context, mon *monitoringv1alpha1.Monitor, desired *ur.Monitor) (ctrl.Result, error) {
	created, err := r.UptimeRobot.CreateMonitor(ctx, desired)
	if err != nil {
		if ur.IsDuplicate(err) {
			// Adopt an existing monitor matching our URL + friendly name.
			adopted, adoptErr := r.adoptExisting(ctx, desired)
			if adoptErr != nil {
				return r.handleAPIError(ctx, mon, "AdoptFailed", adoptErr)
			}
			if adopted != nil {
				created = adopted
			} else {
				return r.handleAPIError(ctx, mon, "DuplicateUnresolved", err)
			}
		} else {
			return r.handleAPIError(ctx, mon, "CreateFailed", err)
		}
	}

	// Persist the ID immediately so a crash cannot leak the monitor.
	mon.Status.MonitorID = strconv.FormatInt(created.ID, 10)
	r.applyLiveStatus(mon, created)
	mon.Status.State = "Active"
	meta.SetStatusCondition(&mon.Status.Conditions, metav1.Condition{
		Type:    conditionReady,
		Status:  metav1.ConditionTrue,
		Reason:  "Created",
		Message: "Monitor created in UptimeRobot",
	})
	if err := r.Status().Update(ctx, mon); err != nil {
		return ctrl.Result{}, err
	}
	r.event(mon, "Created", "Created UptimeRobot monitor %d", created.ID)
	// Requeue to apply paused/reset/stats in a settled pass.
	return ctrl.Result{Requeue: true}, nil
}

// adoptExisting lists monitors and returns the one matching desired by URL and
// friendly name, or nil if none matched.
func (r *MonitorReconciler) adoptExisting(ctx context.Context, desired *ur.Monitor) (*ur.Monitor, error) {
	monitors, err := r.UptimeRobot.ListMonitors(ctx, ur.ListOptions{Search: desired.URL})
	if err != nil {
		return nil, err
	}
	for i := range monitors {
		m := &monitors[i]
		if m.URL == desired.URL && m.FriendlyName == desired.FriendlyName {
			return m, nil
		}
	}
	return nil, nil
}

// reconcileDelete deletes the external monitor then removes the finalizer.
func (r *MonitorReconciler) reconcileDelete(ctx context.Context, mon *monitoringv1alpha1.Monitor) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(mon, finalizerName) {
		return ctrl.Result{}, nil
	}

	if mon.Status.MonitorID != "" {
		if id, err := strconv.ParseInt(mon.Status.MonitorID, 10, 64); err == nil {
			if err := r.UptimeRobot.DeleteMonitor(ctx, id); err != nil && !ur.IsNotFound(err) {
				// Real failure (incl. rate limit): requeue, keep finalizer.
				if ur.IsRateLimited(err) {
					return ctrl.Result{RequeueAfter: ur.RetryAfter(err)}, nil
				}
				return ctrl.Result{}, err
			}
			r.event(mon, "Deleted", "Deleted UptimeRobot monitor %d", id)
		}
	}

	controllerutil.RemoveFinalizer(mon, finalizerName)
	if err := r.Update(ctx, mon); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// reconcilePaused converges the monitor's paused state with the spec.
func (r *MonitorReconciler) reconcilePaused(ctx context.Context, mon *monitoringv1alpha1.Monitor, id int64, live *ur.Monitor) error {
	livePaused := live != nil && live.Status == "PAUSED"
	switch {
	case mon.Spec.Paused && !livePaused:
		if err := r.UptimeRobot.PauseMonitor(ctx, id); err != nil {
			return err
		}
		if live != nil {
			live.Status = "PAUSED"
		}
		r.event(mon, "Paused", "Paused UptimeRobot monitor %d", id)
	case !mon.Spec.Paused && livePaused:
		if err := r.UptimeRobot.StartMonitor(ctx, id); err != nil {
			return err
		}
		if live != nil {
			live.Status = "UP"
		}
		r.event(mon, "Started", "Resumed UptimeRobot monitor %d", id)
	}
	return nil
}

// reconcileResetStats fires a one-shot reset when the annotation nonce changes.
func (r *MonitorReconciler) reconcileResetStats(ctx context.Context, mon *monitoringv1alpha1.Monitor, id int64) error {
	token := mon.Annotations[annotations.ResetStats]
	if token == "" || token == mon.Status.LastResetStatsToken {
		return nil
	}
	if err := r.UptimeRobot.ResetStats(ctx, id); err != nil {
		return err
	}
	mon.Status.LastResetStatsToken = token
	r.event(mon, "StatsReset", "Reset statistics for UptimeRobot monitor %d", id)
	return nil
}

// pollStats fetches uptime + response-time stats into status.
func (r *MonitorReconciler) pollStats(ctx context.Context, mon *monitoringv1alpha1.Monitor, id int64) error {
	up, err := r.UptimeRobot.UptimeStats(ctx, id, "", "")
	if err != nil {
		return err
	}
	rt, err := r.UptimeRobot.ResponseTimeStats(ctx, id, "", "")
	if err != nil {
		return err
	}
	mon.Status.Uptime = &monitoringv1alpha1.UptimeStatus{
		Ratio:           strconv.FormatFloat(up.UptimePercentage, 'f', 3, 64),
		DowntimeSeconds: int64(up.DowntimeSeconds),
		IncidentCount:   up.IncidentCount,
		From:            up.From,
		To:              up.To,
	}
	mon.Status.ResponseTime = &monitoringv1alpha1.ResponseTimeStatus{}
	if rt.AvgMS != nil {
		mon.Status.ResponseTime.AverageMS = int(*rt.AvgMS)
	}
	if rt.MinMS != nil {
		mon.Status.ResponseTime.MinMS = int(*rt.MinMS)
	}
	if rt.MaxMS != nil {
		mon.Status.ResponseTime.MaxMS = int(*rt.MaxMS)
	}
	now := metav1.Now()
	mon.Status.LastStatsSync = &now
	return nil
}

// applyLiveStatus copies read-only metadata from the live monitor into status.
func (r *MonitorReconciler) applyLiveStatus(mon *monitoringv1alpha1.Monitor, live *ur.Monitor) {
	if live == nil {
		return
	}
	mon.Status.ObservedURL = live.URL
	mon.Status.HeartbeatURL = live.HeartbeatURL
	mon.Status.MonitorStatus = live.Status
	mon.Status.CurrentStateDuration = live.CurrentStateDuration
	mon.Status.CreateDateTime = live.CreateDateTime
	mon.Status.SSLBrand = live.SSLBrand
	mon.Status.SSLExpiryDateTime = live.SSLExpiryDateTime
	mon.Status.DomainExpireDate = live.DomainExpireDate
	if live.LastIncident != nil {
		mon.Status.LastIncident = &monitoringv1alpha1.IncidentStatus{
			ID:        live.LastIncident.ID,
			Status:    live.LastIncident.Status,
			Cause:     live.LastIncident.Cause,
			Reason:    live.LastIncident.Reason,
			StartedAt: live.LastIncident.StartedAt,
			Duration:  live.LastIncident.Duration,
		}
	}
}

// handleAPIError sets a Ready=False condition and decides requeue behaviour
// based on whether the error is terminal, rate-limited or transient.
func (r *MonitorReconciler) handleAPIError(ctx context.Context, mon *monitoringv1alpha1.Monitor, reason string, err error) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if ur.IsRateLimited(err) {
		// Back off for the server-advised duration; not an error state.
		meta.SetStatusCondition(&mon.Status.Conditions, metav1.Condition{
			Type: conditionReady, Status: metav1.ConditionFalse,
			Reason: "RateLimited", Message: err.Error(),
		})
		_ = r.Status().Update(ctx, mon)
		return ctrl.Result{RequeueAfter: ur.RetryAfter(err)}, nil
	}

	mon.Status.State = "Error"
	meta.SetStatusCondition(&mon.Status.Conditions, metav1.Condition{
		Type: conditionReady, Status: metav1.ConditionFalse,
		Reason: reason, Message: err.Error(),
	})
	if statusErr := r.Status().Update(ctx, mon); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	r.event(mon, reason, "%s", err.Error())

	if ur.IsTerminal(err) {
		// Won't fix itself: stop requeueing on error (wait for a spec change).
		log.Info("terminal UptimeRobot error; not requeueing", "reason", reason, "error", err.Error())
		return ctrl.Result{}, nil
	}
	// Transient: return the error for backoff requeue.
	return ctrl.Result{}, err
}

// fail sets a Ready=False condition for a non-API error and requeues.
func (r *MonitorReconciler) fail(ctx context.Context, mon *monitoringv1alpha1.Monitor, reason string, err error) (ctrl.Result, error) {
	mon.Status.State = "Error"
	meta.SetStatusCondition(&mon.Status.Conditions, metav1.Condition{
		Type: conditionReady, Status: metav1.ConditionFalse,
		Reason: reason, Message: err.Error(),
	})
	if statusErr := r.Status().Update(ctx, mon); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, err
}

func (r *MonitorReconciler) statusRefresh() time.Duration {
	if r.StatusRefreshInterval > 0 {
		return r.StatusRefreshInterval
	}
	return defaultStatusRefresh
}

func (r *MonitorReconciler) statsRefresh(mon *monitoringv1alpha1.Monitor) time.Duration {
	if mon.Spec.StatsRefreshInterval != nil && mon.Spec.StatsRefreshInterval.Duration > 0 {
		return mon.Spec.StatsRefreshInterval.Duration
	}
	return defaultStatsRefresh
}

func (r *MonitorReconciler) statsStale(mon *monitoringv1alpha1.Monitor, interval time.Duration) bool {
	if mon.Status.LastStatsSync == nil {
		return true
	}
	return time.Since(mon.Status.LastStatsSync.Time) >= interval
}

func (r *MonitorReconciler) event(mon *monitoringv1alpha1.Monitor, reason, format string, args ...any) {
	if r.Recorder != nil {
		r.Recorder.Eventf(mon, "Normal", reason, format, args...)
	}
}

// SetupWithManager wires the controller.
func (r *MonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		// The classic record.EventRecorder API is sufficient here; the newer
		// events API offers no benefit for these simple Normal events.
		r.Recorder = mgr.GetEventRecorderFor("monitor-controller") //nolint:staticcheck
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.Monitor{}).
		Named("monitor").
		Complete(r)
}
