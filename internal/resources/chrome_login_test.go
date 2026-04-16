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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

func newChromeInstance(name, namespace string) *v1alpha1.OpenTalonInstance {
	return &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			ChromeLogin: &v1alpha1.ChromeLoginSpec{},
		},
	}
}

// ── Name helpers ──────────────────────────────────────────────────────────────

func TestChromeLoginSecretName(t *testing.T) {
	inst := newChromeInstance("my-bot", "default")
	got := ChromeLoginSecretName(inst)
	want := "my-bot" + ChromeLoginSecretSuffix
	if got != want {
		t.Errorf("ChromeLoginSecretName() = %q, want %q", got, want)
	}
}

func TestChromeLoginServiceName(t *testing.T) {
	inst := newChromeInstance("my-bot", "default")
	got := ChromeLoginServiceName(inst)
	want := "my-bot" + ChromeLoginServiceSuffix
	if got != want {
		t.Errorf("ChromeLoginServiceName() = %q, want %q", got, want)
	}
}

func TestChromeLoginIngressName(t *testing.T) {
	inst := newChromeInstance("my-bot", "default")
	got := ChromeLoginIngressName(inst)
	want := "my-bot" + ChromeLoginIngressSuffix
	if got != want {
		t.Errorf("ChromeLoginIngressName() = %q, want %q", got, want)
	}
}

func TestChromeLoginLabels_ComponentLabel(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	labels := ChromeLoginLabels(inst)
	if labels["app.kubernetes.io/component"] != "chrome-login" {
		t.Errorf("ChromeLoginLabels() component = %q, want %q",
			labels["app.kubernetes.io/component"], "chrome-login")
	}
}

func TestChromeLoginLabels_InheritsBaseLabels(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	base := Labels(inst)
	labels := ChromeLoginLabels(inst)
	for k, v := range base {
		if k == "app.kubernetes.io/component" {
			continue // overridden by chrome-login labels
		}
		if labels[k] != v {
			t.Errorf("ChromeLoginLabels() missing base label[%q]: got %q, want %q", k, labels[k], v)
		}
	}
}

// ── BuildChromeLoginSecret ────────────────────────────────────────────────────

func TestBuildChromeLoginSecret(t *testing.T) {
	inst := newChromeInstance("bot", "staging")
	secret := BuildChromeLoginSecret(inst, "supersecret")

	if secret == nil {
		t.Fatal("BuildChromeLoginSecret() returned nil")
	}
	if secret.Name != ChromeLoginSecretName(inst) {
		t.Errorf("Secret.Name = %q, want %q", secret.Name, ChromeLoginSecretName(inst))
	}
	if secret.Namespace != "staging" {
		t.Errorf("Secret.Namespace = %q, want staging", secret.Namespace)
	}
	if secret.Type != corev1.SecretTypeOpaque {
		t.Errorf("Secret.Type = %q, want Opaque", secret.Type)
	}
	if secret.StringData[ChromeLoginPasswordKey] != "supersecret" {
		t.Errorf("Secret password = %q, want supersecret", secret.StringData[ChromeLoginPasswordKey])
	}
}

// ── BuildChromeLoginService ───────────────────────────────────────────────────

func TestBuildChromeLoginService_DefaultPorts(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	svc := BuildChromeLoginService(inst)

	if svc == nil {
		t.Fatal("BuildChromeLoginService() returned nil")
	}
	if svc.Name != ChromeLoginServiceName(inst) {
		t.Errorf("Service.Name = %q, want %q", svc.Name, ChromeLoginServiceName(inst))
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("Service.Type = %q, want ClusterIP", svc.Spec.Type)
	}

	// Expect 2 ports: vnc (3000) and cdp (9223 externally)
	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("Service.Ports count = %d, want 2", len(svc.Spec.Ports))
	}

	ports := map[string]int32{}
	for _, p := range svc.Spec.Ports {
		ports[p.Name] = p.Port
	}
	if ports["vnc"] != 3000 {
		t.Errorf("vnc port = %d, want 3000", ports["vnc"])
	}
	if ports["cdp"] != chromeLoginExternalCDPPort {
		t.Errorf("cdp port = %d, want %d", ports["cdp"], chromeLoginExternalCDPPort)
	}
}

