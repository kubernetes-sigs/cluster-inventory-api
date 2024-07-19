/*
Copyright 2024 The Kubernetes Authors.

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

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	cpv1alpha1 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

var _ = ginkgo.Describe("ClusterProfileAPI test", func() {
	var clusterName string
	var clusterManagerName string

	ginkgo.BeforeEach(func() {
		clusterName = fmt.Sprintf("cluster-%s", rand.String(5))
		clusterManagerName = fmt.Sprintf("cluster-manager-%s", rand.String(5))
	})

	ginkgo.It("Should create a ClusterProfile", func() {
		clusterProfile := &cpv1alpha1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:   clusterName,
				Labels: map[string]string{cpv1alpha1.LabelClusterManagerKey: clusterManagerName},
			},
			Spec: cpv1alpha1.ClusterProfileSpec{
				DisplayName: clusterName,
				ClusterManager: cpv1alpha1.ClusterManager{
					Name: clusterManagerName,
				},
			},
		}

		_, err := clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).Create(
			context.TODO(),
			clusterProfile,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.It("Should update the ClusterProfile status", func() {
		clusterProfile := &cpv1alpha1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:   clusterName,
				Labels: map[string]string{cpv1alpha1.LabelClusterManagerKey: clusterManagerName},
			},
			Spec: cpv1alpha1.ClusterProfileSpec{
				DisplayName: clusterName,
				ClusterManager: cpv1alpha1.ClusterManager{
					Name: clusterManagerName,
				},
			},
		}

		clusterProfile, err := clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).Create(
			context.TODO(),
			clusterProfile,
			metav1.CreateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		newClusterProfile := clusterProfile.DeepCopy()
		newClusterProfile.Status.Version.Kubernetes = "1.29.0"
		newClusterProfile.Status.Properties = []cpv1alpha1.Property{{Name: "n1", Value: "v1"}}
		meta.SetStatusCondition(&newClusterProfile.Status.Conditions, metav1.Condition{
			Type:    cpv1alpha1.ClusterConditionControlPlaneHealthy,
			Status:  metav1.ConditionTrue,
			Reason:  "Reason",
			Message: "Message",
		})

		_, err = clusterProfileClient.ApisV1alpha1().ClusterProfiles(testNamespace).UpdateStatus(
			context.TODO(),
			newClusterProfile,
			metav1.UpdateOptions{},
		)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})
})
