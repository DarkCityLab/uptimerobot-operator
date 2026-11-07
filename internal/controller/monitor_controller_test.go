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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	monitoringv1alpha1 "github.com/DarkCityLab/uptimerobot-operator/api/v1alpha1"
	urfake "github.com/DarkCityLab/uptimerobot-operator/internal/uptimerobot/fake"
)

var _ = Describe("Monitor Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		monitor := &monitoringv1alpha1.Monitor{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Monitor")
			err := k8sClient.Get(ctx, typeNamespacedName, monitor)
			if err != nil && errors.IsNotFound(err) {
				resource := &monitoringv1alpha1.Monitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: monitoringv1alpha1.MonitorSpec{
						Type:     monitoringv1alpha1.MonitorHTTP,
						URL:      "https://example.com",
						Interval: 300,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &monitoringv1alpha1.Monitor{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Monitor")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should create an UptimeRobot monitor and set a finalizer", func() {
			fake := urfake.New()
			controllerReconciler := &MonitorReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				UptimeRobot: fake,
			}

			By("reconciling until the monitor ID is recorded")
			Eventually(func(g Gomega) {
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: typeNamespacedName})
				g.Expect(err).NotTo(HaveOccurred())

				var mon monitoringv1alpha1.Monitor
				g.Expect(k8sClient.Get(ctx, typeNamespacedName, &mon)).To(Succeed())
				g.Expect(mon.Finalizers).To(ContainElement(finalizerName))
				g.Expect(mon.Status.MonitorID).NotTo(BeEmpty())
			}).Should(Succeed())
			Expect(fake.Count()).To(Equal(1))
		})
	})
})
