package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	webappv1 "rohit.sandbox/guestbook/api/v1"
)

func TestAvailableCondition(t *testing.T) {
	tests := []struct {
		name       string
		replicas   *int32
		generation int64
		status     appsv1.DeploymentStatus
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{"stale status", ptr.To[int32](2), 2, appsv1.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: 2}, metav1.ConditionFalse, "RolloutInProgress"},
		{"scaled to zero", ptr.To[int32](0), 1, appsv1.DeploymentStatus{ObservedGeneration: 1}, metav1.ConditionFalse, "ScaledToZero"},
		{"all available", ptr.To[int32](3), 1, appsv1.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: 3}, metav1.ConditionTrue, "DeploymentAvailable"},
		{"partially available", ptr.To[int32](3), 1, appsv1.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: 1}, metav1.ConditionFalse, "ScalingUp"},
		{"nil replicas defaults to 1", nil, 1, appsv1.DeploymentStatus{ObservedGeneration: 1, AvailableReplicas: 1}, metav1.ConditionTrue, "DeploymentAvailable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gb := &webappv1.Guestbook{Spec: webappv1.GuestbookSpec{Replicas: tt.replicas}}
			dep := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: tt.generation},
				Status:     tt.status,
			}

			got := availableCondition(gb, dep)
			if got.Type != conditionAvailable || got.Status != tt.wantStatus || got.Reason != tt.wantReason {
				t.Errorf("got {%s %s %s}, want {%s %s %s}",
					got.Type, got.Status, got.Reason, conditionAvailable, tt.wantStatus, tt.wantReason)
			}
		})
	}
}
