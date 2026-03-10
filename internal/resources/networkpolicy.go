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
	networkingv1 "k8s.io/api/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

// BuildNetworkPolicy creates a NetworkPolicy for the OpenTalonInstance.
//
// Default behaviour:
//   - Deny all ingress except on explicitly configured ports.
//   - Allow egress on port 53 (DNS) to support provider API resolution.
//   - Allow egress on port 443 (HTTPS) for LLM provider API calls.
//   - User-supplied extra ingress/egress rules are merged in.
func BuildNetworkPolicy(instance *v1alpha1.OpenTalonInstance) *networkingv1.NetworkPolicy {
	npSpec := instance.Spec.Networking.NetworkPolicy

	ingressRules := buildDefaultIngressRules(instance)
	ingressRules = append(ingressRules, npSpec.IngressRules...)

	egressRules := buildDefaultEgressRules()
	egressRules = append(egressRules, npSpec.EgressRules...)

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: SelectorLabels(instance),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress:  egressRules,
		},
	}
}

// buildDefaultIngressRules builds the baseline ingress allow rules based on
// which channels are enabled in the spec.
func buildDefaultIngressRules(instance *v1alpha1.OpenTalonInstance) []networkingv1.NetworkPolicyIngressRule {
	var rules []networkingv1.NetworkPolicyIngressRule

	if instance.Spec.Config.Channels != nil {
		// Webhook ingress.
		if wh := instance.Spec.Config.Channels.Webhook; wh != nil && wh.Enabled {
			port := wh.Port
			if port == 0 {
				port = 8080
			}
			rules = append(rules, networkingv1.NetworkPolicyIngressRule{
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: protocolTCP(),
						Port:     portPtr(port),
					},
				},
			})
		}

		// WebSocket ingress.
		if ws := instance.Spec.Config.Channels.WebSocket; ws != nil && ws.Enabled {
			port := ws.Port
			if port == 0 {
				port = 8081
			}
			rules = append(rules, networkingv1.NetworkPolicyIngressRule{
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: protocolTCP(),
						Port:     portPtr(port),
					},
				},
			})
		}
	}

	// Metrics ingress — allow Prometheus scraping from anywhere in the cluster.
	if instance.Spec.Observability.Metrics.Enabled {
		metricsPort := instance.Spec.Observability.Metrics.Port
		if metricsPort == 0 {
			metricsPort = 9090
		}
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: protocolTCP(),
					Port:     portPtr(metricsPort),
				},
			},
		})
	}

	return rules
}

// buildDefaultEgressRules returns baseline egress rules that allow DNS lookups
// and outbound HTTPS traffic required to reach LLM provider APIs.
func buildDefaultEgressRules() []networkingv1.NetworkPolicyEgressRule {
	tcpProto := corev1.ProtocolTCP
	udpProto := corev1.ProtocolUDP

	dnsTCP := intstr.FromInt32(53)
	dnsUDP := intstr.FromInt32(53)
	httpsPort := intstr.FromInt32(443)
	httpPort := intstr.FromInt32(80)

	return []networkingv1.NetworkPolicyEgressRule{
		// DNS resolution (both TCP and UDP).
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcpProto, Port: &dnsTCP},
				{Protocol: &udpProto, Port: &dnsUDP},
			},
		},
		// HTTPS to LLM provider APIs.
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcpProto, Port: &httpsPort},
				{Protocol: &tcpProto, Port: &httpPort},
			},
		},
	}
}

func protocolTCP() *corev1.Protocol {
	p := corev1.ProtocolTCP
	return &p
}

func portPtr(port int32) *intstr.IntOrString {
	v := intstr.FromInt32(port)
	return &v
}
