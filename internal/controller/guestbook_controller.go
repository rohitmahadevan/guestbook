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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	webappv1 "rohit.sandbox/guestbook/api/v1"
)

const (
	containerName = "frontend"

	// conditionAvailable reports whether the guestbook frontend is serving traffic.
	conditionAvailable = "Available"
)

// GuestbookReconciler reconciles a Guestbook object
type GuestbookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=webapp.rohit.sandbox,resources=guestbooks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=webapp.rohit.sandbox,resources=guestbooks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=webapp.rohit.sandbox,resources=guestbooks/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives the cluster toward the state declared in a Guestbook:
// a frontend Deployment and a Service in front of it, both owned by the Guestbook.
// Status.Conditions reports whether the frontend is available.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *GuestbookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var gb webappv1.Guestbook
	if err := r.Get(ctx, req.NamespacedName, &gb); err != nil {
		// Deleted Guestbooks need no work: owned children are garbage-collected.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var childName = gb.Name + "-" + containerName
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: childName, Namespace: gb.Namespace}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		mutateDeployment(&gb, dep)
		return controllerutil.SetControllerReference(&gb, dep, r.Scheme)
	})
	if err != nil {
		log.Error(err, "Failed to reconcile Deployment", "name", dep.Name)
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		log.Info("Reconciled Deployment", "name", dep.Name, "operation", op)
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: childName, Namespace: gb.Namespace}}
	op, err = controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		mutateService(&gb, svc)
		return controllerutil.SetControllerReference(&gb, svc, r.Scheme)
	})
	if err != nil {
		log.Error(err, "Failed to reconcile Service", "name", svc.Name)
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		log.Info("Reconciled Service", "name", svc.Name, "operation", op)
	}

	cond := availableCondition(&gb, dep)
	cond.ObservedGeneration = gb.Generation
	if meta.SetStatusCondition(&gb.Status.Conditions, cond) {
		if err := r.Status().Update(ctx, &gb); err != nil {
			// Conflicts are expected when the Guestbook changed mid-reconcile; the retry re-reads it.
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func labelsFor(gb *webappv1.Guestbook) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "guestbook",
		"app.kubernetes.io/instance":   gb.Name,
		"app.kubernetes.io/managed-by": "guestbook-controller",
	}
}

// mutateDeployment sets only the fields the controller owns, leaving
// API-server defaults intact so CreateOrUpdate does not issue no-op updates.
func mutateDeployment(gb *webappv1.Guestbook, dep *appsv1.Deployment) {
	labels := labelsFor(gb)

	dep.Spec.Replicas = gb.Spec.Replicas
	dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
	dep.Spec.Template.Labels = labels

	idx := -1
	for i, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == containerName {
			idx = i
			break
		}
	}
	if idx == -1 {
		dep.Spec.Template.Spec.Containers = append(dep.Spec.Template.Spec.Containers, corev1.Container{Name: containerName})
		idx = len(dep.Spec.Template.Spec.Containers) - 1
	}

	c := &dep.Spec.Template.Spec.Containers[idx]
	c.Image = gb.Spec.Image
	c.Ports = []corev1.ContainerPort{{Name: "http", ContainerPort: gb.Spec.Port, Protocol: corev1.ProtocolTCP}}
}

func mutateService(gb *webappv1.Guestbook, svc *corev1.Service) {
	svc.Spec.Selector = labelsFor(gb)
	svc.Spec.Ports = []corev1.ServicePort{{
		Name:       "http",
		Port:       gb.Spec.Port,
		TargetPort: intstr.FromString("http"),
		Protocol:   corev1.ProtocolTCP,
	}}
}

// availableCondition derives the Guestbook's Available condition from the
// observed state of its Deployment.
func availableCondition(gb *webappv1.Guestbook, dep *appsv1.Deployment) metav1.Condition {
	desired := int32(1)
	if gb.Spec.Replicas != nil {
		desired = *gb.Spec.Replicas
	}

	cond := metav1.Condition{Type: conditionAvailable}
	switch {
	case dep.Status.ObservedGeneration < dep.Generation:
		// The Deployment's status still describes an older spec.
		cond.Status = metav1.ConditionFalse
		cond.Reason = "RolloutInProgress"
		cond.Message = "Waiting for the Deployment controller to observe the latest spec"
	case desired == 0:
		cond.Status = metav1.ConditionFalse
		cond.Reason = "ScaledToZero"
		cond.Message = "Guestbook is scaled to 0 replicas"
	case dep.Status.AvailableReplicas >= desired:
		cond.Status = metav1.ConditionTrue
		cond.Reason = "DeploymentAvailable"
		cond.Message = fmt.Sprintf("%d/%d replicas available", dep.Status.AvailableReplicas, desired)
	default:
		cond.Status = metav1.ConditionFalse
		cond.Reason = "ScalingUp"
		cond.Message = fmt.Sprintf("%d/%d replicas available", dep.Status.AvailableReplicas, desired)
	}
	return cond
}

// SetupWithManager sets up the controller with the Manager.
func (r *GuestbookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.Guestbook{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("guestbook").
		Complete(r)
}
