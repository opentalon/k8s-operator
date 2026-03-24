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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

// BuildService creates the Kubernetes Service for the OpenTalonInstance.
// All enabled channel ports (webhook, websocket) and the metrics port are
// included as named ports on the service.
func BuildService(instance *v1alpha1.OpenTalonInstance) *corev1.Service {
	svcSpec := instance.Spec.Networking.Service

	svcType := svcSpec.Type
	if svcType == "" {
		svcType = corev1.ServiceTypeClusterIP
	}

	labels := Labels(instance)

	// Merge any user-supplied extra service labels.
	svcLabels := make(map[string]string, len(labels)+len(svcSpec.Labels))
	for k, v := range labels {
		svcLabels[k] = v
	}
	for k, v := range svcSpec.Labels {
		svcLabels[k] = v
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ResourceName(instance),
			Namespace:   instance.Namespace,
			Labels:      svcLabels,
			Annotations: svcSpec.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: SelectorLabels(instance),
			Ports:    buildServicePorts(instance),
		},
	}

	return svc
}

// buildServicePorts enumerates all named service ports derived from the enabled
// channel and observability configuration.
func buildServicePorts(instance *v1alpha1.OpenTalonInstance) []corev1.ServicePort {
	var ports []corev1.ServicePort

	svcSpec := instance.Spec.Networking.Service
	primaryPort := svcSpec.Port
	if primaryPort == 0 {
		primaryPort = 8080
	}

	// Webhook port.
	if instance.Spec.Config.Channels != nil {
		if wh := instance.Spec.Config.Channels.Webhook; wh != nil && wh.Enabled {
			whPort := wh.Port
			if whPort == 0 {
				whPort = 8080
			}
			ports = append(ports, corev1.ServicePort{
				Name:       "webhook",
				Port:       whPort,
				TargetPort: intstr.FromString("webhook"),
				Protocol:   corev1.ProtocolTCP,
			})
		}

		// WebSocket port.
		if ws := instance.Spec.Config.Channels.WebSocket; ws != nil && ws.Enabled {
			wsPort := ws.Port
			if wsPort == 0 {
				wsPort = 8081
			}
			ports = append(ports, corev1.ServicePort{
				Name:       "websocket",
				Port:       wsPort,
				TargetPort: intstr.FromString("websocket"),
				Protocol:   corev1.ProtocolTCP,
			})
		}
	}

	// Metrics port.
	if instance.Spec.Observability.Metrics.Enabled != nil && *instance.Spec.Observability.Metrics.Enabled {
		metricsPort := instance.Spec.Observability.Metrics.Port
		if metricsPort == 0 {
			metricsPort = 9090
		}
		ports = append(ports, corev1.ServicePort{
			Name:       "metrics",
			Port:       metricsPort,
			TargetPort: intstr.FromString("metrics"),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	// If no channel ports were added, expose the primary service port as a
	// generic "http" port so the service is always non-empty.
	if len(ports) == 0 {
		ports = append(ports, corev1.ServicePort{
			Name:       "http",
			Port:       primaryPort,
			TargetPort: intstr.FromInt32(primaryPort),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	return ports
}