func TestBuildChromeLoginService_CustomPorts(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.VNCPort = 4000
	inst.Spec.ChromeLogin.CDPPort = 9333

	svc := BuildChromeLoginService(inst)

	ports := map[string]int32{}
	for _, p := range svc.Spec.Ports {
		ports[p.Name] = p.Port
	}
	if ports["vnc"] != 4000 {
		t.Errorf("vnc port = %d, want 4000", ports["vnc"])
	}
	// CDP external port stays fixed at chromeLoginExternalCDPPort regardless of CDPPort.
	if ports["cdp"] != chromeLoginExternalCDPPort {
		t.Errorf("cdp external port = %d, want %d", ports["cdp"], chromeLoginExternalCDPPort)
	}
	// The target port for cdp should map to the custom CDP port.
	for _, p := range svc.Spec.Ports {
		if p.Name == "cdp" && p.TargetPort.IntVal != 9333 {
			t.Errorf("cdp target port = %d, want 9333", p.TargetPort.IntVal)
		}
	}
}

func TestBuildChromeLoginService_SelectorMatchesInstance(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	svc := BuildChromeLoginService(inst)
	sel := SelectorLabels(inst)
	for k, v := range sel {
		if svc.Spec.Selector[k] != v {
			t.Errorf("Service selector[%q] = %q, want %q", k, svc.Spec.Selector[k], v)
		}
	}
}

// ── BuildChromeLoginIngress ───────────────────────────────────────────────────

func TestBuildChromeLoginIngress_NilWhenNoChromeLogin(t *testing.T) {
	inst := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "bot", Namespace: "default"},
	}
	if BuildChromeLoginIngress(inst) != nil {
		t.Error("BuildChromeLoginIngress() should return nil when ChromeLogin is nil")
	}
}

func TestBuildChromeLoginIngress_NilWhenNoIngressSpec(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	// ChromeLogin set but no Ingress sub-spec.
	if BuildChromeLoginIngress(inst) != nil {
		t.Error("BuildChromeLoginIngress() should return nil when Ingress spec is nil")
	}
}

func TestBuildChromeLoginIngress_NilWhenDisabled(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Ingress = &v1alpha1.IngressSpec{Enabled: false, Host: "vnc.example.com"}
	if BuildChromeLoginIngress(inst) != nil {
		t.Error("BuildChromeLoginIngress() should return nil when Ingress.Enabled is false")
	}
}

func TestBuildChromeLoginIngress_Basic(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Ingress = &v1alpha1.IngressSpec{
		Enabled: true,
		Host:    "vnc.example.com",
	}
	ingress := BuildChromeLoginIngress(inst)
	if ingress == nil {
		t.Fatal("BuildChromeLoginIngress() returned nil")
	}
	if ingress.Name != ChromeLoginIngressName(inst) {
		t.Errorf("Ingress.Name = %q, want %q", ingress.Name, ChromeLoginIngressName(inst))
	}
	if ingress.Namespace != "default" {
		t.Errorf("Ingress.Namespace = %q, want default", ingress.Namespace)
	}
	if len(ingress.Spec.Rules) != 1 {
		t.Fatalf("Ingress rules count = %d, want 1", len(ingress.Spec.Rules))
	}
	if ingress.Spec.Rules[0].Host != "vnc.example.com" {
		t.Errorf("Ingress rule host = %q, want vnc.example.com", ingress.Spec.Rules[0].Host)
	}
	if len(ingress.Spec.TLS) != 0 {
		t.Errorf("expected no TLS without TLSSecretName, got %d TLS entries", len(ingress.Spec.TLS))
	}
}

func TestBuildChromeLoginIngress_WithTLS(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Ingress = &v1alpha1.IngressSpec{
		Enabled:       true,
		Host:          "vnc.example.com",
		TLSSecretName: "vnc-tls",
	}
	ingress := BuildChromeLoginIngress(inst)
	if ingress == nil {
		t.Fatal("BuildChromeLoginIngress() returned nil")
	}
	if len(ingress.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS entry, got %d", len(ingress.Spec.TLS))
	}
	if ingress.Spec.TLS[0].SecretName != "vnc-tls" {
		t.Errorf("TLS secret name = %q, want vnc-tls", ingress.Spec.TLS[0].SecretName)
	}
	if len(ingress.Spec.TLS[0].Hosts) != 1 || ingress.Spec.TLS[0].Hosts[0] != "vnc.example.com" {
		t.Errorf("TLS hosts = %v, want [vnc.example.com]", ingress.Spec.TLS[0].Hosts)
	}
}

