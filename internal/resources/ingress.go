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
	"fmt"

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

// WebSocketIngressName returns the name of the dedicated WebSocket Ingress.
func WebSocketIngressName(instance *v1alpha1.OpenTalonInstance) string {
	return ResourceName(instance) + "-ws"
}

// BuildWebSocketIngress creates a dedicated Ingress for the WebSocket channel.
// It merges default WebSocket-friendly annotations (long proxy timeouts) with
// user-supplied annotations from the channel's ingress spec.
func BuildWebSocketIngress(instance *v1alpha1.OpenTalonInstance) *networkingv1.Ingress {
	ws := instance.Spec.Config.Channels.WebSocket
	wsIngress := ws.Ingress

	wsPort := ws.Port
	if wsPort == 0 {
		wsPort = 8081
	}
	wsPath := ws.Path
	if wsPath == "" {
		wsPath = "/ws"
	}

	// Default WebSocket annotations for nginx; user annotations can override.
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout":  "3600",
		"nginx.ingress.kubernetes.io/proxy-send-timeout":  "3600",
	}
	for k, v := range wsIngress.Annotations {
		annotations[k] = v
	}

	pathType := networkingv1.PathTypeExact

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        WebSocketIngressName(instance),
			Namespace:   instance.Namespace,
			Labels:      Labels(instance),
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: wsIngress.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: wsIngress.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     wsPath,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: ResourceName(instance),
											Port: networkingv1.ServiceBackendPort{
												Number: wsPort,
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

	if wsIngress.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{wsIngress.Host},
				SecretName: wsIngress.TLSSecretName,
			},
		}
	}

	return ingress
}

// PluginIngressName returns the name of the dedicated Ingress for a plugin.
func PluginIngressName(instance *v1alpha1.OpenTalonInstance, pluginName string) string {
	return fmt.Sprintf("%s-plugin-%s", ResourceName(instance), pluginName)
}

// BuildPluginIngress creates a dedicated Ingress for a plugin's HTTP endpoint.
// It rewrites the path prefix so the plugin receives requests at its root.
func BuildPluginIngress(instance *v1alpha1.OpenTalonInstance, name string, plugin v1alpha1.PluginConfig) *networkingv1.Ingress {
	pi := plugin.Ingress

	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/rewrite-target": "/$2",
	}
	for k, v := range pi.Annotations {
		annotations[k] = v
	}

	// Match /path and /path/... — the ($|/) boundary plus (.*) capture group
	// feeds the rewrite-target above.
	pathPattern := fmt.Sprintf("%s(/|$)(.*)", pi.Path)
	pathType := networkingv1.PathTypeImplementationSpecific

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        PluginIngressName(instance, name),
			Namespace:   instance.Namespace,
			Labels:      Labels(instance),
			Annotations: annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: pi.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: pi.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     pathPattern,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: ResourceName(instance),
											Port: networkingv1.ServiceBackendPort{
												Number: pi.Port,
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

	if pi.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{
				Hosts:      []string{pi.Host},
				SecretName: pi.TLSSecretName,
			},
		}
	}

	return ingress
}
