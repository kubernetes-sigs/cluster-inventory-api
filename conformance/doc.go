/*
Copyright The Kubernetes Authors.

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

// Package conformance contains the conformance test suite for the Cluster
// Inventory APIs defined by KEP-4322. Each API is a separately selectable and
// reportable section of the suite (see the --apis, --report-format and
// --report-output flags); the ClusterProfile API is covered today.
//
// ClusterProfile objects are created and reconciled by a cluster manager
// implementation, not by API consumers, so the suite verifies the behavior a
// conformant implementation must exhibit: it observes the implementation-managed
// ClusterProfile objects in an inventory namespace (pointed at via the
// --namespace and --cluster-manager flags) and checks them against the
// conventions KEP-4322 places on cluster managers. It never creates
// ClusterProfile objects itself; validation enforced by the CRD schema is
// covered by the repository's integration tests instead.
//
// The suite is written with Ginkgo v2 and runs against any cluster that
// serves the Cluster Inventory APIs, addressed via kubeconfig/context flags.
// It is a separate Go module so that implementations can import the suite
// into their own test pipelines without adding test-only dependencies to
// the main sigs.k8s.io/cluster-inventory-api module, following the pattern
// established by sigs.k8s.io/mcs-api/conformance.
package conformance
