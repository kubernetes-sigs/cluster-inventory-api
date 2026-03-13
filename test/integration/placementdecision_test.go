/*
Copyright 2026 The Kubernetes Authors.

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

package integration

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	cpv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

var _ = ginkgo.Describe("PlacementDecisionAPI test", func() {
	var decisionName string

	ginkgo.BeforeEach(func() {
		decisionName = fmt.Sprintf("decision-%s", rand.String(5))
	})

	ginkgo.It("Should create a PlacementDecision", func() {
		placementDecision := &cpv1alpha1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name: decisionName,
				Labels: map[string]string{
					cpv1alpha1.DecisionKeyLabel: "test-decision",
				},
			},
			Decisions: []cpv1alpha1.ClusterDecision{
				{
					ClusterProfileRef: cpv1alpha1.ClusterProfileReference{
						Name: "cluster-1",
					},
					Reason: "best-fit",
				},
				{
					ClusterProfileRef: cpv1alpha1.ClusterProfileReference{
						Name:      "cluster-2",
						Namespace: "other-ns",
					},
				},
			},
			SchedulerName: "test-scheduler",
		}

		_, err := clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Create(
			context.TODO(),
			placementDecision,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.It("Should update a PlacementDecision", func() {
		placementDecision := &cpv1alpha1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name: decisionName,
			},
			Decisions: []cpv1alpha1.ClusterDecision{
				{
					ClusterProfileRef: cpv1alpha1.ClusterProfileReference{
						Name: "cluster-1",
					},
				},
			},
			SchedulerName: "scheduler-v1",
		}

		placementDecision, err := clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Create(
			context.TODO(),
			placementDecision,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		updated := placementDecision.DeepCopy()
		updated.Decisions = append(updated.Decisions, cpv1alpha1.ClusterDecision{
			ClusterProfileRef: cpv1alpha1.ClusterProfileReference{
				Name: "cluster-2",
			},
			Reason: "best-fit",
		})
		updated.SchedulerName = "scheduler-v2"

		_, err = clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Update(
			context.TODO(),
			updated,
			metav1.UpdateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		got, err := clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Get(
			context.TODO(),
			decisionName,
			metav1.GetOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(got.Decisions).To(gomega.HaveLen(2))
		gomega.Expect(got.SchedulerName).To(gomega.Equal("scheduler-v2"))
	})

	ginkgo.It("Should delete a PlacementDecision", func() {
		placementDecision := &cpv1alpha1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name: decisionName,
			},
			Decisions: []cpv1alpha1.ClusterDecision{},
		}

		_, err := clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Create(
			context.TODO(),
			placementDecision,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		err = clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Delete(
			context.TODO(),
			decisionName,
			metav1.DeleteOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		_, err = clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Get(
			context.TODO(),
			decisionName,
			metav1.GetOptions{},
		)
		gomega.Expect(errors.IsNotFound(err)).To(gomega.BeTrue())
	})

	ginkgo.It("Should reject a PlacementDecision exceeding 100 decisions", func() {
		decisions := make([]cpv1alpha1.ClusterDecision, 101)
		for i := range decisions {
			decisions[i] = cpv1alpha1.ClusterDecision{
				ClusterProfileRef: cpv1alpha1.ClusterProfileReference{
					Name: fmt.Sprintf("cluster-%d", i),
				},
			}
		}

		placementDecision := &cpv1alpha1.PlacementDecision{
			ObjectMeta: metav1.ObjectMeta{
				Name: decisionName,
			},
			Decisions: decisions,
		}

		_, err := clusterProfileClient.ApisV1alpha1().PlacementDecisions(testNamespace).Create(
			context.TODO(),
			placementDecision,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).To(gomega.HaveOccurred())
		gomega.Expect(errors.IsInvalid(err)).To(gomega.BeTrue())
	})
})
