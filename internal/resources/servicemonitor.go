// Copyright 2026 OpenTalon Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

// ServiceMonitorGroupVersionKind identifies the Prometheus Operator ServiceMonitor CRD.
const (
	ServiceMonitorGroup   = "monitoring.coreos.com"
	ServiceMonitorVersion = "v1"
	ServiceMonitorKind    = "ServiceMonitor"
)

// BuildServiceMonitor returns an *unstructured.Unstructured representing a
// Prometheus Operator ServiceMonitor for the OpenTalonInstance.
// Using unstructured avoids a hard dependency on the prometheus-operator CRD
// Go types, keeping the operator functional even when the CRD is not installed
// (the controller gracefully degrades in that case).
func BuildServiceMonitor(instance *v1alpha1.OpenTalonInstance) *unstructured.Unstructured {
	smSpec := instance.Spec.Observability.Metrics.ServiceMonitor

	interval := smSpec.Interval
	if interval == "" {
		interval = "30s"
	}

	metricsPath := instance.Spec.Observability.Metrics.Path
	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	// Merge standard labels with user-supplied ServiceMonitor labels.
	matchLabels := Labels(instance)
	smLabels := Labels(instance)
	for k, v := range smSpec.Labels {
		smLabels[k] = v
	}

	sm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": ServiceMonitorGroup + "/" + ServiceMonitorVersion,
			"kind":       ServiceMonitorKind,
			"metadata": map[string]interface{}{
				"name":      ResourceName(instance),
				"namespace": instance.Namespace,
				"labels":    smLabels,
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": matchLabels,
				},
				"namespaceSelector": map[string]interface{}{
					"matchNames": []interface{}{instance.Namespace},
				},
				"endpoints": []interface{}{
					map[string]interface{}{
						"port":     "metrics",
						"path":     metricsPath,
						"interval": interval,
						"scheme":   "http",
					},
				},
			},
		},
	}

	return sm
}
