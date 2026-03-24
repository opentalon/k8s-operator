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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

const (
	// ConfigHashAnnotation is the annotation key used to trigger rolling updates on config changes.
	ConfigHashAnnotation = "opentalon.io/config-hash"

	// DataVolumeName is the name of the persistent data volume.
	DataVolumeName = "data"

	// ConfigVolumeName is the name of the config.yaml volume.
	ConfigVolumeName = "config"

	// TmpVolumeName is the name of the writable /tmp emptyDir volume.
	TmpVolumeName = "tmp"

	// ConfigMountPath is where config.yaml is mounted inside the container.
	ConfigMountPath = "/etc/opentalon"

	// DataMountPath is where the SQLite database and workspace are mounted.
	DataMountPath = "/data"

	// TmpMountPath is the writable scratch space.
	TmpMountPath = "/tmp"
)

// BuildStatefulSet constructs the StatefulSet for the given OpenTalonInstance.
// configHash is the SHA-256 digest of the current ConfigMap data and is written
// to the pod template annotation so that config changes trigger a rolling restart.
func BuildStatefulSet(instance *v1alpha1.OpenTalonInstance, configHash string) *appsv1.StatefulSet {
	labels := Labels(instance)
	selector := SelectorLabels(instance)

	replicas := int32(1)
	if instance.Spec.Replicas != nil {
		replicas = *instance.Spec.Replicas
	}

	imageRef := ImageRef(instance.Spec.Image)
	pullPolicy := instance.Spec.Image.PullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullIfNotPresent
	}

	container := buildMainContainer(instance, imageRef, pullPolicy)
	volumes := buildVolumes(instance)
	podSecCtx := buildPodSecurityContext(instance)

	podAnnotations := map[string]string{
		ConfigHashAnnotation: configHash,
	}

	// Merge user-supplied pod annotations if any are set on the instance.
	for k, v := range instance.Annotations {
		if k != ConfigHashAnnotation {
			podAnnotations[k] = v
		}
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            &replicas,
			ServiceName:         ResourceName(instance),
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:           serviceAccountName(instance),
					AutomountServiceAccountToken: boolPtr(true),
					SecurityContext:              podSecCtx,
					ImagePullSecrets:             instance.Spec.Image.PullSecrets,
					NodeSelector:                 instance.Spec.NodeSelector,
					Tolerations:                  instance.Spec.Tolerations,
					Affinity:                     instance.Spec.Affinity,
					InitContainers:               instance.Spec.InitContainers,
					Containers:                   append([]corev1.Container{container}, instance.Spec.AdditionalContainers...),
					Volumes:                      volumes,
				},
			},
		},
	}

	// Attach VolumeClaimTemplate for persistent data when PVC is enabled and
	// no existing claim is referenced (existing claims are mounted as a volume).
	if instance.Spec.Storage.Persistence.Enabled && instance.Spec.Storage.Persistence.ExistingClaim == "" {
		sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			buildVolumeClaimTemplate(instance),
		}
	}

	return sts
}

