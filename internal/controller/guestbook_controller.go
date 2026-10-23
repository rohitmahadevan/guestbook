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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	webappv1 "rohit.sandbox/guestbook/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Guestbook object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *GuestbookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the Guestbook resource. If it was deleted, there's nothing to do.
	var guestbook webappv1.Guestbook
	if err := r.Get(ctx, req.NamespacedName, &guestbook); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get Guestbook: %w", err)
		}

		log.Info("Guestbook not found, probably deleted", "name", req.Name)
		return ctrl.Result{}, nil
	}
	log.Info("Reconciling Guestbook", "spec", guestbook.Spec)

	// Get the Deployment object. If it doesn't exist yet, create it.
	var deployment appsv1.Deployment
	if err := r.Get(ctx, req.NamespacedName, &deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get Deployment: %w", err)
		}

		log.Info("Deployment not found, creating a new one", "name", req.Name)
		newDeployment := createDeployment(&guestbook)
		if err := r.Create(ctx, newDeployment); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create Deployment: %w", err)
		}
		log.Info("Created Deployment for Guestbook", "deployment", newDeployment.Name)
	}

	return ctrl.Result{}, nil
}

func createDeployment(gb *webappv1.Guestbook) *appsv1.Deployment {
	label := map[string]string{"app": gb.Name}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gb.Name,
			Namespace: gb.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: gb.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: label,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "guestbook",
						Image: gb.Spec.Image,
					}},
				},
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *GuestbookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.Guestbook{}).
		Named("guestbook").
		Complete(r)
}
