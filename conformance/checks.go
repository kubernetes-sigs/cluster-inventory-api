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

package conformance

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
)

// discovered is the served resource list of the API group version.
type discovered struct {
	resources *metav1.APIResourceList
	apiError  error
}

// servedResource names a resource, how its listing is described, and the API checks that cover it.
type servedResource struct {
	name, listing, availabilityCheck, namespacedCheck string
}

var clusterProfileResource = servedResource{
	name:              cp.ClusterProfileSchemeGroupVersionResource.Resource,
	listing:           "the inventory",
	availabilityCheck: CheckAPIAvailability,
	namespacedCheck:   CheckNamespaced,
}

type snapshot struct {
	discovered
	profiles     []cp.ClusterProfile
	managed      []cp.ClusterProfile
	profileError error
	expected     int
}

func outcome(req Requirement, state Outcome, message string) TestResult {
	return TestResult{Requirement: req, Outcome: state, Message: message}
}

func checkAPI(req Requirement, s discovered, r servedResource, listError error) TestResult {
	if s.apiError != nil {
		if apierrors.IsNotFound(s.apiError) {
			return outcome(req, Failed, "API group version is not served")
		}
		return outcome(req, Unknown, "API discovery failed: "+s.apiError.Error())
	}
	if s.resources == nil {
		return outcome(req, Unknown, "API discovery returned no resources")
	}
	idx := slices.IndexFunc(
		s.resources.APIResources,
		func(a metav1.APIResource) bool { return a.Name == r.name },
	)
	if idx < 0 {
		return outcome(req, Failed, r.name+" is not served")
	}
	if req.ID == r.namespacedCheck && !s.resources.APIResources[idx].Namespaced {
		return outcome(req, Failed, r.name+" is cluster scoped")
	}
	if req.ID == r.availabilityCheck && listError != nil {
		return outcome(req, Unknown, "Cannot list "+r.listing+": "+listError.Error())
	}
	return outcome(req, Passed, "")
}

func check(req Requirement, s snapshot) TestResult {
	if req.Scope == ScopeAPI {
		return checkAPI(req, s.discovered, clusterProfileResource, s.profileError)
	}
	if s.profileError != nil {
		return outcome(req, Unknown, "Cannot list the inventory: "+s.profileError.Error())
	}
	profiles := s.managed
	if req.Scope == ScopeInventory {
		profiles = s.profiles
	}
	if req.ID == CheckRepresentation {
		if len(profiles) == 0 {
			return outcome(req, Failed, "No managed ClusterProfile; register at least one member before running")
		}
		if s.expected > 0 && len(profiles) != s.expected {
			return outcome(
				req,
				Failed,
				fmt.Sprintf("Expected %d managed profiles, observed %d", s.expected, len(profiles)),
			)
		}
		return outcome(req, Passed, "")
	}
	if len(profiles) == 0 {
		return outcome(req, Unknown, "No profiles to verify")
	}
	if req.ID == InventoryMemberUniqueness {
		return checkUniqueness(req, profiles)
	}
	return checkFields(req, profiles)
}

func checkUniqueness(req Requirement, profiles []cp.ClusterProfile) TestResult {
	missing := []string{}
	ids := map[string][]string{}
	for _, p := range profiles {
		name := types.NamespacedName{Namespace: p.Namespace, Name: p.Name}.String()
		id := p.Labels[cp.LabelInventoryMemberIDKey]
		if id == "" {
			missing = append(missing, name)
		} else {
			ids[id] = append(ids[id], name)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		if len(ids[id]) > 1 {
			slices.Sort(ids[id])
			return outcome(req, Failed, fmt.Sprintf("Member ID %q is duplicated: %s", id, strings.Join(ids[id], ", ")))
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		return outcome(
			req,
			Skipped,
			"Uniqueness cannot be determined; missing member IDs: "+strings.Join(missing, ", "),
		)
	}
	return outcome(req, Passed, "")
}

func checkFields(req Requirement, profiles []cp.ClusterProfile) TestResult {
	missing := []string{}
	for _, p := range profiles {
		name := types.NamespacedName{Namespace: p.Namespace, Name: p.Name}.String()
		manager, hasManager := p.Labels[cp.LabelClusterManagerKey]
		id, hasID := p.Labels[cp.LabelInventoryMemberIDKey]
		switch req.ID {
		case CheckManagerValue:
			if hasManager && manager != p.Spec.ClusterManager.Name {
				return outcome(req, Failed, name+": cluster manager label does not match spec.clusterManager.name")
			}
		case CheckMemberIDValue:
			if hasID && id == "" {
				return outcome(req, Failed, name+": inventory member ID is empty")
			}
		case CheckManagerPresence:
			if !hasManager {
				missing = append(missing, name)
			}
		case CheckMemberIDPresence:
			if id == "" {
				missing = append(missing, name)
			}
		case CheckHealthyPresence:
			if meta.FindStatusCondition(p.Status.Conditions, cp.ClusterConditionControlPlaneHealthy) == nil {
				missing = append(missing, name)
			}
		case CheckJoinedPresence:
			if meta.FindStatusCondition(p.Status.Conditions, Joined) == nil {
				missing = append(missing, name)
			}
		case CheckVersionPresence:
			if p.Status.Version.Kubernetes == "" {
				missing = append(missing, name)
			}
		}
	}
	return unmet(req, "Not reported on: ", missing)
}

// unmet grades a capability the listed objects lack: a SHOULD fails, while a
// MAY or Informative capability is not supported.
func unmet(req Requirement, reason string, objects []string) TestResult {
	if len(objects) == 0 {
		return outcome(req, Passed, "")
	}
	slices.Sort(objects)
	state := NotSupported
	if req.Level == LevelShould {
		state = Failed
	}
	return outcome(req, state, reason+strings.Join(objects, ", "))
}

// referenceTo identifies the exact object version a check observed.
func referenceTo(m metav1.ObjectMeta) ObjectReference {
	return ObjectReference{Namespace: m.Namespace, Name: m.Name, UID: string(m.UID), ResourceVersion: m.ResourceVersion}
}