// buildMainContainer assembles the primary OpenTalon container spec.
func buildMainContainer(
	instance *v1alpha1.OpenTalonInstance,
	imageRef string,
	pullPolicy corev1.PullPolicy,
) corev1.Container {
	resources := defaultResources()
	if instance.Spec.Resources.Requests != nil || instance.Spec.Resources.Limits != nil {
		resources = instance.Spec.Resources
	}

	containerSecCtx := buildContainerSecurityContext(instance)

	ports := buildContainerPorts(instance)

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      ConfigVolumeName,
			MountPath: ConfigMountPath,
			ReadOnly:  true,
		},
		{
			Name:      DataVolumeName,
			MountPath: DataMountPath,
		},
		{
			Name:      TmpVolumeName,
			MountPath: TmpMountPath,
		},
	}

	container := corev1.Container{
		Name:            "opentalon",
		Image:           imageRef,
		ImagePullPolicy: pullPolicy,
		Args:            []string{"-config", ConfigMountPath + "/config.yaml"},
		Ports:           ports,
		Env:             instance.Spec.Env,
		EnvFrom:         instance.Spec.EnvFrom,
		Resources:       resources,
		SecurityContext: containerSecCtx,
		VolumeMounts:    volumeMounts,
	}

	// Populate env vars for model API keys from secrets defined in the spec.
	for _, m := range instance.Spec.Config.Models {
		if m.APIKeySecret != nil {
			envName := apiKeyEnvVar(m.Provider)
			container.Env = append(container.Env, corev1.EnvVar{
				Name: envName,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: m.APIKeySecret,
				},
			})
		}
	}

	// Populate Slack token env vars from secrets defined in the Slack channel config.
	if instance.Spec.Config.Channels != nil && instance.Spec.Config.Channels.Slack != nil {
		slack := instance.Spec.Config.Channels.Slack
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "SLACK_BOT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &slack.TokenSecret,
			},
		})
		if slack.AppTokenSecret != nil {
			container.Env = append(container.Env, corev1.EnvVar{
				Name: "SLACK_APP_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: slack.AppTokenSecret,
				},
			})
		}
	}

	// Add liveness and readiness probes based on which channel is enabled.
	container.LivenessProbe = buildLivenessProbe(instance)
	container.ReadinessProbe = buildReadinessProbe(instance)

	return container
}

// buildLivenessProbe returns an appropriate liveness probe.
// When a webhook or websocket port is available, an HTTP GET is used.
// Otherwise a process-exists exec probe is used.
func buildLivenessProbe(instance *v1alpha1.OpenTalonInstance) *corev1.Probe {
	if port := httpProbePort(instance); port > 0 {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 15,
			PeriodSeconds:       20,
			FailureThreshold:    3,
			TimeoutSeconds:      5,
		}
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "test -f /data/opentalon.db || true"},
			},
		},
		InitialDelaySeconds: 10,
		PeriodSeconds:       30,
		FailureThreshold:    3,
		TimeoutSeconds:      5,
	}
}

// buildReadinessProbe returns an appropriate readiness probe.
func buildReadinessProbe(instance *v1alpha1.OpenTalonInstance) *corev1.Probe {
	if port := httpProbePort(instance); port > 0 {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(port),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			FailureThreshold:    3,
			TimeoutSeconds:      5,
		}
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{"/bin/sh", "-c", "test -f /data/opentalon.db || true"},
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
		FailureThreshold:    3,
		TimeoutSeconds:      5,
	}
}

// httpProbePort returns the first available HTTP server port (webhook or metrics).
// Returns 0 if no HTTP port is configured.
func httpProbePort(instance *v1alpha1.OpenTalonInstance) int32 {
	if instance.Spec.Config.Channels != nil {
		if wh := instance.Spec.Config.Channels.Webhook; wh != nil && wh.Enabled {
			if wh.Port > 0 {
				return wh.Port
			}
			return 8080
		}
	}
	if instance.Spec.Observability.Metrics.Enabled != nil && *instance.Spec.Observability.Metrics.Enabled {
		p := instance.Spec.Observability.Metrics.Port
		if p > 0 {
			return p
		}
		return 9090
	}
	return 0
}

// buildContainerPorts produces the list of named container ports based on the spec.
func buildContainerPorts(instance *v1alpha1.OpenTalonInstance) []corev1.ContainerPort {
	var ports []corev1.ContainerPort

	if instance.Spec.Config.Channels != nil {
		if wh := instance.Spec.Config.Channels.Webhook; wh != nil && wh.Enabled {
			p := wh.Port
			if p == 0 {
				p = 8080
			}
			ports = append(ports, corev1.ContainerPort{Name: "webhook", ContainerPort: p, Protocol: corev1.ProtocolTCP})
		}
		if ws := instance.Spec.Config.Channels.WebSocket; ws != nil && ws.Enabled {
			p := ws.Port
			if p == 0 {
				p = 8081
			}
			ports = append(ports, corev1.ContainerPort{Name: "websocket", ContainerPort: p, Protocol: corev1.ProtocolTCP})
		}
	}

	if instance.Spec.Observability.Metrics.Enabled != nil && *instance.Spec.Observability.Metrics.Enabled {
		p := instance.Spec.Observability.Metrics.Port
		if p == 0 {
			p = 9090
		}
		ports = append(ports, corev1.ContainerPort{Name: "metrics", ContainerPort: p, Protocol: corev1.ProtocolTCP})
	}

	return ports
}

