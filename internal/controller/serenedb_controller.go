/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	databasev1alpha1 "github.com/4thel00z/serenedb-operator/api/v1alpha1"
	"github.com/4thel00z/serenedb-operator/internal/resources"
)

const (
	reasonPodReady          = "PodReady"
	reasonPodNotReady       = "PodNotReady"
	reasonRollingOut        = "RollingOut"
	reasonStable            = "Stable"
	reasonReconcileError    = "ReconcileError"
	reasonSecretMissing     = "SecretMissing"
	reasonSecretKeyMissing  = "SecretKeyMissing"
	reasonTLSSecretMissing  = "TLSSecretMissing"
	reasonTLSSecretRequired = "TLSSecretNameRequired"

	missingDependencyRequeue = 30 * time.Second
)

// SereneDBReconciler reconciles a SereneDB object.
type SereneDBReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// blocked describes a precondition the user must fix before the database can be rolled out.
type blocked struct {
	reason  string
	message string
}

func (b *blocked) Error() string {
	return b.message
}

// +kubebuilder:rbac:groups=database.serenedb.com,resources=serenedbs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.serenedb.com,resources=serenedbs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.serenedb.com,resources=serenedbs/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives the owned Secret, ConfigMap, Services, StatefulSet and NetworkPolicy
// towards the SereneDB spec and reports the result in status.
func (r *SereneDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	db := &databasev1alpha1.SereneDB{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !db.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	original := db.DeepCopy()
	sts, err := r.reconcileResources(ctx, db)
	describe(db, sts, err)
	if statusErr := r.Status().Patch(ctx, db, client.MergeFrom(original)); statusErr != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", statusErr)
	}

	var precondition *blocked
	if asBlocked(err, &precondition) {
		log.Info("waiting for precondition", "reason", precondition.reason, "message", precondition.message)
		return ctrl.Result{RequeueAfter: missingDependencyRequeue}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *SereneDBReconciler) reconcileResources(ctx context.Context, db *databasev1alpha1.SereneDB) (*appsv1.StatefulSet, error) {
	if err := r.reconcileSecret(ctx, db); err != nil {
		return nil, err
	}
	if err := r.checkTLSSecret(ctx, db); err != nil {
		return nil, err
	}
	configHash, err := r.reconcileConfigMap(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := r.reconcileServices(ctx, db); err != nil {
		return nil, err
	}
	sts, err := r.reconcileStatefulSet(ctx, db, configHash)
	if err != nil {
		return nil, err
	}
	if err := r.reconcileNetworkPolicy(ctx, db); err != nil {
		return nil, err
	}
	return sts, nil
}

func (r *SereneDBReconciler) reconcileSecret(ctx context.Context, db *databasev1alpha1.SereneDB) error {
	if db.Spec.Auth.ExistingSecret != "" {
		return r.checkExistingSecret(ctx, db)
	}

	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: db.Namespace, Name: resources.Name(db)}, existing)
	if apierrors.IsNotFound(err) {
		return r.createGeneratedSecret(ctx, db)
	}
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}
	key := resources.PasswordKey(db)
	if _, ok := existing.Data[key]; !ok {
		return &blocked{reasonSecretKeyMissing, fmt.Sprintf("secret %q has no key %q", existing.Name, key)}
	}
	return r.syncSecretOwnership(ctx, db, existing)
}

func (r *SereneDBReconciler) checkExistingSecret(ctx context.Context, db *databasev1alpha1.SereneDB) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: db.Namespace, Name: db.Spec.Auth.ExistingSecret}, secret)
	if apierrors.IsNotFound(err) {
		return &blocked{reasonSecretMissing, fmt.Sprintf("secret %q not found", db.Spec.Auth.ExistingSecret)}
	}
	if err != nil {
		return fmt.Errorf("get existing secret: %w", err)
	}
	key := resources.PasswordKey(db)
	if _, ok := secret.Data[key]; !ok {
		return &blocked{reasonSecretKeyMissing, fmt.Sprintf("secret %q has no key %q", db.Spec.Auth.ExistingSecret, key)}
	}
	return nil
}

func (r *SereneDBReconciler) createGeneratedSecret(ctx context.Context, db *databasev1alpha1.SereneDB) error {
	secret, err := resources.GeneratedSecret(db)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	if resources.SecretOwnedByDatabase(db) {
		if err := controllerutil.SetControllerReference(db, secret, r.Scheme); err != nil {
			return fmt.Errorf("set secret owner: %w", err)
		}
	}
	if err := r.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create secret: %w", err)
	}
	return nil
}

