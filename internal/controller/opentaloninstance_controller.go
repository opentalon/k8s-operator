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
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opentalon "github.com/opentalon/k8s-operator/api/v1alpha1"
	"github.com/opentalon/k8s-operator/internal/resources"
)

// +kubebuilder:rbac:groups=opentalon.io,resources=opentaloninstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=opentalon.io,resources=opentaloninstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=opentalon.io,resources=opentaloninstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;secrets;persistentvolumeclaims;serviceaccounts;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// OpenTalonInstanceReconciler reconciles an OpenTalonInstance resource.
type OpenTalonInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager registers the controller with the manager and sets up watches
// for all owned resource types.
func (r *OpenTalonInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opentalon.OpenTalonInstance{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete(r)
}

// Reconcile is the main reconciliation loop invoked by controller-runtime when an
// OpenTalonInstance or any owned resource changes.
func (r *OpenTalonInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// ── Fetch the instance ────────────────────────────────────────────────────
	instance := &opentalon.OpenTalonInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch OpenTalonInstance")
		return ctrl.Result{}, err
	}

	// ── Deletion / finalizer handling ─────────────────────────────────────────
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, instance)
	}

	// Ensure our finalizer is present so we can clean up on delete.
	if !controllerutil.ContainsFinalizer(instance, resources.FinalizerName) {
		controllerutil.AddFinalizer(instance, resources.FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// ── Set initial Provisioning phase on first reconcile ─────────────────────
	if instance.Status.Phase == "" {
		if err := r.setPhase(ctx, instance, opentalon.PhaseProvisioning); err != nil {
			return ctrl.Result{}, err
		}
	}

	// ── Reconcile child resources ─────────────────────────────────────────────
	if err := r.reconcileResources(ctx, instance); err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "ReconcileError",
			"Failed to reconcile resources: %v", err)
		_ = r.setPhase(ctx, instance, opentalon.PhaseFailed)
		return ctrl.Result{}, err
	}

	// ── Update status from live StatefulSet ───────────────────────────────────
	result, err := r.syncStatus(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, nil
}

