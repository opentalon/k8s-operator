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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

const (
	// ChromeLoginSuffix is appended to the instance name for all chrome-login child resources
	// (Secret, Service, Ingress) so they share a predictable, consistent naming scheme.
	ChromeLoginSuffix = "-chrome-login"

	// ChromeLoginVNCContainerName is the sidecar container name inside the StatefulSet pod.
	ChromeLoginVNCContainerName = "chrome-login"

	// ChromeLoginPasswordKey is the key in the Secret that holds the VNC password.
	ChromeLoginPasswordKey = "password"

)

// ChromeLoginSecretName returns the name of the Secret holding the VNC password.
func ChromeLoginSecretName(instance *v1alpha1.OpenTalonInstance) string {
	return instance.Name + ChromeLoginSuffix
}

// ChromeLoginServiceName returns the name of the Service for the VNC + CDP ports.
func ChromeLoginServiceName(instance *v1alpha1.OpenTalonInstance) string {
	return instance.Name + ChromeLoginSuffix
}

// ChromeLoginIngressName returns the name of the Ingress for the VNC web UI.
func ChromeLoginIngressName(instance *v1alpha1.OpenTalonInstance) string {
	return instance.Name + ChromeLoginSuffix
}

// ChromeLoginLabels returns the label set for chrome-login child resources.
func ChromeLoginLabels(instance *v1alpha1.OpenTalonInstance) map[string]string {
	l := Labels(instance)
	l["app.kubernetes.io/component"] = "chrome-login"
	return l
}

// BuildChromeLoginSecret returns a Secret that stores the noVNC session password.
// Callers must only invoke this when the Secret does not already exist; the
// controller never rotates the password in place.
func BuildChromeLoginSecret(instance *v1alpha1.OpenTalonInstance, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ChromeLoginSecretName(instance),
			Namespace: instance.Namespace,
			Labels:    ChromeLoginLabels(instance),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			ChromeLoginPasswordKey: password,
		},
	}
}

