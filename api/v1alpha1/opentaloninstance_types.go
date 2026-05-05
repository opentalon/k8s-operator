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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase constants represent the lifecycle phase of an OpenTalonInstance.
const (
	PhasePending      = "Pending"
	PhaseProvisioning = "Provisioning"
	PhaseRunning      = "Running"
	PhaseDegraded     = "Degraded"
	PhaseFailed       = "Failed"
	PhaseTerminating  = "Terminating"
)

// Condition type constants track the readiness of individual managed resources.
const (
	ConditionStatefulSetReady    = "StatefulSetReady"
	ConditionConfigMapReady      = "ConfigMapReady"
	ConditionServiceReady        = "ServiceReady"
	ConditionPVCReady            = "PVCReady"
	ConditionRBACReady           = "RBACReady"
	ConditionNetworkPolicyReady  = "NetworkPolicyReady"
	ConditionServiceMonitorReady = "ServiceMonitorReady"
)

// OpenTalonInstanceSpec defines the desired state of an OpenTalonInstance.
type OpenTalonInstanceSpec struct {
	// Image configuration.
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Config provides inline OpenTalon configuration sections.
	// +optional
	Config ConfigSpec `json:"config,omitempty"`

	// ConfigFrom references an existing ConfigMap containing a complete config.yaml.
	// When set, the inline Config field is ignored.
	// +optional
	ConfigFrom *corev1.LocalObjectReference `json:"configFrom,omitempty"`

	// EnvFrom injects environment variables from ConfigMaps or Secrets into the main container.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Env sets individual environment variables in the main container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources sets resource requests and limits for the main container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configures SQLite and workspace persistence.
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Networking configures service exposure and ingress.
	// +optional
	Networking NetworkingSpec `json:"networking,omitempty"`

	// Security configures pod and container security contexts.
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// Observability configures metrics endpoints and ServiceMonitors.
	// +optional
	Observability ObservabilitySpec `json:"observability,omitempty"`

	// Availability configures HPA and PDB settings.
	// +optional
	Availability AvailabilitySpec `json:"availability,omitempty"`

	// AutoUpdate enables and configures automatic image updates.
	// +optional
	AutoUpdate AutoUpdateSpec `json:"autoUpdate,omitempty"`

	// AdditionalContainers defines extra sidecar containers to run alongside the main container.
	// +optional
	AdditionalContainers []corev1.Container `json:"additionalContainers,omitempty"`

	// InitContainers defines extra init containers to run before the main container starts.
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// Replicas sets the number of OpenTalon instance pods (default 1).
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// NodeSelector constrains pod scheduling to nodes matching these labels.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations allow the pod to tolerate node taints.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity provides advanced scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// PodAnnotations sets additional annotations on the pod template.
	// Useful for Prometheus scrape annotations or other integrations.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// ServiceAccountName sets a custom service account for the pod.
	// When empty the operator creates and manages a dedicated service account.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// ChromeLogin deploys an interactive Chrome+VNC sidecar for cookie-capture
	// login sessions. Users open the VNC URL in their browser to log into
	// third-party services; opentalon-chrome then captures the session cookies via CDP.
	// +optional
	ChromeLogin *ChromeLoginSpec `json:"chromeLogin,omitempty"`

	// AdditionalVolumes defines extra volumes to add to the pod.
	// +optional
	AdditionalVolumes []corev1.Volume `json:"additionalVolumes,omitempty"`

	// AdditionalVolumeMounts defines extra volume mounts to add to the main container.
	// +optional
	AdditionalVolumeMounts []corev1.VolumeMount `json:"additionalVolumeMounts,omitempty"`
}