func (r *SereneDBReconciler) syncSecretOwnership(ctx context.Context, db *databasev1alpha1.SereneDB, secret *corev1.Secret) error {
	wantOwned := resources.SecretOwnedByDatabase(db)
	isOwned := metav1.IsControlledBy(secret, db)
	if wantOwned == isOwned {
		return nil
	}
	patch := client.MergeFrom(secret.DeepCopy())
	if wantOwned {
		if err := controllerutil.SetControllerReference(db, secret, r.Scheme); err != nil {
			return fmt.Errorf("set secret owner: %w", err)
		}
	}
	if !wantOwned {
		if err := controllerutil.RemoveControllerReference(db, secret, r.Scheme); err != nil {
			return fmt.Errorf("remove secret owner: %w", err)
		}
	}
	if err := r.Patch(ctx, secret, patch); err != nil {
		return fmt.Errorf("patch secret owner: %w", err)
	}
	return nil
}

func (r *SereneDBReconciler) checkTLSSecret(ctx context.Context, db *databasev1alpha1.SereneDB) error {
	if !db.Spec.TLS.Enabled {
		return nil
	}
	if db.Spec.TLS.SecretName == "" {
		return &blocked{reasonTLSSecretRequired, "tls.secretName is required when tls.enabled is true"}
	}
	err := r.Get(ctx, types.NamespacedName{Namespace: db.Namespace, Name: db.Spec.TLS.SecretName}, &corev1.Secret{})
	if apierrors.IsNotFound(err) {
		return &blocked{reasonTLSSecretMissing, fmt.Sprintf("tls secret %q not found", db.Spec.TLS.SecretName)}
	}
	if err != nil {
		return fmt.Errorf("get tls secret: %w", err)
	}
	return nil
}

func (r *SereneDBReconciler) reconcileConfigMap(ctx context.Context, db *databasev1alpha1.SereneDB) (string, error) {
	desired := resources.ConfigMap(db)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = mergeMaps(cm.Labels, desired.Labels)
		cm.Data = desired.Data
		return controllerutil.SetControllerReference(db, cm, r.Scheme)
	})
	if err != nil {
		return "", fmt.Errorf("reconcile configmap: %w", err)
	}
	return resources.ConfigHash(desired), nil
}

func (r *SereneDBReconciler) reconcileServices(ctx context.Context, db *databasev1alpha1.SereneDB) error {
	if err := r.applyService(ctx, db, resources.Service(db)); err != nil {
		return fmt.Errorf("reconcile service: %w", err)
	}
	if err := r.applyService(ctx, db, resources.HeadlessService(db)); err != nil {
		return fmt.Errorf("reconcile headless service: %w", err)
	}
	return nil
}

func (r *SereneDBReconciler) applyService(ctx context.Context, db *databasev1alpha1.SereneDB, desired *corev1.Service) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = mergeMaps(svc.Labels, desired.Labels)
		svc.Annotations = mergeMaps(svc.Annotations, desired.Annotations)
		svc.Spec.Type = desired.Spec.Type
		svc.Spec.Selector = desired.Spec.Selector
		svc.Spec.PublishNotReadyAddresses = desired.Spec.PublishNotReadyAddresses
		svc.Spec.Ports = keepAssignedNodePorts(svc.Spec.Ports, desired.Spec.Ports)
		if svc.ResourceVersion == "" {
			svc.Spec.ClusterIP = desired.Spec.ClusterIP
		}
		return controllerutil.SetControllerReference(db, svc, r.Scheme)
	})
	return err
}

func keepAssignedNodePorts(current, desired []corev1.ServicePort) []corev1.ServicePort {
	assigned := make(map[string]int32, len(current))
	for _, p := range current {
		assigned[p.Name] = p.NodePort
	}
	for i := range desired {
		if desired[i].NodePort != 0 {
			continue
		}
		desired[i].NodePort = assigned[desired[i].Name]
	}
	return desired
}