// BuildChromeLoginService creates a ClusterIP Service that exposes the noVNC web UI
// and the Chrome DevTools Protocol endpoint of the VNC Chrome sidecar.
func BuildChromeLoginService(instance *v1alpha1.OpenTalonInstance) *corev1.Service {
	cl := instance.Spec.ChromeLogin

	vncPort := cl.VNCPort
	if vncPort == 0 {
		vncPort = 3000
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ChromeLoginServiceName(instance),
			Namespace: instance.Namespace,
			Labels:    ChromeLoginLabels(instance),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: SelectorLabels(instance), // selects the same StatefulSet pods
			Ports: []corev1.ServicePort{
				{
					Name:       "vnc",
					Port:       vncPort,
					TargetPort: intstr.FromInt32(vncPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// BuildChromeLoginIngress creates an Ingress resource that exposes the noVNC web UI
// at the configured host with optional TLS/cert-manager support.
// Returns nil if no Ingress spec is provided.
func BuildChromeLoginIngress(instance *v1alpha1.OpenTalonInstance) *networkingv1.Ingress {
	cl := instance.Spec.ChromeLogin
	if cl == nil || cl.Ingress == nil || !cl.Ingress.Enabled {
		return nil
	}
	ingressSpec := cl.Ingress

	vncPort := cl.VNCPort
	if vncPort == 0 {
		vncPort = 3000
	}

	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ChromeLoginIngressName(instance),
			Namespace:   instance.Namespace,
			Labels:      ChromeLoginLabels(instance),
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
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: ChromeLoginServiceName(instance),
											Port: networkingv1.ServiceBackendPort{
												Number: vncPort,
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

	// Add TLS block when a secret name is provided — enables cert-manager / Let's Encrypt.
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

// ChromeLoginSidecarContainer builds the container spec for the linuxserver/chromium
// VNC sidecar. It is added to the StatefulSet pod alongside the main OpenTalon container.
func ChromeLoginSidecarContainer(instance *v1alpha1.OpenTalonInstance) corev1.Container {
	cl := instance.Spec.ChromeLogin

	image := cl.Image
	if image == "" {
		image = "lscr.io/linuxserver/chromium:latest"
	}
	vncPort := cl.VNCPort
	if vncPort == 0 {
		vncPort = 3000
	}
	cdpPort := cl.CDPPort
	if cdpPort == 0 {
		cdpPort = 9222
	}

	res := cl.Resources
	if res.Requests == nil && res.Limits == nil {
		res = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}
	}

	falseVal := false
	sc := cl.SecurityContext
	if sc == nil {
		sc = &corev1.SecurityContext{
			AllowPrivilegeEscalation: &falseVal,
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		}
	}

	return corev1.Container{
		Name:  ChromeLoginVNCContainerName,
		Image: image,
		Env: []corev1.EnvVar{
			{Name: "CUSTOM_USER", Value: "opentalon"},
			{
				Name: "PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: ChromeLoginSecretName(instance),
						},
						Key: ChromeLoginPasswordKey,
					},
				},
			},
			// Enable Chrome remote debugging so opentalon-chrome can capture cookies via CDP.
			{Name: "CHROME_CLI", Value: "--remote-debugging-port=9222 --remote-debugging-address=0.0.0.0 --no-sandbox"},
		},
		Ports: []corev1.ContainerPort{
			{Name: "vnc", ContainerPort: vncPort, Protocol: corev1.ProtocolTCP},
			{Name: "cdp", ContainerPort: cdpPort, Protocol: corev1.ProtocolTCP},
		},
		// Mount the Chrome profile directory from the shared data PVC so that cookies
		// and session state survive pod restarts. linuxserver/chromium stores its
		// profile under /config; the subpath keeps it isolated from the main container.
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      DataVolumeName,
				MountPath: "/config",
				SubPath:   "chrome-login",
			},
		},
		Resources:       res,
		SecurityContext: sc,
	}
}

// ChromeLoginURL returns the public URL for the Chrome+noVNC session derived from the
// ingress spec, or an empty string when no ingress is configured or enabled.
func ChromeLoginURL(cl *v1alpha1.ChromeLoginSpec) string {
	if cl == nil || cl.Ingress == nil || !cl.Ingress.Enabled || cl.Ingress.Host == "" {
		return ""
	}
	scheme := "http"
	if cl.Ingress.TLSSecretName != "" {
		scheme = "https"
	}
	return scheme + "://" + cl.Ingress.Host
}

// ChromeLoginEnvVars returns the environment variables to inject into the main OpenTalon
// container so it (and the opentalon-chrome plugin) can tell users the VNC URL and
// connect to the interactive Chrome's CDP endpoint.
func ChromeLoginEnvVars(instance *v1alpha1.OpenTalonInstance) []corev1.EnvVar {
	cl := instance.Spec.ChromeLogin

	cdpPort := cl.CDPPort
	if cdpPort == 0 {
		cdpPort = 9222
	}

	loginURL := ChromeLoginURL(cl)

	vars := []corev1.EnvVar{
		// CDP URL for the opentalon-chrome plugin to connect to the VNC Chrome.
		// The sidecar runs in the same pod so localhost works.
		{Name: "CHROME_LOGIN_CDP_URL", Value: "http://localhost:" + strconv.Itoa(int(cdpPort))},
		// Path for the credential SQLite DB (reuses the existing persistent data volume).
		{Name: "CHROME_DATA_DIR", Value: DataMountPath + "/chrome-credentials"},
		// Password for the VNC session — forwarded to the user by the agent.
		{
			Name: "CHROME_LOGIN_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ChromeLoginSecretName(instance),
					},
					Key: ChromeLoginPasswordKey,
				},
			},
		},
	}

	if loginURL != "" {
		vars = append(vars, corev1.EnvVar{Name: "CHROME_LOGIN_URL", Value: loginURL})
	}

	return vars
}
