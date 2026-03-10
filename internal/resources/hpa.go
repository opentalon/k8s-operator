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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

// BuildHPA creates a HorizontalPodAutoscaler for the OpenTalonInstance StatefulSet.
func BuildHPA(instance *v1alpha1.OpenTalonInstance) *autoscalingv2.HorizontalPodAutoscaler {
	hpaSpec := instance.Spec.Availability.HorizontalPodAutoscaler

	minReplicas := int32(1)
	if hpaSpec.MinReplicas != nil {
		minReplicas = *hpaSpec.MinReplicas
	}

	maxReplicas := int32(5)
	if hpaSpec.MaxReplicas > 0 {
		maxReplicas = hpaSpec.MaxReplicas
	}

	metrics := buildHPAMetrics(hpaSpec)

	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
				Name:       ResourceName(instance),
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics:     metrics,
		},
	}
}

// buildHPAMetrics constructs the metrics slice for the HPA.
func buildHPAMetrics(spec v1alpha1.HPASpec) []autoscalingv2.MetricSpec {
	var metrics []autoscalingv2.MetricSpec

	if spec.CPUUtilization != nil {
		utilization := *spec.CPUUtilization
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &utilization,
				},
			},
		})
	}

	if spec.MemoryUtilization != nil {
		utilization := *spec.MemoryUtilization
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &utilization,
				},
			},
		})
	}

	// Default: scale on CPU utilisation at 80% when nothing is explicitly configured.
	if len(metrics) == 0 {
		defaultUtil := int32(80)
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &defaultUtil,
				},
			},
		})
	}

	return metrics
}