// ImageSpec configures the OpenTalon container image.
type ImageSpec struct {
	// Repository is the image repository.
	// +optional
	// +kubebuilder:default="ghcr.io/opentalon/opentalon"
	Repository string `json:"repository,omitempty"`

	// Tag is the image tag.
	// +optional
	// +kubebuilder:default="latest"
	Tag string `json:"tag,omitempty"`

	// Digest overrides Tag with an exact image digest (e.g. sha256:abc123…).
	// +optional
	Digest string `json:"digest,omitempty"`

	// PullPolicy controls when the image is pulled.
	// +optional
	// +kubebuilder:default="IfNotPresent"
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// PullSecrets lists image pull secrets.
	// +optional
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// ConfigSpec contains inline OpenTalon configuration that is rendered into config.yaml.
type ConfigSpec struct {
	// Models lists LLM model configurations.
	// +optional
	Models []ModelConfig `json:"models,omitempty"`

	// Routing configures model selection and fallback behaviour.
	// +optional
	Routing *RoutingConfig `json:"routing,omitempty"`

	// Channels configures communication channels (console, Slack, webhook, WebSocket).
	// +optional
	Channels *ChannelsConfig `json:"channels,omitempty"`

	// Plugins configures Kubernetes-level resources for plugins, keyed by plugin name.
	// +optional
	Plugins map[string]PluginConfig `json:"plugins,omitempty"`

	// State configures SQLite-backed session persistence.
	// +optional
	State *StateConfig `json:"state,omitempty"`

	// Logging configures log output format and verbosity.
	// +optional
	Logging *LoggingConfig `json:"logging,omitempty"`

	// ExtraConfig is additional raw YAML that is merged verbatim into config.yaml.
	// +optional
	ExtraConfig string `json:"extraConfig,omitempty"`
}

// ModelConfig configures an LLM provider and model.
type ModelConfig struct {
	// Name is the model identifier used in routing rules.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Provider is the LLM provider (e.g. anthropic, openai, deepseek, ollama).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// APIKeySecret references the Kubernetes Secret key that holds the API key.
	// +optional
	APIKeySecret *corev1.SecretKeySelector `json:"apiKeySecret,omitempty"`

	// BaseURL overrides the provider's default API endpoint URL.
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// ContextWindow sets the model's maximum context size in tokens.
	// +optional
	ContextWindow int64 `json:"contextWindow,omitempty"`

	// MaxTokens limits the maximum number of tokens in a response.
	// +optional
	MaxTokens int `json:"maxTokens,omitempty"`
}

// RoutingConfig configures model selection logic.
type RoutingConfig struct {
	// Primary is the name of the default model used for all requests.
	// +optional
	Primary string `json:"primary,omitempty"`

	// Fallbacks lists model names tried in order when the primary model fails.
	// +optional
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// ChannelsConfig configures the active communication channels.
type ChannelsConfig struct {
	// Console enables the interactive console (stdin/stdout) channel.
	// +optional
	Console *ConsoleChannelConfig `json:"console,omitempty"`

	// Slack configures Slack bot integration.
	// +optional
	Slack *SlackChannelConfig `json:"slack,omitempty"`

	// Webhook configures inbound HTTP webhook ingestion.
	// +optional
	Webhook *WebhookChannelConfig `json:"webhook,omitempty"`

	// WebSocket configures a WebSocket server channel.
	// +optional
	WebSocket *WebSocketChannelConfig `json:"websocket,omitempty"`
}

// ConsoleChannelConfig configures the interactive console channel.
type ConsoleChannelConfig struct {
	// Enabled enables the console channel.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`
}

// SlackChannelConfig configures the Slack bot channel.
type SlackChannelConfig struct {
	// Enabled enables the Slack channel.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// TokenSecret references the Secret key that holds the Slack bot token (xoxb-…).
	// +kubebuilder:validation:Required
	TokenSecret corev1.SecretKeySelector `json:"tokenSecret"`

	// AppTokenSecret references the Secret key holding the Slack app-level token (xapp-…)
	// required for Socket Mode.
	// +optional
	AppTokenSecret *corev1.SecretKeySelector `json:"appTokenSecret,omitempty"`
}

// WebhookChannelConfig configures the HTTP webhook ingestion channel.
type WebhookChannelConfig struct {
	// Enabled enables the webhook channel.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Port is the TCP port the webhook server listens on.
	// +optional
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Path is the HTTP path at which the webhook endpoint is served.
	// +optional
	// +kubebuilder:default="/webhook"
	Path string `json:"path,omitempty"`
}

// WebSocketChannelConfig configures the WebSocket server channel.
type WebSocketChannelConfig struct {
	// Enabled enables the WebSocket channel.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Port is the TCP port the WebSocket server listens on.
	// +optional
	// +kubebuilder:default=8081
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Path is the HTTP path at which the WebSocket endpoint is served.
	// +optional
	// +kubebuilder:default="/ws"
	Path string `json:"path,omitempty"`

	// CORSOrigins lists the allowed CORS origins for WebSocket connections.
	// An empty list allows all origins (dev mode).
	// +optional
	CORSOrigins []string `json:"corsOrigins,omitempty"`

	// Ingress configures a dedicated Ingress for the WebSocket endpoint.
	// When omitted the WebSocket is only reachable cluster-internally.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`
}

// PluginConfig configures a plugin's Kubernetes-level resources (ingress, service port).
// The plugin's runtime configuration (source, github/ref, dial_timeout, config map)
// belongs in the OpenTalon config.yaml — either via configFrom (external ConfigMap)
// or via spec.config.extraConfig.
type PluginConfig struct {
	// Ingress configures a dedicated Ingress for the plugin's HTTP endpoint.
	// The plugin must expose an HTTP server (e.g. via http_addr in its config).
	// +optional
	Ingress *PluginIngressSpec `json:"ingress,omitempty"`
}

// PluginIngressSpec configures an Ingress for a plugin's HTTP endpoint.
type PluginIngressSpec struct {
	// Enabled enables Ingress creation for this plugin.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Host is the hostname routed to the plugin.
	// +optional
	Host string `json:"host,omitempty"`

	// Path is the HTTP path prefix routed to the plugin (e.g. "/weaviate").
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Port is the container port the plugin's HTTP server listens on.
	// This must match the port configured in the plugin's config (e.g. http_addr: ":8082").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// TLSSecretName references a TLS Secret for HTTPS termination.
	// +optional
	TLSSecretName string `json:"tlsSecretName,omitempty"`

	// ClassName sets the IngressClass name.
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Annotations sets annotations on the Ingress resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// StateConfig configures SQLite-backed session persistence.
type StateConfig struct {
	// Path is the filesystem path for the SQLite database file.
	// +optional
	// +kubebuilder:default="/data/opentalon.db"
	Path string `json:"path,omitempty"`

	// MaxMessages caps the number of stored messages per session. Zero means unlimited.
	// +optional
	MaxMessages *int `json:"maxMessages,omitempty"`

	// IdleSessionTTL is a Go duration string (e.g. "24h") after which idle sessions are pruned.
	// +optional
	IdleSessionTTL string `json:"idleSessionTTL,omitempty"`

	// Summarize enables automatic conversation summarization when approaching the context window.
	// +optional
	Summarize bool `json:"summarize,omitempty"`
}

// LoggingConfig configures log output.
type LoggingConfig struct {
	// Level sets the minimum log level.
	// +optional
	// +kubebuilder:default="info"
	// +kubebuilder:validation:Enum=debug;info;warn;error
	Level string `json:"level,omitempty"`

	// Format sets the log output format.
	// +optional
	// +kubebuilder:default="json"
	// +kubebuilder:validation:Enum=json;text
	Format string `json:"format,omitempty"`
}

// StorageSpec configures data persistence for the OpenTalon instance.
type StorageSpec struct {
	// Persistence configures the PersistentVolumeClaim mounted at /data.
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`
}

// PersistenceSpec configures PVC-based persistent storage.
type PersistenceSpec struct {
	// Enabled enables the PVC. When false an emptyDir is used instead.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Size is the requested PVC storage capacity.
	// +optional
	// +kubebuilder:default="1Gi"
	Size resource.Quantity `json:"size,omitempty"`

	// StorageClassName sets the StorageClass for the PVC.
	// When nil the cluster's default StorageClass is used.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// ExistingClaim references a pre-existing PVC to use instead of creating a new one.
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`

	// AccessModes sets the PVC access modes.
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
}

// NetworkingSpec configures how the OpenTalon instance is exposed.
type NetworkingSpec struct {
	// Service configures the Kubernetes Service resource.
	// +optional
	Service ServiceSpec `json:"service,omitempty"`

	// Ingress configures an optional Ingress resource.
	// +optional
	Ingress IngressSpec `json:"ingress,omitempty"`

	// NetworkPolicy configures NetworkPolicy isolation.
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`
}

// ServiceSpec configures the Kubernetes Service.
type ServiceSpec struct {
	// Type sets the Service type.
	// +optional
	// +kubebuilder:default="ClusterIP"
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer;ExternalName
	Type corev1.ServiceType `json:"type,omitempty"`

	// Port is the primary service port exposed.
	// +optional
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Annotations sets annotations on the Service resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels sets additional labels on the Service resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// IngressSpec configures an optional Kubernetes Ingress resource.
type IngressSpec struct {
	// Enabled enables Ingress creation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ClassName sets the IngressClass name.
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Host is the hostname routed to the OpenTalon service.
	// +optional
	Host string `json:"host,omitempty"`

	// TLSSecretName references a TLS Secret for HTTPS termination.
	// +optional
	TLSSecretName string `json:"tlsSecretName,omitempty"`

	// Annotations sets annotations on the Ingress resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NetworkPolicySpec configures a NetworkPolicy for the instance.
type NetworkPolicySpec struct {
	// Enabled enables NetworkPolicy creation (default: false).
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// IngressRules are additional NetworkPolicy ingress rules merged with the defaults.
	// +optional
	IngressRules []networkingv1.NetworkPolicyIngressRule `json:"ingressRules,omitempty"`

	// EgressRules are additional NetworkPolicy egress rules merged with the defaults.
	// +optional
	EgressRules []networkingv1.NetworkPolicyEgressRule `json:"egressRules,omitempty"`
}

// SecuritySpec configures security contexts for the pod and main container.
type SecuritySpec struct {
	// PodSecurityContext sets the pod-level security context.
	// When set it overrides the operator's default pod security context.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// ContainerSecurityContext sets the container-level security context.
	// When set it overrides the operator's default container security context.
	// +optional
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`

	// RunAsUser sets the UID under which the container process runs.
	// +optional
	// +kubebuilder:default=1000
	RunAsUser *int64 `json:"runAsUser,omitempty"`

	// RunAsGroup sets the primary GID under which the container process runs.
	// +optional
	// +kubebuilder:default=1000
	RunAsGroup *int64 `json:"runAsGroup,omitempty"`

	// ReadOnlyRootFilesystem makes the container root filesystem read-only.
	// +optional
	// +kubebuilder:default=true
	ReadOnlyRootFilesystem *bool `json:"readOnlyRootFilesystem,omitempty"`
}

// ObservabilitySpec configures metrics and monitoring integration.
type ObservabilitySpec struct {
	// Metrics configures the Prometheus metrics endpoint.
	// +optional
	Metrics MetricsSpec `json:"metrics,omitempty"`

	// Health configures the gRPC health probe server for Kubernetes
	// liveness, readiness, and startup probes.
	// +optional
	Health HealthSpec `json:"health,omitempty"`
}

// HealthSpec configures the gRPC health probe server.
type HealthSpec struct {
	// Port is the port on which the gRPC health service listens.
	// +optional
	// +kubebuilder:default=8086
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// StartupTimeoutSeconds is the maximum time to wait for all plugins
	// and channels to load before Kubernetes kills the pod. Set this based
	// on how many plugins compile at startup (e.g. 5 plugins x 7min = 2100s).
	// +optional
	// +kubebuilder:default=600
	// +kubebuilder:validation:Minimum=10
	StartupTimeoutSeconds int32 `json:"startupTimeoutSeconds,omitempty"`
}

// MetricsSpec configures the Prometheus metrics endpoint.
type MetricsSpec struct {
	// Enabled enables the metrics HTTP endpoint.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Port is the port on which metrics are served.
	// +optional
	// +kubebuilder:default=9090
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Path is the HTTP path at which metrics are served.
	// +optional
	// +kubebuilder:default="/metrics"
	Path string `json:"path,omitempty"`

	// ServiceMonitor configures a Prometheus Operator ServiceMonitor resource.
	// +optional
	ServiceMonitor ServiceMonitorSpec `json:"serviceMonitor,omitempty"`
}

// ServiceMonitorSpec configures a Prometheus Operator ServiceMonitor.
type ServiceMonitorSpec struct {
	// Enabled enables ServiceMonitor creation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Interval sets the Prometheus scrape interval.
	// +optional
	// +kubebuilder:default="30s"
	Interval string `json:"interval,omitempty"`

	// Labels sets additional labels on the ServiceMonitor, used for Prometheus discovery.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// AvailabilitySpec configures high-availability features.
type AvailabilitySpec struct {
	// PodDisruptionBudget configures a PodDisruptionBudget for the instance.
	// +optional
	PodDisruptionBudget PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`

	// HorizontalPodAutoscaler configures a HorizontalPodAutoscaler for the instance.
	// +optional
	HorizontalPodAutoscaler HPASpec `json:"horizontalPodAutoscaler,omitempty"`
}

// PodDisruptionBudgetSpec configures the PodDisruptionBudget.
type PodDisruptionBudgetSpec struct {
	// Enabled enables PDB creation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MinAvailable sets the minimum number of available pods during disruptions.
	// Mutually exclusive with MaxUnavailable.
	// +optional
	MinAvailable *int32 `json:"minAvailable,omitempty"`

	// MaxUnavailable sets the maximum number of unavailable pods during disruptions.
	// Mutually exclusive with MinAvailable.
	// +optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}

// HPASpec configures a HorizontalPodAutoscaler.
type HPASpec struct {
	// Enabled enables HPA creation.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MinReplicas sets the minimum replica count.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas sets the maximum replica count.
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// CPUUtilization sets the target average CPU utilization percentage.
	// +optional
	CPUUtilization *int32 `json:"cpuUtilization,omitempty"`

	// MemoryUtilization sets the target average memory utilization percentage.
	// +optional
	MemoryUtilization *int32 `json:"memoryUtilization,omitempty"`
}

// AutoUpdateSpec configures automatic image update checks.
type AutoUpdateSpec struct {
	// Enabled enables the automatic update controller.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Schedule is the cron expression controlling when update checks run.
	// +optional
	// +kubebuilder:default="0 2 * * *"
	Schedule string `json:"schedule,omitempty"`

	// AllowPrerelease permits updating to pre-release image tags.
	// +optional
	AllowPrerelease bool `json:"allowPrerelease,omitempty"`
}

// OpenTalonInstanceStatus defines the observed state of an OpenTalonInstance.
type OpenTalonInstanceStatus struct {
	// Phase is the current high-level lifecycle phase of the instance.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions tracks detailed reconciliation status for each managed resource.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the .metadata.generation of the OpenTalonInstance that was
	// last successfully reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the number of pods in the Ready state.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CurrentImage is the fully-qualified image reference currently running.
	// +optional
	CurrentImage string `json:"currentImage,omitempty"`

	// ManagedResources is the list of Kubernetes resource references owned by this operator.
	// +optional
	ManagedResources []string `json:"managedResources,omitempty"`

	// LastUpdateTime records when the status was last written.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// ChromeLoginURL is the public URL of the interactive VNC Chrome session,
	// populated when spec.chromeLogin is enabled and an Ingress host is configured.
	// +optional
	ChromeLoginURL string `json:"chromeLoginURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.readyReplicas
// +kubebuilder:resource:shortName=oti,categories=opentalon
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=".status.currentImage"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// OpenTalonInstance is the Schema for the opentaloninstances API.
// It represents a single deployment of the OpenTalon LLM orchestration platform.
type OpenTalonInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenTalonInstanceSpec   `json:"spec,omitempty"`
	Status OpenTalonInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenTalonInstanceList contains a list of OpenTalonInstance resources.
type OpenTalonInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenTalonInstance `json:"items"`
}

// ChromeLoginSpec configures an interactive Chrome+noVNC sidecar for cookie-capture login sessions.
type ChromeLoginSpec struct {
	// Image is the container image for the Chrome+noVNC sidecar.
	// lscr.io/linuxserver/chromium exposes noVNC on port 3000 and CDP on port 9222.
	// +optional
	// +kubebuilder:default="lscr.io/linuxserver/chromium:latest"
	Image string `json:"image,omitempty"`

	// VNCPort is the noVNC web UI port exposed by the sidecar (default: 3000).
	// +optional
	// +kubebuilder:default=3000
	VNCPort int32 `json:"vncPort,omitempty"`

	// CDPPort is the Chrome DevTools Protocol port inside the sidecar (default: 9222).
	// The Service exposes it as 9223 externally to avoid clashing with a separate
	// headless-shell sidecar that also uses 9222.
	// +optional
	// +kubebuilder:default=9222
	CDPPort int32 `json:"cdpPort,omitempty"`

	// Ingress configures external HTTPS access to the noVNC web UI.
	// Reuses IngressSpec so TLS/Let's Encrypt works the same way as the main ingress:
	//   tlsSecretName sets the TLS secret, annotations carry cert-manager directives.
	// When omitted the session is accessible only via kubectl port-forward.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Resources sets CPU/memory requests and limits for the Chrome+noVNC sidecar.
	// Defaults to 250m/512Mi requests and 1/1Gi limits when not specified.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// SecurityContext overrides the security context applied to the Chrome+noVNC sidecar.
	// Defaults to allowPrivilegeEscalation:false and seccompProfile:RuntimeDefault.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
}

func init() {
	SchemeBuilder.Register(&OpenTalonInstance{}, &OpenTalonInstanceList{})
}
