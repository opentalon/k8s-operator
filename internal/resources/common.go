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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	v1alpha1 "github.com/opentalon/k8s-operator/api/v1alpha1"
)

const (
	// DefaultImage is the default OpenTalon container image repository.
	DefaultImage = "ghcr.io/opentalon/opentalon"

	// DefaultTag is the default OpenTalon image tag.
	DefaultTag = "latest"

	// ManagedByLabel is the standard label value indicating operator ownership.
	ManagedByLabel = "opentalon-operator"

	// FinalizerName is the finalizer added to OpenTalonInstance resources.
	FinalizerName = "opentalon.io/finalizer"
)

// ResourceName returns the canonical base name for all resources managed for the given instance.
// All child resources (StatefulSet, Service, ConfigMap, etc.) share this name.
func ResourceName(instance *v1alpha1.OpenTalonInstance) string {
	return instance.Name
}

// Labels returns the full set of recommended Kubernetes labels for resources owned by instance.
func Labels(instance *v1alpha1.OpenTalonInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "opentalon",
		"app.kubernetes.io/instance":   instance.Name,
		"app.kubernetes.io/component":  "opentalon-instance",
		"app.kubernetes.io/part-of":    "opentalon",
		"app.kubernetes.io/managed-by": ManagedByLabel,
	}
}

// SelectorLabels returns the minimal set of labels used as a pod selector for the StatefulSet.
// These must remain stable across upgrades and must not overlap with user-supplied labels.
func SelectorLabels(instance *v1alpha1.OpenTalonInstance) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "opentalon",
		"app.kubernetes.io/instance": instance.Name,
	}
}

// ImageRef builds the fully-qualified image reference for the given ImageSpec.
// If a digest is set it takes precedence over the tag, producing repo@digest form.
// If neither repository nor tag is set, the defaults are used.
func ImageRef(spec v1alpha1.ImageSpec) string {
	repo := spec.Repository
	if repo == "" {
		repo = DefaultImage
	}
	if spec.Digest != "" {
		return fmt.Sprintf("%s@%s", repo, spec.Digest)
	}
	tag := spec.Tag
	if tag == "" {
		tag = DefaultTag
	}
	return fmt.Sprintf("%s:%s", repo, tag)
}

// HashSecretData computes a deterministic SHA-256 hex digest of the provided map.
// Keys are sorted before hashing so the result is stable regardless of insertion order.
// This is used to annotate the StatefulSet so that config or secret changes trigger
// a rolling restart.
func HashSecretData(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write(data[k])
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashStringData computes a deterministic SHA-256 hex digest of string-keyed map data.
func HashStringData(data map[string]string) string {
	raw := make(map[string][]byte, len(data))
	for k, v := range data {
		raw[k] = []byte(v)
	}
	return HashSecretData(raw)
}