func (r *SereneDBReconciler) reconcileStatefulSet(ctx context.Context, db *databasev1alpha1.SereneDB, configHash string) (*appsv1.StatefulSet, error) {
	desired := resources.StatefulSet(db, configHash)
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		sts.Labels = mergeMaps(sts.Labels, desired.Labels)
		if sts.ResourceVersion == "" {
			sts.Spec = desired.Spec
			return controllerutil.SetControllerReference(db, sts, r.Scheme)
		}
		sts.Spec.Replicas = desired.Spec.Replicas
		sts.Spec.Template = desired.Spec.Template
		sts.Spec.UpdateStrategy = desired.Spec.UpdateStrategy
		sts.Spec.PersistentVolumeClaimRetentionPolicy = desired.Spec.PersistentVolumeClaimRetentionPolicy
		return controllerutil.SetControllerReference(db, sts, r.Scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile statefulset: %w", err)
	}
	return sts, nil
}

func (r *SereneDBReconciler) reconcileNetworkPolicy(ctx context.Context, db *databasev1alpha1.SereneDB) error {
	desired := resources.NetworkPolicy(db)
	if !db.Spec.NetworkPolicy.Enabled {
		return r.deleteOwnedNetworkPolicy(ctx, db, desired)
	}
	np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: desired.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = mergeMaps(np.Labels, desired.Labels)
		np.Spec = desired.Spec
		return controllerutil.SetControllerReference(db, np, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("reconcile networkpolicy: %w", err)
	}
	return nil
}

func (r *SereneDBReconciler) deleteOwnedNetworkPolicy(ctx context.Context, db *databasev1alpha1.SereneDB, desired *networkingv1.NetworkPolicy) error {
	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get networkpolicy: %w", err)
	}
	if !metav1.IsControlledBy(existing, db) {
		return nil
	}
	if err := r.Delete(ctx, existing); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete networkpolicy: %w", err)
	}
	return nil
}

// mergeMaps overlays desired keys onto current so keys written by other controllers survive.
func mergeMaps(current, desired map[string]string) map[string]string {
	if current == nil {
		current = make(map[string]string, len(desired))
	}
	for k, v := range desired {
		current[k] = v
	}
	return current
}

// describe writes status from the reconcile outcome.
func describe(db *databasev1alpha1.SereneDB, sts *appsv1.StatefulSet, err error) {
	db.Status.ObservedGeneration = db.Generation
	db.Status.SecretName = resources.SecretName(db)
	db.Status.Host = resources.Host(db)
	db.Status.Port = resources.PostgresPort(db)

	var precondition *blocked
	if asBlocked(err, &precondition) {
		db.Status.ReadyReplicas = 0
		setCondition(db, databasev1alpha1.ConditionReady, metav1.ConditionFalse, precondition.reason, precondition.message)
		setCondition(db, databasev1alpha1.ConditionProgressing, metav1.ConditionFalse, precondition.reason, precondition.message)
		return
	}
	if err != nil {
		setCondition(db, databasev1alpha1.ConditionReady, metav1.ConditionFalse, reasonReconcileError, err.Error())
		setCondition(db, databasev1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonReconcileError, err.Error())
		return
	}

	db.Status.ReadyReplicas = sts.Status.ReadyReplicas
	if rolloutComplete(sts) {
		db.Status.Version = resources.ImageTag(db)
		setCondition(db, databasev1alpha1.ConditionReady, metav1.ConditionTrue, reasonPodReady, "database pod is serving")
		setCondition(db, databasev1alpha1.ConditionProgressing, metav1.ConditionFalse, reasonStable, "rollout complete")
		return
	}
	setCondition(db, databasev1alpha1.ConditionReady, metav1.ConditionFalse, reasonPodNotReady, "database pod is not ready")
	setCondition(db, databasev1alpha1.ConditionProgressing, metav1.ConditionTrue, reasonRollingOut, "statefulset rollout in progress")
}

func rolloutComplete(sts *appsv1.StatefulSet) bool {
	if sts.Status.ObservedGeneration != sts.Generation {
		return false
	}
	if sts.Status.ReadyReplicas != 1 || sts.Status.UpdatedReplicas != 1 {
		return false
	}
	return sts.Status.CurrentRevision == sts.Status.UpdateRevision
}

func setCondition(db *databasev1alpha1.SereneDB, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&db.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: db.Generation,
	})
}

func asBlocked(err error, target **blocked) bool {
	if err == nil {
		return false
	}
	b, ok := err.(*blocked)
	if !ok {
		return false
	}
	*target = b
	return true
}

// SetupWithManager registers the controller and its owned resource watches.
func (r *SereneDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.SereneDB{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("serenedb").
		Complete(r)
}
