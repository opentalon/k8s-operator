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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

func newConfigMapInstance(name, namespace string) *v1alpha1.OpenTalonInstance {
	return &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestBuildConfigMap_BasicCase(t *testing.T) {
	inst := newConfigMapInstance("my-bot", "default")
	cm := BuildConfigMap(inst)

	if cm == nil {
		t.Fatal("BuildConfigMap() returned nil")
	}

	_, ok := cm.Data["config.yaml"]
	if !ok {
		t.Error("BuildConfigMap() missing config.yaml key in Data")
	}
}

func TestBuildConfigMap_NameAndNamespace(t *testing.T) {
	inst := newConfigMapInstance("talon-1", "prod")
	cm := BuildConfigMap(inst)

	wantName := ConfigMapName(inst)
	if cm.Name != wantName {
		t.Errorf("ConfigMap.Name = %q, want %q", cm.Name, wantName)
	}
	if cm.Namespace != "prod" {
		t.Errorf("ConfigMap.Namespace = %q, want %q", cm.Namespace, "prod")
	}
}

func TestBuildConfigMap_Labels(t *testing.T) {
	inst := newConfigMapInstance("talon-2", "default")
	cm := BuildConfigMap(inst)

	want := Labels(inst)
	for k, v := range want {
		if cm.Labels[k] != v {
			t.Errorf("ConfigMap.Labels[%q] = %q, want %q", k, cm.Labels[k], v)
		}
	}
}

func TestBuildConfigMap_ModelsSection(t *testing.T) {
	inst := newConfigMapInstance("talon-3", "default")
	inst.Spec.Config.Models = []v1alpha1.ModelConfig{
		{
			Name:     "claude-sonnet",
			Provider: "anthropic",
			BaseURL:  "https://api.anthropic.com",
		},
	}

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	for _, want := range []string{"models:", "claude-sonnet", "anthropic", "api.anthropic.com"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config.yaml missing %q; got:\n%s", want, yaml)
		}
	}
}

func TestBuildConfigMap_RoutingSection(t *testing.T) {
	inst := newConfigMapInstance("talon-4", "default")
	inst.Spec.Config.Routing = &v1alpha1.RoutingConfig{
		Primary:   "claude-sonnet",
		Fallbacks: []string{"gpt4", "deepseek-chat"},
	}

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	for _, want := range []string{"routing:", "primary:", "claude-sonnet", "fallbacks:", "gpt4", "deepseek-chat"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config.yaml missing %q; got:\n%s", want, yaml)
		}
	}
}

func TestBuildConfigMap_ConsoleChannel(t *testing.T) {
	inst := newConfigMapInstance("talon-5", "default")
	inst.Spec.Config.Channels = &v1alpha1.ChannelsConfig{
		Console: &v1alpha1.ConsoleChannelConfig{Enabled: true},
	}

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	for _, want := range []string{"channels:", "console:", "enabled: true"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config.yaml missing %q; got:\n%s", want, yaml)
		}
	}
}

func TestBuildConfigMap_WebhookChannel(t *testing.T) {
	inst := newConfigMapInstance("talon-6", "default")
	inst.Spec.Config.Channels = &v1alpha1.ChannelsConfig{
		Webhook: &v1alpha1.WebhookChannelConfig{
			Enabled: true,
			Port:    9000,
			Path:    "/events",
		},
	}

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	for _, want := range []string{"webhook:", "9000", "/events"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config.yaml missing %q; got:\n%s", want, yaml)
		}
	}
}

func TestBuildConfigMap_WebhookDefaultPortAndPath(t *testing.T) {
	inst := newConfigMapInstance("talon-7", "default")
	inst.Spec.Config.Channels = &v1alpha1.ChannelsConfig{
		Webhook: &v1alpha1.WebhookChannelConfig{Enabled: true},
	}

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	if !strings.Contains(yaml, "8080") {
		t.Errorf("config.yaml missing default port 8080; got:\n%s", yaml)
	}
	if !strings.Contains(yaml, "/webhook") {
		t.Errorf("config.yaml missing default path /webhook; got:\n%s", yaml)
	}
}

func TestBuildConfigMap_ExtraConfig(t *testing.T) {
	inst := newConfigMapInstance("talon-8", "default")
	inst.Spec.Config.ExtraConfig = "custom_key: custom_value\n"

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	if !strings.Contains(yaml, "custom_key: custom_value") {
		t.Errorf("config.yaml missing extra config; got:\n%s", yaml)
	}
}

func TestBuildConfigMap_ExtraConfigWithoutNewline(t *testing.T) {
	inst := newConfigMapInstance("talon-9", "default")
	// No trailing newline — renderConfigYAML should add one.
	inst.Spec.Config.ExtraConfig = "no_newline: yes"

	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	if !strings.Contains(yaml, "no_newline: yes") {
		t.Errorf("config.yaml missing extra config content; got:\n%s", yaml)
	}
}

func TestBuildConfigMap_DefaultStateAndLogging(t *testing.T) {
	// With an empty spec the generated YAML should still include state and logging defaults.
	inst := newConfigMapInstance("talon-10", "default")
	cm := BuildConfigMap(inst)
	yaml := cm.Data["config.yaml"]

	for _, want := range []string{
		"state:",
		"/data/opentalon.db",
		"logging:",
		"level: info",
		"format: json",
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("config.yaml missing default section %q; got:\n%s", want, yaml)
		}
	}
}
