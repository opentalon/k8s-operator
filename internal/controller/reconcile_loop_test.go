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
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	opentalon "github.com/opentalon/k8s-operator/api/v1alpha1"
	"github.com/opentalon/k8s-operator/internal/resources"
)

const (
	envtestTimeout = 20 * time.Second
	envtestPoll    = 100 * time.Millisecond
	idleWindow     = 3 * time.Second
)

func startEnvtest(t *testing.T) client.Client {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS is not set, run via make test")
	}

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			t.Logf("stop envtest: %v", err)
		}
	})

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := opentalon.AddToScheme(scheme); err != nil {
		t.Fatalf("add opentalon scheme: %v", err)
	}

	ctrl.SetLogger(logr.Discard())
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	r := &OpenTalonInstanceReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: record.NewFakeRecorder(1024),
	}
	if err := r.SetupWithManager(mgr); err != nil {
		t.Fatalf("setup reconciler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := mgr.Start(ctx); err != nil {
			t.Errorf("manager stopped with error: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return c
}

func waitFor(t *testing.T, what string, cond func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(envtestTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := cond()
		if ok {
			return
		}
		lastErr = err
		time.Sleep(envtestPoll)
	}
	t.Fatalf("timed out after %s waiting for %s, last error: %v", envtestTimeout, what, lastErr)
}

func createNamespace(t *testing.T, c client.Client, name string) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Create(context.Background(), ns); err != nil {
		t.Fatalf("create namespace %s: %v", name, err)
	}
}

func newLoopInstance(namespace string) *opentalon.OpenTalonInstance {
	minAvailable := int32(1)
	maxReplicas := int32(3)
	return &opentalon.OpenTalonInstance{
		ObjectMeta: metav1.ObjectMeta{Name: "loop", Namespace: namespace},
		Spec: opentalon.OpenTalonInstanceSpec{
			Image: opentalon.ImageSpec{Repository: "ghcr.io/opentalon/opentalon", Tag: "v1.0.0"},
			Networking: opentalon.NetworkingSpec{
				Ingress:       opentalon.IngressSpec{Enabled: true, Host: "loop.example.com"},
				NetworkPolicy: opentalon.NetworkPolicySpec{Enabled: true},
			},
			Availability: opentalon.AvailabilitySpec{
				PodDisruptionBudget:     opentalon.PodDisruptionBudgetSpec{Enabled: true, MinAvailable: &minAvailable},
				HorizontalPodAutoscaler: opentalon.HPASpec{Enabled: true, MaxReplicas: maxReplicas},
			},
		},
	}
}

func markStatefulSetReady(t *testing.T, c client.Client, key types.NamespacedName) {
	t.Helper()
	ctx := context.Background()
	waitFor(t, "StatefulSet "+key.String()+" to become ready", func() (bool, error) {
		sts := &appsv1.StatefulSet{}
		if err := c.Get(ctx, key, sts); err != nil {
			return false, err
		}
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		sts.Status.ObservedGeneration = sts.Generation
		sts.Status.Replicas = replicas
		sts.Status.ReadyReplicas = replicas
		sts.Status.AvailableReplicas = replicas
		sts.Status.CurrentReplicas = replicas
		sts.Status.UpdatedReplicas = replicas
		if err := c.Status().Update(ctx, sts); err != nil {
			return false, err
		}
		return true, nil
	})
}

func waitForPhase(t *testing.T, c client.Client, key types.NamespacedName, phase string) {
	t.Helper()
	waitFor(t, "instance "+key.String()+" phase "+phase, func() (bool, error) {
		inst := &opentalon.OpenTalonInstance{}
		if err := c.Get(context.Background(), key, inst); err != nil {
			return false, err
		}
		return inst.Status.Phase == phase, nil
	})
}

func resourceVersions(t *testing.T, c client.Client, namespace string) map[string]string {
	t.Helper()
	ctx := context.Background()
	lists := map[string]client.ObjectList{
		"OpenTalonInstance":       &opentalon.OpenTalonInstanceList{},
		"ServiceAccount":          &corev1.ServiceAccountList{},
		"ConfigMap":               &corev1.ConfigMapList{},
		"Service":                 &corev1.ServiceList{},
		"StatefulSet":             &appsv1.StatefulSetList{},
		"Role":                    &rbacv1.RoleList{},
		"RoleBinding":             &rbacv1.RoleBindingList{},
		"Ingress":                 &networkingv1.IngressList{},
		"NetworkPolicy":           &networkingv1.NetworkPolicyList{},
		"PodDisruptionBudget":     &policyv1.PodDisruptionBudgetList{},
		"HorizontalPodAutoscaler": &autoscalingv2.HorizontalPodAutoscalerList{},
	}
	out := map[string]string{}
	for kind, list := range lists {
		if err := c.List(ctx, list, client.InNamespace(namespace)); err != nil {
			t.Fatalf("list %s in %s: %v", kind, namespace, err)
		}
		items, err := apimeta.ExtractList(list)
		if err != nil {
			t.Fatalf("extract %s items: %v", kind, err)
		}
		for _, item := range items {
			obj, err := apimeta.Accessor(item)
			if err != nil {
				t.Fatalf("access %s metadata: %v", kind, err)
			}
			out[kind+"/"+obj.GetName()] = obj.GetResourceVersion()
		}
	}
	return out
}

func setupRunningInstance(t *testing.T, c client.Client, namespace string) *opentalon.OpenTalonInstance {
	t.Helper()
	createNamespace(t, c, namespace)
	inst := newLoopInstance(namespace)
	if err := c.Create(context.Background(), inst); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	key := client.ObjectKeyFromObject(inst)
	markStatefulSetReady(t, c, types.NamespacedName{Namespace: namespace, Name: resources.ResourceName(inst)})
	waitForPhase(t, c, key, opentalon.PhaseRunning)
	return inst
}

func TestReconcileLoop(t *testing.T) {
	c := startEnvtest(t)
	ctx := context.Background()

	t.Run("idle running instance stops writing", func(t *testing.T) {
		inst := setupRunningInstance(t, c, "idle")

		time.Sleep(time.Second)
		before := resourceVersions(t, c, inst.Namespace)
		time.Sleep(idleWindow)
		after := resourceVersions(t, c, inst.Namespace)

		if len(before) < 11 {
			t.Fatalf("expected the instance and all ten child kinds to exist, got %d objects: %v", len(before), before)
		}
		for name, rv := range before {
			if after[name] != rv {
				t.Errorf("%s resourceVersion changed from %s to %s on an idle instance", name, rv, after[name])
			}
		}
	})

	t.Run("lastUpdateTime only moves on phase change", func(t *testing.T) {
		inst := setupRunningInstance(t, c, "lastupdate")
		key := client.ObjectKeyFromObject(inst)

		first := &opentalon.OpenTalonInstance{}
		if err := c.Get(ctx, key, first); err != nil {
			t.Fatalf("get instance: %v", err)
		}
		if first.Status.LastUpdateTime == nil {
			t.Fatalf("lastUpdateTime is nil after reaching Running")
		}
		time.Sleep(1100 * time.Millisecond)

		svc := &corev1.Service{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: resources.ResourceName(inst)}, svc); err != nil {
			t.Fatalf("get service: %v", err)
		}
		svc.Labels["drift"] = "yes"
		if err := c.Update(ctx, svc); err != nil {
			t.Fatalf("update service labels: %v", err)
		}
		waitFor(t, "service drift label removed", func() (bool, error) {
			got := &corev1.Service{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(svc), got); err != nil {
				return false, err
			}
			_, ok := got.Labels["drift"]
			return !ok, nil
		})

		second := &opentalon.OpenTalonInstance{}
		if err := c.Get(ctx, key, second); err != nil {
			t.Fatalf("get instance: %v", err)
		}
		if !second.Status.LastUpdateTime.Equal(first.Status.LastUpdateTime) {
			t.Errorf("lastUpdateTime moved from %s to %s without a phase change", first.Status.LastUpdateTime, second.Status.LastUpdateTime)
		}
	})

	t.Run("spec change still rolls out", func(t *testing.T) {
		inst := setupRunningInstance(t, c, "specchange")
		key := client.ObjectKeyFromObject(inst)

		live := &opentalon.OpenTalonInstance{}
		if err := c.Get(ctx, key, live); err != nil {
			t.Fatalf("get instance: %v", err)
		}
		live.Spec.Image.Tag = "v2.0.0"
		if err := c.Update(ctx, live); err != nil {
			t.Fatalf("update instance image: %v", err)
		}

		want := "ghcr.io/opentalon/opentalon:v2.0.0"
		waitFor(t, "StatefulSet image "+want, func() (bool, error) {
			sts := &appsv1.StatefulSet{}
			if err := c.Get(ctx, types.NamespacedName{Namespace: inst.Namespace, Name: resources.ResourceName(inst)}, sts); err != nil {
				return false, err
			}
			return sts.Spec.Template.Spec.Containers[0].Image == want, nil
		})
		waitFor(t, "status.currentImage "+want, func() (bool, error) {
			got := &opentalon.OpenTalonInstance{}
			if err := c.Get(ctx, key, got); err != nil {
				return false, err
			}
			return got.Status.CurrentImage == want && got.Status.ObservedGeneration == got.Generation, nil
		})
	})

	t.Run("child drift is reverted", func(t *testing.T) {
		inst := setupRunningInstance(t, c, "drift")
		name := types.NamespacedName{Namespace: inst.Namespace, Name: resources.ResourceName(inst)}

		svc := &corev1.Service{}
		if err := c.Get(ctx, name, svc); err != nil {
			t.Fatalf("get service: %v", err)
		}
		wantPort := svc.Spec.Ports[0].Port
		wantSelectorLen := len(svc.Spec.Selector)
		svc.Spec.Ports[0].Port = 9999
		svc.Spec.Selector = map[string]string{"hijacked": "true"}
		if err := c.Update(ctx, svc); err != nil {
			t.Fatalf("update service: %v", err)
		}
		waitFor(t, "service ports and selector reverted", func() (bool, error) {
			got := &corev1.Service{}
			if err := c.Get(ctx, name, got); err != nil {
				return false, err
			}
			return got.Spec.Ports[0].Port == wantPort && got.Spec.Selector["hijacked"] == "" && len(got.Spec.Selector) == wantSelectorLen, nil
		})

		sa := &corev1.ServiceAccount{}
		if err := c.Get(ctx, name, sa); err != nil {
			t.Fatalf("get service account: %v", err)
		}
		sa.Labels = map[string]string{"hijacked": "true"}
		if err := c.Update(ctx, sa); err != nil {
			t.Fatalf("update service account: %v", err)
		}
		waitFor(t, "service account labels reverted", func() (bool, error) {
			got := &corev1.ServiceAccount{}
			if err := c.Get(ctx, name, got); err != nil {
				return false, err
			}
			return got.Labels["hijacked"] == "" && got.Labels["app.kubernetes.io/instance"] == inst.Name, nil
		})

		ing := &networkingv1.Ingress{}
		if err := c.Get(ctx, name, ing); err != nil {
			t.Fatalf("get ingress: %v", err)
		}
		ing.Spec.Rules[0].Host = "evil.example.com"
		if err := c.Update(ctx, ing); err != nil {
			t.Fatalf("update ingress: %v", err)
		}
		waitFor(t, "ingress host reverted", func() (bool, error) {
			got := &networkingv1.Ingress{}
			if err := c.Get(ctx, name, got); err != nil {
				return false, err
			}
			return got.Spec.Rules[0].Host == "loop.example.com", nil
		})

		np := &networkingv1.NetworkPolicy{}
		if err := c.Get(ctx, name, np); err != nil {
			t.Fatalf("get network policy: %v", err)
		}
		np.Spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{"hijacked": "true"}}
		if err := c.Update(ctx, np); err != nil {
			t.Fatalf("update network policy: %v", err)
		}
		waitFor(t, "network policy pod selector reverted", func() (bool, error) {
			got := &networkingv1.NetworkPolicy{}
			if err := c.Get(ctx, name, got); err != nil {
				return false, err
			}
			return got.Spec.PodSelector.MatchLabels["hijacked"] == "", nil
		})

		pdb := &policyv1.PodDisruptionBudget{}
		if err := c.Get(ctx, name, pdb); err != nil {
			t.Fatalf("get pdb: %v", err)
		}
		pdb.Labels = map[string]string{"hijacked": "true"}
		if err := c.Update(ctx, pdb); err != nil {
			t.Fatalf("update pdb: %v", err)
		}
		waitFor(t, "pdb labels reverted", func() (bool, error) {
			got := &policyv1.PodDisruptionBudget{}
			if err := c.Get(ctx, name, got); err != nil {
				return false, err
			}
			return got.Labels["hijacked"] == "", nil
		})

		hpa := &autoscalingv2.HorizontalPodAutoscaler{}
		if err := c.Get(ctx, name, hpa); err != nil {
			t.Fatalf("get hpa: %v", err)
		}
		hpa.Spec.MaxReplicas = 42
		if err := c.Update(ctx, hpa); err != nil {
			t.Fatalf("update hpa: %v", err)
		}
		waitFor(t, "hpa maxReplicas reverted", func() (bool, error) {
			got := &autoscalingv2.HorizontalPodAutoscaler{}
			if err := c.Get(ctx, name, got); err != nil {
				return false, err
			}
			return got.Spec.MaxReplicas == 3, nil
		})
	})

	t.Run("deletion still removes the finalizer", func(t *testing.T) {
		inst := setupRunningInstance(t, c, "deletion")
		key := client.ObjectKeyFromObject(inst)

		live := &opentalon.OpenTalonInstance{}
		if err := c.Get(ctx, key, live); err != nil {
			t.Fatalf("get instance: %v", err)
		}
		if len(live.Finalizers) == 0 {
			t.Fatalf("expected finalizer on instance %s, got none", key)
		}
		if err := c.Delete(ctx, live); err != nil {
			t.Fatalf("delete instance: %v", err)
		}
		waitFor(t, "instance "+key.String()+" to be gone", func() (bool, error) {
			err := c.Get(ctx, key, &opentalon.OpenTalonInstance{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		})
	})
}
