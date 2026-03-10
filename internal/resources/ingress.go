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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

// BuildIngress creates a Kubernetes Ingress resource for the OpenTalonInstance.
// The Ingress routes all traffic to the primary service port determined by the
// webhook channel configuration (or the default service port).
func BuildIngress(instance *v1alpha1.OpenTalonInstance) *networkingv1.Ingress {
	ingressSpec := instance.Spec.Networking.Ingress

	servicePort := instance.Spec.Networking.Service.Port
	if servicePort == 0 {
		servicePort = 8080
	}
	// Prefer the webhook port when available.
	if instance.Spec.Config.Channels != nil {
		if wh := instance.Spec.Config.Channels.Webhook; wh != nil && wh.Enabled {
			if wh.Port > 0 {
				servicePort = wh.Port
			}
		}
	}

	pathType := networkingv1.PathTypePrefix
	path := "/"

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ResourceName(instance),
			Namespace:   instance.Namespace,
			Labels:      Labels(instance),
			Annotations: ingressSpec.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ingressSpec.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: ingressSpec.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: ResourceName(instance),
											Port: networkingv1.ServiceBackendPort{
												Number: servicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Configure TLS when a secret name is provided.
	if ingressSpec.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{ingressSpec.Host},
				SecretName: ingressSpec.TLSSecretName,
			},
		}
	}

	return ingress
}
