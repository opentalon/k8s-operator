package resources_test

import (
	"testing"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
	"github.com/opentalon/k8s-operator/internal/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPluginIngress(t *testing.T) {
	instance := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opentalon",
			Namespace: "opentalon",
		},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			Config: v1alpha1.ConfigSpec{
				Plugins: map[string]v1alpha1.PluginConfig{
					"weaviate": {
						Ingress: &v1alpha1.PluginIngressSpec{
							Enabled:       true,
							Host:          "opentalon.timly.com",
							Path:          "/weaviate",
							Port:          8082,
							TLSSecretName: "opentalon-ws-tls",
							Annotations: map[string]string{
								"cert-manager.io/cluster-issuer": "letsencrypt",
							},
						},
					},
				},
			},
		},
	}

	// Test Ingress name
	name := resources.PluginIngressName(instance, "weaviate")
	if name != "opentalon-plugin-weaviate" {
		t.Errorf("expected opentalon-plugin-weaviate, got %s", name)
	}

	// Test BuildPluginIngress
	ingress := resources.BuildPluginIngress(instance, "weaviate", instance.Spec.Config.Plugins["weaviate"])
	if ingress.Name != "opentalon-plugin-weaviate" {
		t.Errorf("ingress name: expected opentalon-plugin-weaviate, got %s", ingress.Name)
	}
	if ingress.Namespace != "opentalon" {
		t.Errorf("ingress namespace: expected opentalon, got %s", ingress.Namespace)
	}
	if len(ingress.Spec.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(ingress.Spec.Rules))
	}
	rule := ingress.Spec.Rules[0]
	if rule.Host != "opentalon.timly.com" {
		t.Errorf("host: expected opentalon.timly.com, got %s", rule.Host)
	}
	if len(rule.HTTP.Paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(rule.HTTP.Paths))
	}
	path := rule.HTTP.Paths[0]
	if path.Path != "/weaviate(/|$)(.*)" {
		t.Errorf("path: expected /weaviate(/|$)(.*), got %s", path.Path)
	}
	if path.Backend.Service.Port.Number != 8082 {
		t.Errorf("port: expected 8082, got %d", path.Backend.Service.Port.Number)
	}
	if path.Backend.Service.Name != "opentalon" {
		t.Errorf("service name: expected opentalon, got %s", path.Backend.Service.Name)
	}

	// Check rewrite-target annotation
	if ingress.Annotations["nginx.ingress.kubernetes.io/rewrite-target"] != "/$2" {
		t.Error("missing rewrite-target annotation")
	}
	// Check user annotation merged
	if ingress.Annotations["cert-manager.io/cluster-issuer"] != "letsencrypt" {
		t.Error("missing cert-manager annotation")
	}

	// Check TLS
	if len(ingress.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS entry, got %d", len(ingress.Spec.TLS))
	}
	if ingress.Spec.TLS[0].SecretName != "opentalon-ws-tls" {
		t.Errorf("TLS secret: expected opentalon-ws-tls, got %s", ingress.Spec.TLS[0].SecretName)
	}

	t.Log("Plugin ingress builds correctly")

	// Test Service has plugin port
	svc := resources.BuildService(instance)
	foundPluginPort := false
	for _, p := range svc.Spec.Ports {
		if p.Name == "plugin-weaviate" && p.Port == 8082 {
			foundPluginPort = true
		}
	}
	if !foundPluginPort {
		t.Error("service missing plugin-weaviate port 8082")
	}

	// Test StatefulSet has plugin container port
	sts := resources.BuildStatefulSet(instance, "hash")
	foundContainerPort := false
	for _, p := range sts.Spec.Template.Spec.Containers[0].Ports {
		if p.Name == "plugin-weaviat" || p.Name == "plugin-weaviate" {
			if p.ContainerPort == 8082 {
				foundContainerPort = true
			}
		}
	}
	if !foundContainerPort {
		t.Error("statefulset missing plugin container port 8082")
	}
}

func TestPluginIngress_GRPC(t *testing.T) {
	instance := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opentalon",
			Namespace: "opentalon",
		},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			Config: v1alpha1.ConfigSpec{
				Plugins: map[string]v1alpha1.PluginConfig{
					"talooner": {
						Ingress: &v1alpha1.PluginIngressSpec{
							Enabled:       true,
							Host:          "talooner.zhisme.com",
							Path:          "/",
							Port:          50100,
							Protocol:      "GRPC",
							TLSSecretName: "talooner-tls",
						},
					},
				},
			},
		},
	}

	ingress := resources.BuildPluginIngress(instance, "talooner", instance.Spec.Config.Plugins["talooner"])

	// gRPC method paths must reach the plugin unrewritten — no
	// rewrite-target, no regex capture groups mangling the path.
	if _, ok := ingress.Annotations["nginx.ingress.kubernetes.io/rewrite-target"]; ok {
		t.Error("GRPC ingress must not set rewrite-target")
	}
	if got := ingress.Annotations["nginx.ingress.kubernetes.io/backend-protocol"]; got != "GRPC" {
		t.Errorf("backend-protocol: expected GRPC, got %q", got)
	}

	if len(ingress.Spec.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(ingress.Spec.Rules))
	}
	path := ingress.Spec.Rules[0].HTTP.Paths[0]
	if path.Path != "/" {
		t.Errorf("path: expected /, got %s", path.Path)
	}
	if path.Backend.Service.Port.Number != 50100 {
		t.Errorf("port: expected 50100, got %d", path.Backend.Service.Port.Number)
	}

	if len(ingress.Spec.TLS) != 1 || ingress.Spec.TLS[0].SecretName != "talooner-tls" {
		t.Error("expected TLS secret talooner-tls to be set")
	}
}