// reconcileDeletion handles cleanup when the instance is being deleted.
func (r *OpenTalonInstanceReconciler) reconcileDeletion(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(instance, resources.FinalizerName) {
		logger.Info("running finalizer cleanup", "instance", instance.Name)

		if err := r.setPhase(ctx, instance, opentalon.PhaseTerminating); err != nil {
			return ctrl.Result{}, err
		}

		// Child resources owned via OwnerReferences are garbage-collected
		// automatically by Kubernetes. We only need to clean up resources that
		// are not owned (none at the moment).

		controllerutil.RemoveFinalizer(instance, resources.FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileResources creates or updates all child resources for the instance.
func (r *OpenTalonInstanceReconciler) reconcileResources(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
) error {
	managedResources := []string{}

	// 1. ServiceAccount ────────────────────────────────────────────────────────
	if instance.Spec.ServiceAccountName == "" {
		sa := resources.BuildServiceAccount(instance)
		if err := r.createOrUpdateServiceAccount(ctx, instance, sa); err != nil {
			return fmt.Errorf("ServiceAccount: %w", err)
		}
		managedResources = append(managedResources, "ServiceAccount/"+sa.Name)
		r.setCondition(ctx, instance, opentalon.ConditionRBACReady, metav1.ConditionTrue, "ServiceAccountReady", "ServiceAccount reconciled")
	}

	// 2. Role ──────────────────────────────────────────────────────────────────
	role := resources.BuildRole(instance)
	if err := r.createOrUpdateRole(ctx, instance, role); err != nil {
		return fmt.Errorf("role: %w", err)
	}
	managedResources = append(managedResources, "Role/"+role.Name)

	// 3. RoleBinding ───────────────────────────────────────────────────────────
	rb := resources.BuildRoleBinding(instance)
	if err := r.createOrUpdateRoleBinding(ctx, instance, rb); err != nil {
		return fmt.Errorf("RoleBinding: %w", err)
	}
	managedResources = append(managedResources, "RoleBinding/"+rb.Name)
	r.setCondition(ctx, instance, opentalon.ConditionRBACReady, metav1.ConditionTrue, "RBACReady", "Role and RoleBinding reconciled")

	// 4. ConfigMap ─────────────────────────────────────────────────────────────
	// When ConfigFrom is set, the user provides the config externally; skip
	// generating the ConfigMap but still track the reference.
	var configHash string
	if instance.Spec.ConfigFrom != nil {
		// Read the external ConfigMap to compute a config hash for rollout detection.
		externalCM := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      instance.Spec.ConfigFrom.Name,
		}, externalCM); err != nil {
			if apierrors.IsNotFound(err) {
				r.setCondition(ctx, instance, opentalon.ConditionConfigMapReady, metav1.ConditionFalse,
					"ExternalConfigMapNotFound", fmt.Sprintf("referenced ConfigMap %q not found", instance.Spec.ConfigFrom.Name))
				return fmt.Errorf("external ConfigMap %q not found: %w", instance.Spec.ConfigFrom.Name, err)
			}
			return err
		}
		configHash = resources.HashStringData(externalCM.Data)
		r.setCondition(ctx, instance, opentalon.ConditionConfigMapReady, metav1.ConditionTrue, "ExternalConfigMapFound", "External ConfigMap found")
	} else {
		cm := resources.BuildConfigMap(instance)
		if err := r.createOrUpdateConfigMap(ctx, instance, cm); err != nil {
			return fmt.Errorf("ConfigMap: %w", err)
		}
		configHash = resources.HashStringData(cm.Data)
		managedResources = append(managedResources, "ConfigMap/"+cm.Name)
		r.setCondition(ctx, instance, opentalon.ConditionConfigMapReady, metav1.ConditionTrue, "ConfigMapReady", "ConfigMap reconciled")
	}

	// 5. StatefulSet ───────────────────────────────────────────────────────────
	sts := resources.BuildStatefulSet(instance, configHash)
	if err := r.createOrUpdateStatefulSet(ctx, instance, sts); err != nil {
		return fmt.Errorf("StatefulSet: %w", err)
	}
	managedResources = append(managedResources, "StatefulSet/"+sts.Name)

	// 6. Service ───────────────────────────────────────────────────────────────
	svc := resources.BuildService(instance)
	if err := r.createOrUpdateService(ctx, instance, svc); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	managedResources = append(managedResources, "Service/"+svc.Name)
	r.setCondition(ctx, instance, opentalon.ConditionServiceReady, metav1.ConditionTrue, "ServiceReady", "Service reconciled")

	// 7. Ingress (optional) ────────────────────────────────────────────────────
	if instance.Spec.Networking.Ingress.Enabled {
		ingress := resources.BuildIngress(instance)
		if err := r.createOrUpdateIngress(ctx, instance, ingress); err != nil {
			return fmt.Errorf("ingress: %w", err)
		}
		managedResources = append(managedResources, "Ingress/"+ingress.Name)
	}

	// 8. NetworkPolicy (optional) ──────────────────────────────────────────────
	if instance.Spec.Networking.NetworkPolicy.Enabled {
		np := resources.BuildNetworkPolicy(instance)
		if err := r.createOrUpdateNetworkPolicy(ctx, instance, np); err != nil {
			return fmt.Errorf("NetworkPolicy: %w", err)
		}
		managedResources = append(managedResources, "NetworkPolicy/"+np.Name)
		r.setCondition(ctx, instance, opentalon.ConditionNetworkPolicyReady, metav1.ConditionTrue, "NetworkPolicyReady", "NetworkPolicy reconciled")
	}

	// 9. ServiceMonitor (optional) ─────────────────────────────────────────────
	if instance.Spec.Observability.Metrics.ServiceMonitor.Enabled {
		sm := resources.BuildServiceMonitor(instance)
		if err := r.createOrUpdateServiceMonitor(ctx, instance, sm); err != nil {
			// ServiceMonitor creation may fail if the CRD is not installed.
			// Log but do not fail the reconcile so the operator works without
			// the Prometheus Operator.
			log.FromContext(ctx).Info("ServiceMonitor reconcile skipped (CRD may not be installed)",
				"error", err.Error())
			r.setCondition(ctx, instance, opentalon.ConditionServiceMonitorReady, metav1.ConditionFalse,
				"ServiceMonitorCRDMissing", err.Error())
		} else {
			managedResources = append(managedResources, "ServiceMonitor/"+sm.GetName())
			r.setCondition(ctx, instance, opentalon.ConditionServiceMonitorReady, metav1.ConditionTrue, "ServiceMonitorReady", "ServiceMonitor reconciled")
		}
	}

	// 10. PodDisruptionBudget (optional) ───────────────────────────────────────
	if instance.Spec.Availability.PodDisruptionBudget.Enabled {
		pdb := resources.BuildPDB(instance)
		if err := r.createOrUpdatePDB(ctx, instance, pdb); err != nil {
			return fmt.Errorf("PodDisruptionBudget: %w", err)
		}
		managedResources = append(managedResources, "PodDisruptionBudget/"+pdb.Name)
	}

	// 11. HorizontalPodAutoscaler (optional) ───────────────────────────────────
	if instance.Spec.Availability.HorizontalPodAutoscaler.Enabled {
		hpa := resources.BuildHPA(instance)
		if err := r.createOrUpdateHPA(ctx, instance, hpa); err != nil {
			return fmt.Errorf("HorizontalPodAutoscaler: %w", err)
		}
		managedResources = append(managedResources, "HorizontalPodAutoscaler/"+hpa.Name)
	}

	// Persist the managed resource list.
	sort.Strings(managedResources)
	patch := client.MergeFrom(instance.DeepCopy())
	instance.Status.ManagedResources = managedResources
	instance.Status.CurrentImage = resources.ImageRef(instance.Spec.Image)
	now := metav1.Now()
	instance.Status.LastUpdateTime = &now
	instance.Status.ObservedGeneration = instance.Generation
	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		return err
	}

	return nil
}

