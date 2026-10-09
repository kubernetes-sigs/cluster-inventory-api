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
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cp "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
)

type snapshot struct {
	resources    *metav1.APIResourceList
	apiError     error
	profiles     []cp.ClusterProfile
	profileError error
	manager      string
	expected     int
}

func (s snapshot) managed() []cp.ClusterProfile {
	return slices.DeleteFunc(slices.Clone(s.profiles), func(p cp.ClusterProfile) bool {
		return s.manager != "" && p.Spec.ClusterManager.Name != s.manager
	})
}

func outcome(req Requirement, state Outcome, message string) TestResult {
	return TestResult{Requirement: req, Outcome: state, Message: message}
}

func checkAPI(req Requirement, s snapshot) TestResult {
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
		func(a metav1.APIResource) bool { return a.Name == "clusterprofiles" },
	)
	if idx < 0 {
		return outcome(req, Failed, "clusterprofiles is not served")
	}
	if req.ID == CheckNamespaced && !s.resources.APIResources[idx].Namespaced {
		return outcome(req, Failed, "clusterprofiles is cluster scoped")
	}
	if req.ID == CheckAPIAvailability && s.profileError != nil {
		return outcome(req, Unknown, "Cannot list the inventory: "+s.profileError.Error())
	}
	return outcome(req, Passed, "")
}

func check(req Requirement, s snapshot) TestResult {
	if req.Scope == ScopeAPI {
		return checkAPI(req, s)
	}
	if s.profileError != nil {
		return outcome(req, Unknown, "Cannot list the inventory: "+s.profileError.Error())
	}
	profiles := s.managed()
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
		name := p.Namespace + "/" + p.Name
		id := p.Labels[cp.LabelInventoryMemberIDKey]
		if id == "" {
			missing = append(missing, name)
		} else {
			ids[id] = append(ids[id], name)
		}
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	for _, id := range keys {
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
		name := p.Namespace + "/" + p.Name
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
	if len(missing) > 0 {
		slices.Sort(missing)
		state := NotSupported
		if req.Level == LevelShould {
			state = Failed
		}
		return outcome(req, state, "Not reported on: "+strings.Join(missing, ", "))
	}
	return outcome(req, Passed, "")
}
