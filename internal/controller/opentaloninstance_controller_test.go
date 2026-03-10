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

package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	opentalon "github.com/opentalon/k8s-operator/api/v1alpha1"
	"github.com/opentalon/k8s-operator/internal/resources"
)

// makeSts is a test helper that builds a minimal StatefulSet with the given
// replica count, container image, and config hash annotation.
func makeSts(replicas int32, image, configHash string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						resources.ConfigHashAnnotation: configHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: image},
					},
				},
			},
		},
	}
}

// TestReconcilerImplementsInterface verifies at compile time that
// OpenTalonInstanceReconciler satisfies the reconcile.Reconciler interface.
func TestReconcilerImplementsInterface(t *testing.T) {
	var _ reconcile.Reconciler = &OpenTalonInstanceReconciler{}
}

// TestSetConditionOnInstance verifies that setConditionOnInstance correctly
// sets a condition on the in-memory instance without touching the API server.
func TestSetConditionOnInstance(t *testing.T) {
	r := &OpenTalonInstanceReconciler{}

	inst := &opentalon.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	r.setConditionOnInstance(inst, opentalon.ConditionStatefulSetReady, func() (metav1.ConditionStatus, string, string) {
		return metav1.ConditionTrue, "StatefulSetReady", "all replicas ready"
	})

	cond := apimeta.FindStatusCondition(inst.Status.Conditions, opentalon.ConditionStatefulSetReady)
	if cond == nil {
		t.Fatalf("condition %q not found after setConditionOnInstance()", opentalon.ConditionStatefulSetReady)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
	if cond.Reason != "StatefulSetReady" {
		t.Errorf("condition reason = %q, want %q", cond.Reason, "StatefulSetReady")
	}
	if cond.Message != "all replicas ready" {
		t.Errorf("condition message = %q, want %q", cond.Message, "all replicas ready")
	}
}

// TestSetConditionOnInstance_Overwrite verifies that calling setConditionOnInstance
// a second time overwrites the previous value for the same condition type.
func TestSetConditionOnInstance_Overwrite(t *testing.T) {
	r := &OpenTalonInstanceReconciler{}
	inst := &opentalon.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	r.setConditionOnInstance(inst, opentalon.ConditionConfigMapReady, func() (metav1.ConditionStatus, string, string) {
		return metav1.ConditionFalse, "NotReady", "first call"
	})
	r.setConditionOnInstance(inst, opentalon.ConditionConfigMapReady, func() (metav1.ConditionStatus, string, string) {
		return metav1.ConditionTrue, "Ready", "second call"
	})

	cond := apimeta.FindStatusCondition(inst.Status.Conditions, opentalon.ConditionConfigMapReady)
	if cond == nil {
		t.Fatalf("condition %q not found", opentalon.ConditionConfigMapReady)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected condition to be overwritten to True, got %q", cond.Status)
	}
	if cond.Message != "second call" {
		t.Errorf("expected message %q, got %q", "second call", cond.Message)
	}

	// Only one condition entry of this type should be present.
	count := 0
	for _, c := range inst.Status.Conditions {
		if c.Type == opentalon.ConditionConfigMapReady {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 condition of type %q, got %d", opentalon.ConditionConfigMapReady, count)
	}
}

// TestStatefulSetNeedsUpdate verifies the diff helper that guards unnecessary writes.
func TestStatefulSetNeedsUpdate(t *testing.T) {
	t.Run("no diff returns false", func(t *testing.T) {
		replicas := int32(1)
		existing := makeSts(replicas, "img:v1", "hash1")
		desired := makeSts(replicas, "img:v1", "hash1")
		if statefulSetNeedsUpdate(existing, desired) {
			t.Error("statefulSetNeedsUpdate() = true, want false for identical specs")
		}
	})

	t.Run("replica change returns true", func(t *testing.T) {
		r1, r2 := int32(1), int32(3)
		existing := makeSts(r1, "img:v1", "hash1")
		desired := makeSts(r2, "img:v1", "hash1")
		if !statefulSetNeedsUpdate(existing, desired) {
			t.Error("statefulSetNeedsUpdate() = false, want true for replica change")
		}
	})

	t.Run("image change returns true", func(t *testing.T) {
		replicas := int32(1)
		existing := makeSts(replicas, "img:v1", "hash1")
		desired := makeSts(replicas, "img:v2", "hash1")
		if !statefulSetNeedsUpdate(existing, desired) {
			t.Error("statefulSetNeedsUpdate() = false, want true for image change")
		}
	})

	t.Run("config hash change returns true", func(t *testing.T) {
		replicas := int32(1)
		existing := makeSts(replicas, "img:v1", "hash1")
		desired := makeSts(replicas, "img:v1", "hash2")
		if !statefulSetNeedsUpdate(existing, desired) {
			t.Error("statefulSetNeedsUpdate() = false, want true for hash change")
		}
	})
}
