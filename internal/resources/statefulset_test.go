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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

func newStsInstance(name, namespace string) *v1alpha1.OpenTalonInstance {
	return &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestBuildStatefulSet_NotNil(t *testing.T) {
	inst := newStsInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")
	if sts == nil {
		t.Fatal("BuildStatefulSet() returned nil")
	}
}

func TestBuildStatefulSet_NameAndNamespace(t *testing.T) {
	inst := newStsInstance("my-bot", "production")
	sts := BuildStatefulSet(inst, "")

	if sts.Name != "my-bot" {
		t.Errorf("StatefulSet.Name = %q, want %q", sts.Name, "my-bot")
	}
	if sts.Namespace != "production" {
		t.Errorf("StatefulSet.Namespace = %q, want %q", sts.Namespace, "production")
	}
}

func TestBuildStatefulSet_SelectorMatchesPodTemplateLabels(t *testing.T) {
	inst := newStsInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	if sts.Spec.Selector == nil {
		t.Fatal("StatefulSet.Spec.Selector is nil")
	}
	for k, v := range sts.Spec.Selector.MatchLabels {
		if sts.Spec.Template.Labels[k] != v {
			t.Errorf("pod template label[%q] = %q, want %q", k, sts.Spec.Template.Labels[k], v)
		}
	}
}

func TestBuildStatefulSet_MainContainerImage(t *testing.T) {
	inst := newStsInstance("bot", "default")
	inst.Spec.Image = v1alpha1.ImageSpec{
		Repository: "example.io/myimage",
		Tag:        "v3.0.0",
	}
	sts := BuildStatefulSet(inst, "")

	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("no containers in pod template")
	}
	want := ImageRef(inst.Spec.Image)
	got := sts.Spec.Template.Spec.Containers[0].Image
	if got != want {
		t.Errorf("container image = %q, want %q", got, want)
	}
}

func TestBuildStatefulSet_SecurityContext(t *testing.T) {
	inst := newStsInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	// Pod-level security context.
	psc := sts.Spec.Template.Spec.SecurityContext
	if psc == nil {
		t.Fatal("pod security context is nil")
	}
	if psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
		t.Error("pod SecurityContext.RunAsNonRoot should be true")
	}

	// Container-level security context.
	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("no containers in pod template")
	}
	csc := sts.Spec.Template.Spec.Containers[0].SecurityContext
	if csc == nil {
		t.Fatal("container security context is nil")
	}
	if csc.ReadOnlyRootFilesystem == nil || !*csc.ReadOnlyRootFilesystem {
		t.Error("container SecurityContext.ReadOnlyRootFilesystem should be true")
	}
	if csc.RunAsNonRoot == nil || !*csc.RunAsNonRoot {
		t.Error("container SecurityContext.RunAsNonRoot should be true")
	}
	if csc.Capabilities == nil {
		t.Fatal("container SecurityContext.Capabilities is nil")
	}
	found := false
	for _, cap := range csc.Capabilities.Drop {
		if cap == "ALL" {
			found = true
			break
		}
	}
	if !found {
		t.Error("container capabilities should drop ALL")
	}
}

func TestBuildStatefulSet_ConfigHashAnnotation(t *testing.T) {
	inst := newStsInstance("bot", "default")
	hash := "abc123def456"
	sts := BuildStatefulSet(inst, hash)

	got := sts.Spec.Template.Annotations[ConfigHashAnnotation]
	if got != hash {
		t.Errorf("config hash annotation = %q, want %q", got, hash)
	}
}

func TestBuildStatefulSet_ConfigHashAnnotationEmpty(t *testing.T) {
	inst := newStsInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	got := sts.Spec.Template.Annotations[ConfigHashAnnotation]
	if got != "" {
		t.Errorf("config hash annotation = %q, want empty string", got)
	}
}

func TestBuildStatefulSet_AdditionalVolumesAndMounts(t *testing.T) {
	inst := newStsInstance("bot", "default")
	inst.Spec.AdditionalVolumes = []corev1.Volume{
		{
			Name: "extra-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"},
			},
		},
	}
	inst.Spec.AdditionalVolumeMounts = []corev1.VolumeMount{
		{
			Name:      "extra-secret",
			MountPath: "/etc/extra-secret",
			ReadOnly:  true,
		},
	}
	sts := BuildStatefulSet(inst, "")

	// Check volume is present.
	foundVol := false
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "extra-secret" && v.Secret != nil && v.Secret.SecretName == "my-secret" {
			foundVol = true
			break
		}
	}
	if !foundVol {
		t.Error("expected additional volume 'extra-secret' in pod volumes")
	}

	// Check volume mount is present on the main container.
	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("no containers in pod template")
	}
	foundMount := false
	for _, vm := range sts.Spec.Template.Spec.Containers[0].VolumeMounts {
		if vm.Name == "extra-secret" && vm.MountPath == "/etc/extra-secret" {
			foundMount = true
			break
		}
	}
	if !foundMount {
		t.Error("expected additional volume mount 'extra-secret' on main container")
	}
}