// A user annotation for backend-protocol (or anything else) must win over
// the GRPC default — same override behavior the HTTP path already has for
// rewrite-target.
func TestPluginIngress_GRPC_AnnotationOverride(t *testing.T) {
	instance := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "opentalon", Namespace: "opentalon"},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			Config: v1alpha1.ConfigSpec{
				Plugins: map[string]v1alpha1.PluginConfig{
					"talooner": {
						Ingress: &v1alpha1.PluginIngressSpec{
							Enabled:  true,
							Host:     "talooner.zhisme.com",
							Path:     "/",
							Port:     50100,
							Protocol: "GRPC",
							Annotations: map[string]string{
								"nginx.ingress.kubernetes.io/backend-protocol": "GRPCS",
							},
						},
					},
				},
			},
		},
	}

	ingress := resources.BuildPluginIngress(instance, "talooner", instance.Spec.Config.Plugins["talooner"])
	if got := ingress.Annotations["nginx.ingress.kubernetes.io/backend-protocol"]; got != "GRPCS" {
		t.Errorf("backend-protocol: expected user override GRPCS, got %q", got)
	}
}

// A user who copies an HTTP example's Path (Path is required by the CRD, so
// there's always a value) must not get it used as a literal Prefix match —
// GRPC routing is host-based and documented as ignoring Path entirely.
func TestPluginIngress_GRPC_IgnoresNonRootPath(t *testing.T) {
	instance := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "opentalon", Namespace: "opentalon"},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			Config: v1alpha1.ConfigSpec{
				Plugins: map[string]v1alpha1.PluginConfig{
					"talooner": {
						Ingress: &v1alpha1.PluginIngressSpec{
							Enabled:  true,
							Host:     "talooner.zhisme.com",
							Path:     "/weaviate", // copied from an HTTP example, must be ignored
							Port:     50100,
							Protocol: "GRPC",
						},
					},
				},
			},
		},
	}

	ingress := resources.BuildPluginIngress(instance, "talooner", instance.Spec.Config.Plugins["talooner"])
	path := ingress.Spec.Rules[0].HTTP.Paths[0]
	if path.Path != "/" {
		t.Errorf("path: expected / regardless of Path field, got %s", path.Path)
	}
}

func TestPluginIngress_NoIngress(t *testing.T) {
	instance := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			Config: v1alpha1.ConfigSpec{
				Plugins: map[string]v1alpha1.PluginConfig{
					"mcp": {},
				},
			},
		},
	}

	// Service should not have plugin port
	svc := resources.BuildService(instance)
	for _, p := range svc.Spec.Ports {
		if p.Name == "plugin-mcp" {
			t.Error("service should not have plugin-mcp port when no ingress configured")
		}
	}
}

func TestPluginPort_WithoutIngressEnabled(t *testing.T) {
	instance := &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "opentalon", Namespace: "opentalon"},
		Spec: v1alpha1.OpenTalonInstanceSpec{
			Config: v1alpha1.ConfigSpec{
				Plugins: map[string]v1alpha1.PluginConfig{
					"api": {
						Ingress: &v1alpha1.PluginIngressSpec{
							Enabled: false,
							Port:    8080,
							Path:    "/api",
						},
					},
				},
			},
		},
	}

	// Service should have plugin port even when ingress is not enabled.
	svc := resources.BuildService(instance)
	foundPort := false
	for _, p := range svc.Spec.Ports {
		if p.Name == "plugin-api" && p.Port == 8080 {
			foundPort = true
		}
	}
	if !foundPort {
		t.Error("service should have plugin-api port 8080 even when ingress.enabled is false")
	}

	// StatefulSet should have container port too.
	sts := resources.BuildStatefulSet(instance, "hash")
	foundContainerPort := false
	for _, p := range sts.Spec.Template.Spec.Containers[0].Ports {
		if p.Name == "plugin-api" && p.ContainerPort == 8080 {
			foundContainerPort = true
		}
	}
	if !foundContainerPort {
		t.Error("statefulset should have plugin-api container port 8080 even when ingress.enabled is false")
	}
}
