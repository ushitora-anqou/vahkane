package controller

import (
	"context"
	"fmt"

	discord "github.com/ushitora-anqou/vahkane/internal/discord"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	LabelKeyJob                     = "vahkane.anqou.net/job"
	AnnotKeyDiscordInteractionToken = "vahkane.anqou.net/discord-interaction-token"
)

type JobReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	namespace     string
	discordClient *discord.Client
}

func NewJobReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	namespace string,
	discordClient *discord.Client,
) *JobReconciler {
	return &JobReconciler{
		Client:        client,
		Scheme:        scheme,
		namespace:     namespace,
		discordClient: discordClient,
	}
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var job batchv1.Job
	if err := r.Client.Get(ctx, req.NamespacedName, &job); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.reconcileJob(ctx, &job); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *JobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		Named("job").
		Complete(r)
}

func (r *JobReconciler) reconcileJob(ctx context.Context, job *batchv1.Job) error {
	logger := log.FromContext(ctx)

	if _, ok := job.GetLabels()[LabelKeyJob]; !ok {
		return nil
	}

	if !IsJobStatusConditionTrue(job.Status.Conditions, batchv1.JobComplete) &&
		!IsJobStatusConditionTrue(job.Status.Conditions, batchv1.JobFailed) {
		return nil
	}

	msg := "completed"
	if IsJobStatusConditionTrue(job.Status.Conditions, batchv1.JobFailed) {
		msg = "failed"
	}

	discordInteractionToken := job.GetAnnotations()[AnnotKeyDiscordInteractionToken]
	if err := r.discordClient.SendFollowupMessage(
		ctx,
		discordInteractionToken,
		msg,
	); err != nil {
		logger.Error(err, "failed to send followup messages")
	}

	propagationPolicy := metav1.DeletePropagationBackground
	if err := r.Client.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Job: %w", err)
	}

	return nil
}

func IsJobStatusConditionTrue(
	conditions []batchv1.JobCondition,
	condType batchv1.JobConditionType,
) bool {
	for _, cond := range conditions {
		if cond.Type == condType && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