func TestBuildChromeLoginIngress_AnnotationsForwarded(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Ingress = &v1alpha1.IngressSpec{
		Enabled:     true,
		Host:        "vnc.example.com",
		Annotations: map[string]string{"cert-manager.io/cluster-issuer": "letsencrypt"},
	}
	ingress := BuildChromeLoginIngress(inst)
	if ingress.Annotations["cert-manager.io/cluster-issuer"] != "letsencrypt" {
		t.Errorf("annotation not forwarded to Ingress")
	}
}

// ── ChromeLoginSidecarContainer ───────────────────────────────────────────────

func TestChromeLoginSidecarContainer_Name(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	c := ChromeLoginSidecarContainer(inst)
	if c.Name != ChromeLoginVNCContainerName {
		t.Errorf("container Name = %q, want %q", c.Name, ChromeLoginVNCContainerName)
	}
}

func TestChromeLoginSidecarContainer_DefaultImage(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	c := ChromeLoginSidecarContainer(inst)
	if !strings.Contains(c.Image, "linuxserver/chromium") {
		t.Errorf("container Image = %q, want to contain linuxserver/chromium", c.Image)
	}
}

func TestChromeLoginSidecarContainer_CustomImage(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Image = "custom/chromium:v99"
	c := ChromeLoginSidecarContainer(inst)
	if c.Image != "custom/chromium:v99" {
		t.Errorf("container Image = %q, want custom/chromium:v99", c.Image)
	}
}

func TestChromeLoginSidecarContainer_DefaultPorts(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	c := ChromeLoginSidecarContainer(inst)

	ports := map[string]int32{}
	for _, p := range c.Ports {
		ports[p.Name] = p.ContainerPort
	}
	if ports["vnc"] != 3000 {
		t.Errorf("vnc port = %d, want 3000", ports["vnc"])
	}
	if ports["cdp"] != 9222 {
		t.Errorf("cdp port = %d, want 9222", ports["cdp"])
	}
}

func TestChromeLoginSidecarContainer_PasswordFromSecret(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	c := ChromeLoginSidecarContainer(inst)

	var pwdEnv *corev1.EnvVar
	for i := range c.Env {
		if c.Env[i].Name == "PASSWORD" {
			pwdEnv = &c.Env[i]
			break
		}
	}
	if pwdEnv == nil {
		t.Fatal("PASSWORD env var not found in chrome-login sidecar")
	}
	if pwdEnv.ValueFrom == nil || pwdEnv.ValueFrom.SecretKeyRef == nil {
		t.Fatal("PASSWORD env should come from a SecretKeyRef")
	}
	if pwdEnv.ValueFrom.SecretKeyRef.Name != ChromeLoginSecretName(inst) {
		t.Errorf("SECRET ref name = %q, want %q",
			pwdEnv.ValueFrom.SecretKeyRef.Name, ChromeLoginSecretName(inst))
	}
}

func TestChromeLoginSidecarContainer_ChromeCLIEnv(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	c := ChromeLoginSidecarContainer(inst)

	for _, e := range c.Env {
		if e.Name == "CHROME_CLI" {
			if !strings.Contains(e.Value, "--remote-debugging-port=9222") {
				t.Errorf("CHROME_CLI missing remote debugging flag: %q", e.Value)
			}
			return
		}
	}
	t.Error("CHROME_CLI env var not found in chrome-login sidecar")
}

// ── ChromeLoginEnvVars ────────────────────────────────────────────────────────

func TestChromeLoginEnvVars_CDP_URL(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_LOGIN_CDP_URL" {
			if !strings.Contains(e.Value, "localhost:9222") {
				t.Errorf("CHROME_LOGIN_CDP_URL = %q, expected localhost:9222", e.Value)
			}
			return
		}
	}
	t.Error("CHROME_LOGIN_CDP_URL not found in ChromeLoginEnvVars()")
}

func TestChromeLoginEnvVars_CustomCDPPort(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.CDPPort = 9333
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_LOGIN_CDP_URL" {
			if !strings.Contains(e.Value, "localhost:9333") {
				t.Errorf("CHROME_LOGIN_CDP_URL = %q, expected localhost:9333", e.Value)
			}
			return
		}
	}
	t.Error("CHROME_LOGIN_CDP_URL not found")
}

