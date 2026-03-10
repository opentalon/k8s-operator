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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

func newInstance(name, namespace string) *v1alpha1.OpenTalonInstance {
	return &v1alpha1.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestResourceName(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		want     string
	}{
		{"simple name", "my-instance", "my-instance"},
		{"with dashes", "foo-bar-baz", "foo-bar-baz"},
		{"single char", "x", "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := newInstance(tc.instance, "default")
			got := ResourceName(inst)
			if got != tc.want {
				t.Errorf("ResourceName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLabels(t *testing.T) {
	inst := newInstance("test-inst", "ns1")
	labels := Labels(inst)

	required := map[string]string{
		"app.kubernetes.io/name":       "opentalon",
		"app.kubernetes.io/instance":   "test-inst",
		"app.kubernetes.io/component":  "opentalon-instance",
		"app.kubernetes.io/part-of":    "opentalon",
		"app.kubernetes.io/managed-by": ManagedByLabel,
	}
	for k, want := range required {
		got, ok := labels[k]
		if !ok {
			t.Errorf("Labels() missing key %q", k)
			continue
		}
		if got != want {
			t.Errorf("Labels()[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestSelectorLabels(t *testing.T) {
	inst := newInstance("sel-inst", "ns1")
	full := Labels(inst)
	sel := SelectorLabels(inst)

	// All selector labels must exist in the full label set with identical values.
	for k, v := range sel {
		if full[k] != v {
			t.Errorf("SelectorLabels()[%q] = %q not present/matching in Labels()", k, v)
		}
	}

	// Selector must contain at least the two stable keys.
	for _, k := range []string{"app.kubernetes.io/name", "app.kubernetes.io/instance"} {
		if _, ok := sel[k]; !ok {
			t.Errorf("SelectorLabels() missing key %q", k)
		}
	}

	// Selector must be a strict subset (fewer keys than full labels).
	if len(sel) >= len(full) {
		t.Errorf("SelectorLabels() should have fewer keys than Labels(); got %d vs %d", len(sel), len(full))
	}
}

func TestImageRef(t *testing.T) {
	tests := []struct {
		name string
		spec v1alpha1.ImageSpec
		want string
	}{
		{
			name: "tag only",
			spec: v1alpha1.ImageSpec{Repository: "ghcr.io/opentalon/opentalon", Tag: "v1.2.3"},
			want: "ghcr.io/opentalon/opentalon:v1.2.3",
		},
		{
			name: "digest wins over tag",
			spec: v1alpha1.ImageSpec{
				Repository: "ghcr.io/opentalon/opentalon",
				Tag:        "v1.2.3",
				Digest:     "sha256:abc123",
			},
			want: "ghcr.io/opentalon/opentalon@sha256:abc123",
		},
		{
			name: "digest only (no tag)",
			spec: v1alpha1.ImageSpec{
				Repository: "ghcr.io/opentalon/opentalon",
				Digest:     "sha256:deadbeef",
			},
			want: "ghcr.io/opentalon/opentalon@sha256:deadbeef",
		},
		{
			name: "empty tag uses default",
			spec: v1alpha1.ImageSpec{Repository: "ghcr.io/opentalon/opentalon"},
			want: "ghcr.io/opentalon/opentalon:" + DefaultTag,
		},
		{
			name: "empty repo uses default",
			spec: v1alpha1.ImageSpec{Tag: "v2.0.0"},
			want: DefaultImage + ":v2.0.0",
		},
		{
			name: "both empty uses defaults",
			spec: v1alpha1.ImageSpec{},
			want: DefaultImage + ":" + DefaultTag,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ImageRef(tc.spec)
			if got != tc.want {
				t.Errorf("ImageRef() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHashSecretData(t *testing.T) {
	t.Run("same input produces same hash", func(t *testing.T) {
		data := map[string][]byte{
			"key1": []byte("value1"),
			"key2": []byte("value2"),
		}
		h1 := HashSecretData(data)
		h2 := HashSecretData(data)
		if h1 != h2 {
			t.Errorf("HashSecretData() not deterministic: %q vs %q", h1, h2)
		}
	})

	t.Run("different input produces different hash", func(t *testing.T) {
		d1 := map[string][]byte{"k": []byte("v1")}
		d2 := map[string][]byte{"k": []byte("v2")}
		if HashSecretData(d1) == HashSecretData(d2) {
			t.Error("HashSecretData() collision on different values")
		}
	})

	t.Run("key order does not affect hash", func(t *testing.T) {
		// Populate maps in different orders (map iteration order is random in Go).
		d1 := map[string][]byte{"a": []byte("1"), "b": []byte("2"), "c": []byte("3")}
		d2 := map[string][]byte{"c": []byte("3"), "a": []byte("1"), "b": []byte("2")}
		if HashSecretData(d1) != HashSecretData(d2) {
			t.Error("HashSecretData() should be order-independent")
		}
	})

	t.Run("empty map does not panic", func(t *testing.T) {
		h := HashSecretData(map[string][]byte{})
		if h == "" {
			t.Error("HashSecretData() returned empty string for empty map")
		}
	})

	t.Run("nil map does not panic", func(t *testing.T) {
		h := HashSecretData(nil)
		if h == "" {
			t.Error("HashSecretData() returned empty string for nil map")
		}
	})
}

func TestHashStringData(t *testing.T) {
	t.Run("same input produces same hash", func(t *testing.T) {
		data := map[string]string{"foo": "bar", "baz": "qux"}
		h1 := HashStringData(data)
		h2 := HashStringData(data)
		if h1 != h2 {
			t.Errorf("HashStringData() not deterministic: %q vs %q", h1, h2)
		}
	})

	t.Run("different input produces different hash", func(t *testing.T) {
		d1 := map[string]string{"k": "v1"}
		d2 := map[string]string{"k": "v2"}
		if HashStringData(d1) == HashStringData(d2) {
			t.Error("HashStringData() collision on different values")
		}
	})

	t.Run("consistent with HashSecretData", func(t *testing.T) {
		d := map[string]string{"hello": "world"}
		raw := map[string][]byte{"hello": []byte("world")}
		if HashStringData(d) != HashSecretData(raw) {
			t.Error("HashStringData() and HashSecretData() diverge for same content")
		}
	})

	t.Run("empty map does not panic", func(t *testing.T) {
		h := HashStringData(map[string]string{})
		if h == "" {
			t.Error("HashStringData() returned empty string for empty map")
		}
	})

	t.Run("nil map does not panic", func(t *testing.T) {
		h := HashStringData(nil)
		if h == "" {
			t.Error("HashStringData() returned empty string for nil map")
		}
	})
}