// syncStatus fetches the live StatefulSet and updates the instance phase and ready replicas.
func (r *OpenTalonInstanceReconciler) syncStatus(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
) (ctrl.Result, error) {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      resources.ResourceName(instance),
	}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	desiredReplicas := int32(1)
	if instance.Spec.Replicas != nil {
		desiredReplicas = *instance.Spec.Replicas
	}

	ready := sts.Status.ReadyReplicas
	patch := client.MergeFrom(instance.DeepCopy())
	instance.Status.ReadyReplicas = ready

	var phase string
	switch {
	case sts.Status.ObservedGeneration < sts.Generation:
		phase = opentalon.PhaseProvisioning
	case ready == 0 && desiredReplicas > 0:
		phase = opentalon.PhaseDegraded
	case ready < desiredReplicas:
		phase = opentalon.PhaseDegraded
	default:
		phase = opentalon.PhaseRunning
	}

	instance.Status.Phase = phase
	r.setConditionOnInstance(instance, opentalon.ConditionStatefulSetReady,
		func() (metav1.ConditionStatus, string, string) {
			if phase == opentalon.PhaseRunning {
				return metav1.ConditionTrue, "StatefulSetReady",
					fmt.Sprintf("%d/%d replicas ready", ready, desiredReplicas)
			}
			return metav1.ConditionFalse, "StatefulSetNotReady",
				fmt.Sprintf("%d/%d replicas ready", ready, desiredReplicas)
		})

	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}

	if phase != opentalon.PhaseRunning {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// ── Create-or-update helpers ──────────────────────────────────────────────────

func (r *OpenTalonInstanceReconciler) createOrUpdateConfigMap(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *corev1.ConfigMap,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.Data, desired.Data) {
		existing.Data = desired.Data
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *OpenTalonInstanceReconciler) createOrUpdateServiceAccount(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *corev1.ServiceAccount,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *OpenTalonInstanceReconciler) createOrUpdateRole(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *rbacv1.Role,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &rbacv1.Role{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.Rules, desired.Rules) {
		existing.Rules = desired.Rules
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *OpenTalonInstanceReconciler) createOrUpdateRoleBinding(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *rbacv1.RoleBinding,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &rbacv1.RoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.RoleRef, desired.RoleRef) ||
		!equality.Semantic.DeepEqual(existing.Subjects, desired.Subjects) {
		existing.RoleRef = desired.RoleRef
		existing.Subjects = desired.Subjects
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *OpenTalonInstanceReconciler) createOrUpdateStatefulSet(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *appsv1.StatefulSet,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		r.Recorder.Eventf(instance, corev1.EventTypeNormal, "Created", "Created StatefulSet %s", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Preserve the existing resource version and carry over immutable fields.
	desired.ResourceVersion = existing.ResourceVersion
	// VolumeClaimTemplates cannot be changed after creation; preserve them.
	desired.Spec.VolumeClaimTemplates = existing.Spec.VolumeClaimTemplates

	// Only update if the spec has materially changed to avoid needless writes.
	if !statefulSetNeedsUpdate(existing, desired) {
		return nil
	}

	r.Recorder.Eventf(instance, corev1.EventTypeNormal, "Updated", "Updated StatefulSet %s", desired.Name)
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template = desired.Spec.Template
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *OpenTalonInstanceReconciler) createOrUpdateService(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *corev1.Service,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	// Preserve ClusterIP which is immutable once assigned.
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	desired.ResourceVersion = existing.ResourceVersion
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations
	return r.Update(ctx, existing)
}

func (r *OpenTalonInstanceReconciler) createOrUpdateIngress(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *networkingv1.Ingress,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations
	return r.Update(ctx, existing)
}

func (r *OpenTalonInstanceReconciler) createOrUpdateNetworkPolicy(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *networkingv1.NetworkPolicy,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *OpenTalonInstanceReconciler) createOrUpdateServiceMonitor(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *unstructured.Unstructured,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		// ServiceMonitor may not be in the scheme; log and continue.
		log.FromContext(ctx).V(1).Info("could not set owner reference on ServiceMonitor", "error", err)
	}
	desired.SetGroupVersionKind(desired.GroupVersionKind())

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())

	err := r.Get(ctx, types.NamespacedName{Namespace: desired.GetNamespace(), Name: desired.GetName()}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

func (r *OpenTalonInstanceReconciler) createOrUpdatePDB(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *policyv1.PodDisruptionBudget,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *OpenTalonInstanceReconciler) createOrUpdateHPA(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	desired *autoscalingv2.HorizontalPodAutoscaler,
) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// ── Status helpers ────────────────────────────────────────────────────────────

// setPhase updates only the phase field in the status subresource.
func (r *OpenTalonInstanceReconciler) setPhase(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	phase string,
) error {
	patch := client.MergeFrom(instance.DeepCopy())
	instance.Status.Phase = phase
	now := metav1.Now()
	instance.Status.LastUpdateTime = &now
	err := r.Status().Patch(ctx, instance, patch)
	if apierrors.IsConflict(err) {
		return nil
	}
	return err
}

// setCondition patches a single condition on the instance status.
// It re-fetches the instance before patching to avoid conflicts.
func (r *OpenTalonInstanceReconciler) setCondition(
	ctx context.Context,
	instance *opentalon.OpenTalonInstance,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	// Best-effort: log but don't fail the reconcile on condition patch errors.
	patch := client.MergeFrom(instance.DeepCopy())
	apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: instance.Generation,
	})
	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		log.FromContext(ctx).V(1).Info("failed to patch condition", "type", condType, "error", err)
	}
}