func TestChromeLoginEnvVars_DataDir(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_DATA_DIR" {
			if !strings.HasPrefix(e.Value, DataMountPath) {
				t.Errorf("CHROME_DATA_DIR = %q, expected prefix %s", e.Value, DataMountPath)
			}
			return
		}
	}
	t.Error("CHROME_DATA_DIR not found in ChromeLoginEnvVars()")
}

func TestChromeLoginEnvVars_PasswordRef(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_LOGIN_PASSWORD" {
			if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
				t.Fatal("CHROME_LOGIN_PASSWORD should come from a SecretKeyRef")
			}
			if e.ValueFrom.SecretKeyRef.Name != ChromeLoginSecretName(inst) {
				t.Errorf("secret ref name = %q, want %q",
					e.ValueFrom.SecretKeyRef.Name, ChromeLoginSecretName(inst))
			}
			return
		}
	}
	t.Error("CHROME_LOGIN_PASSWORD not found in ChromeLoginEnvVars()")
}

func TestChromeLoginEnvVars_NoLoginURL_WhenNoIngress(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_LOGIN_URL" {
			t.Errorf("CHROME_LOGIN_URL should not be set when no ingress configured; got %q", e.Value)
		}
	}
}

func TestChromeLoginEnvVars_LoginURL_HTTP(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Ingress = &v1alpha1.IngressSpec{
		Enabled: true,
		Host:    "vnc.example.com",
	}
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_LOGIN_URL" {
			if e.Value != "http://vnc.example.com" {
				t.Errorf("CHROME_LOGIN_URL = %q, want http://vnc.example.com", e.Value)
			}
			return
		}
	}
	t.Error("CHROME_LOGIN_URL not set when ingress configured")
}

func TestChromeLoginEnvVars_LoginURL_HTTPS_WhenTLS(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	inst.Spec.ChromeLogin.Ingress = &v1alpha1.IngressSpec{
		Enabled:       true,
		Host:          "vnc.example.com",
		TLSSecretName: "tls-secret",
	}
	vars := ChromeLoginEnvVars(inst)

	for _, e := range vars {
		if e.Name == "CHROME_LOGIN_URL" {
			if e.Value != "https://vnc.example.com" {
				t.Errorf("CHROME_LOGIN_URL = %q, want https://vnc.example.com", e.Value)
			}
			return
		}
	}
	t.Error("CHROME_LOGIN_URL not set when TLS ingress configured")
}

// ── int32ToString ─────────────────────────────────────────────────────────────

func TestInt32ToString(t *testing.T) {
	tests := []struct {
		n    int32
		want string
	}{
		{0, "0"},
		{1, "1"},
		{9222, "9222"},
		{-1, "-1"},
		{-9222, "-9222"},
	}
	for _, tc := range tests {
		got := int32ToString(tc.n)
		if got != tc.want {
			t.Errorf("int32ToString(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// ── StatefulSet chrome-login integration ─────────────────────────────────────

func TestBuildStatefulSet_ChromeLoginSidecar_Present(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	found := false
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == ChromeLoginVNCContainerName {
			found = true
			break
		}
	}
	if !found {
		t.Error("chrome-login sidecar container not found in StatefulSet when ChromeLogin is set")
	}
}

func TestBuildStatefulSet_NoChromeLoginSidecar_WhenNil(t *testing.T) {
	inst := newInstance("bot", "default")
	// ChromeLogin is nil (default)
	sts := BuildStatefulSet(inst, "")

	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == ChromeLoginVNCContainerName {
			t.Error("chrome-login sidecar should not be added when ChromeLogin spec is nil")
		}
	}
}

func TestBuildStatefulSet_ChromeLogin_EnvVarsInjected(t *testing.T) {
	inst := newChromeInstance("bot", "default")
	sts := BuildStatefulSet(inst, "")

	if len(sts.Spec.Template.Spec.Containers) == 0 {
		t.Fatal("no containers in pod template")
	}
	mainContainer := sts.Spec.Template.Spec.Containers[0]

	found := false
	for _, e := range mainContainer.Env {
		if e.Name == "CHROME_LOGIN_CDP_URL" {
			found = true
			break
		}
	}
	if !found {
		t.Error("CHROME_LOGIN_CDP_URL env var not injected into main container when ChromeLogin is set")
	}
}