func TestBuildStatefulSet_PVCDisabled_EmptyDir(t *testing.T) {
	inst := newStsInstance("bot", "default")
	// Persistence disabled (default zero value = false).
	inst.Spec.Storage.Persistence.Enabled = false

	sts := BuildStatefulSet(inst, "")

	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Errorf("expected no VolumeClaimTemplates when persistence disabled, got %d", len(sts.Spec.VolumeClaimTemplates))
	}

	found := false
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == DataVolumeName && v.EmptyDir != nil {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected emptyDir volume for data when persistence disabled")
	}
}

func TestBuildStatefulSet_PVCEnabled_VolumeClaimTemplate(t *testing.T) {
	inst := newStsInstance("bot", "default")
	inst.Spec.Storage.Persistence.Enabled = true
	inst.Spec.Storage.Persistence.Size = resource.MustParse("5Gi")

	sts := BuildStatefulSet(inst, "")

	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("expected 1 VolumeClaimTemplate, got %d", len(sts.Spec.VolumeClaimTemplates))
	}
	pvc := sts.Spec.VolumeClaimTemplates[0]
	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.Cmp(resource.MustParse("5Gi")) != 0 {
		t.Errorf("PVC storage request = %s, want 5Gi", storageReq.String())
	}
}

func TestBuildStatefulSet_GRPCHealthProbes(t *testing.T) {
	inst := newStsInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("no containers in pod template")
	}
	c := sts.Spec.Template.Spec.Containers[0]

	// Liveness probe: gRPC on default port 8086, no service name.
	if c.LivenessProbe == nil || c.LivenessProbe.GRPC == nil {
		t.Fatal("expected gRPC liveness probe")
	}
	if c.LivenessProbe.GRPC.Port != 8086 {
		t.Errorf("liveness grpc port = %d, want 8086", c.LivenessProbe.GRPC.Port)
	}

	// Readiness probe: gRPC on default port 8086, service "opentalon".
	if c.ReadinessProbe == nil || c.ReadinessProbe.GRPC == nil {
		t.Fatal("expected gRPC readiness probe")
	}
	if c.ReadinessProbe.GRPC.Port != 8086 {
		t.Errorf("readiness grpc port = %d, want 8086", c.ReadinessProbe.GRPC.Port)
	}
	if c.ReadinessProbe.GRPC.Service == nil || *c.ReadinessProbe.GRPC.Service != "opentalon" {
		t.Errorf("readiness grpc service = %v, want \"opentalon\"", c.ReadinessProbe.GRPC.Service)
	}
	if c.ReadinessProbe.InitialDelaySeconds != 10 {
		t.Errorf("readiness initialDelaySeconds = %d, want 10", c.ReadinessProbe.InitialDelaySeconds)
	}

	// Startup probe: gRPC, default 600s timeout → failureThreshold=120.
	if c.StartupProbe == nil || c.StartupProbe.GRPC == nil {
		t.Fatal("expected gRPC startup probe")
	}
	if c.StartupProbe.GRPC.Port != 8086 {
		t.Errorf("startup grpc port = %d, want 8086", c.StartupProbe.GRPC.Port)
	}
	if c.StartupProbe.FailureThreshold != 120 {
		t.Errorf("startup failureThreshold = %d, want 120", c.StartupProbe.FailureThreshold)
	}
}

func TestBuildStatefulSet_CustomHealthPort(t *testing.T) {
	inst := newStsInstance("bot", "default")
	inst.Spec.Observability.Health.Port = 9999
	inst.Spec.Observability.Health.StartupTimeoutSeconds = 300
	inst.Spec.Observability.Health.ReadinessInitialDelaySeconds = 30
	sts := BuildStatefulSet(inst, "")

	c := sts.Spec.Template.Spec.Containers[0]
	if c.LivenessProbe.GRPC.Port != 9999 {
		t.Errorf("liveness grpc port = %d, want 9999", c.LivenessProbe.GRPC.Port)
	}
	if c.StartupProbe.FailureThreshold != 60 {
		t.Errorf("startup failureThreshold = %d, want 60 (300/5)", c.StartupProbe.FailureThreshold)
	}
	if c.ReadinessProbe.InitialDelaySeconds != 30 {
		t.Errorf("readiness initialDelaySeconds = %d, want 30", c.ReadinessProbe.InitialDelaySeconds)
	}

	// Check health port in container ports.
	found := false
	for _, p := range c.Ports {
		if p.Name == "health" && p.ContainerPort == 9999 {
			found = true
		}
	}
	if !found {
		t.Error("expected health container port 9999")
	}
}

func TestBuildStatefulSet_ConfigArg(t *testing.T) {
	inst := newStsInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("no containers in pod template")
	}
	args := sts.Spec.Template.Spec.Containers[0].Args
	found := false
	for i, a := range args {
		if a == "-config" && i+1 < len(args) && strings.HasSuffix(args[i+1], "config.yaml") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("container args do not include -config <path>/config.yaml; got %v", args)
	}
}