// buildVolumes returns the pod volumes (ConfigMap mount + data volume if using emptyDir or existingClaim).
func buildVolumes(instance *v1alpha1.OpenTalonInstance) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: ConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ConfigMapName(instance),
					},
					DefaultMode: int32Ptr(0444),
				},
			},
		},
		{
			Name: TmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	persistence := instance.Spec.Storage.Persistence
	if !persistence.Enabled {
		// Use emptyDir when persistence is disabled.
		volumes = append(volumes, corev1.Volume{
			Name: DataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	} else if persistence.ExistingClaim != "" {
		// Reference a pre-existing PVC.
		volumes = append(volumes, corev1.Volume{
			Name: DataVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: persistence.ExistingClaim,
				},
			},
		})
	}
	// When persistence is enabled and no existingClaim is set, the volume is
	// provided by the VolumeClaimTemplate on the StatefulSet.

	return volumes
}

// buildVolumeClaimTemplate builds the PVC template for the StatefulSet.
func buildVolumeClaimTemplate(instance *v1alpha1.OpenTalonInstance) corev1.PersistentVolumeClaim {
	persistence := instance.Spec.Storage.Persistence

	size := persistence.Size
	if size.IsZero() {
		size = resource.MustParse("1Gi")
	}

	accessModes := persistence.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   DataVolumeName,
			Labels: Labels(instance),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	if persistence.StorageClassName != nil {
		pvc.Spec.StorageClassName = persistence.StorageClassName
	}

	return pvc
}

// buildPodSecurityContext constructs the pod-level security context.
// The user-supplied PodSecurityContext overrides the defaults if set.
func buildPodSecurityContext(instance *v1alpha1.OpenTalonInstance) *corev1.PodSecurityContext {
	if instance.Spec.Security.PodSecurityContext != nil {
		return instance.Spec.Security.PodSecurityContext
	}

	runAsUser := int64(1000)
	runAsGroup := int64(1000)
	fsGroup := int64(1000)

	if instance.Spec.Security.RunAsUser != nil {
		runAsUser = *instance.Spec.Security.RunAsUser
		fsGroup = runAsUser
	}
	if instance.Spec.Security.RunAsGroup != nil {
		runAsGroup = *instance.Spec.Security.RunAsGroup
	}

	nonRoot := true
	return &corev1.PodSecurityContext{
		RunAsNonRoot: &nonRoot,
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsGroup,
		FSGroup:      &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// buildContainerSecurityContext constructs the container-level security context.
// The user-supplied ContainerSecurityContext overrides the defaults if set.
func buildContainerSecurityContext(instance *v1alpha1.OpenTalonInstance) *corev1.SecurityContext {
	if instance.Spec.Security.ContainerSecurityContext != nil {
		return instance.Spec.Security.ContainerSecurityContext
	}

	readOnly := true
	if instance.Spec.Security.ReadOnlyRootFilesystem != nil {
		readOnly = *instance.Spec.Security.ReadOnlyRootFilesystem
	}

	allowPrivilegeEscalation := false
	nonRoot := true

	return &corev1.SecurityContext{
		ReadOnlyRootFilesystem:   &readOnly,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		RunAsNonRoot:             &nonRoot,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// serviceAccountName returns the service account name the pod should use.
func serviceAccountName(instance *v1alpha1.OpenTalonInstance) string {
	if instance.Spec.ServiceAccountName != "" {
		return instance.Spec.ServiceAccountName
	}
	return ResourceName(instance)
}

// defaultResources returns sensible default resource requests/limits.
func defaultResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool { return &b }

// int32Ptr returns a pointer to i.
func int32Ptr(i int32) *int32 { return &i }