// setConditionOnInstance sets a condition on the in-memory instance without patching.
// Used in syncStatus before the outer patch.
func (r *OpenTalonInstanceReconciler) setConditionOnInstance(
	instance *opentalon.OpenTalonInstance,
	condType string,
	fn func() (metav1.ConditionStatus, string, string),
) {
	status, reason, message := fn()
	apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: instance.Generation,
	})
}

// ── Diff helpers ──────────────────────────────────────────────────────────────

// statefulSetNeedsUpdate returns true when the existing StatefulSet spec differs
// from the desired spec in ways the operator manages (replicas, pod template).
func statefulSetNeedsUpdate(existing, desired *appsv1.StatefulSet) bool {
	if !equality.Semantic.DeepEqual(existing.Spec.Replicas, desired.Spec.Replicas) {
		return true
	}
	// Compare the config hash annotation to detect config-driven rollouts.
	existingHash := existing.Spec.Template.Annotations[resources.ConfigHashAnnotation]
	desiredHash := desired.Spec.Template.Annotations[resources.ConfigHashAnnotation]
	if existingHash != desiredHash {
		return true
	}
	// Compare container images.
	if len(existing.Spec.Template.Spec.Containers) > 0 &&
		len(desired.Spec.Template.Spec.Containers) > 0 {
		if existing.Spec.Template.Spec.Containers[0].Image !=
			desired.Spec.Template.Spec.Containers[0].Image {
			return true
		}
	}
	return false
}
