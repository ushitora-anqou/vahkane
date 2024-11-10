package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
	discord "github.com/ushitora-anqou/vahkane/internal/discord"
)

const (
	annotKeyCommands       = "vahkane.anqou.net/commands"
	annotKeyActions        = "vahkane.anqou.net/actions"
	labelKeyDiscordGuildID = "vahkane.anqou.net/discord-guild-id"
)

// DiscordInteractionReconciler reconciles a DiscordInteraction object
type DiscordInteractionReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	namespace     string
	discordClient *discord.Client
}

func NewDiscordInteractionReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	namespace string,
	discordClient *discord.Client,
) *DiscordInteractionReconciler {
	return &DiscordInteractionReconciler{
		Client:        client,
		Scheme:        scheme,
		namespace:     namespace,
		discordClient: discordClient,
	}
}

// +kubebuilder:rbac:groups=vahkane.anqou.net,resources=discordinteractions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vahkane.anqou.net,resources=discordinteractions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vahkane.anqou.net,resources=discordinteractions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DiscordInteraction object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *DiscordInteractionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var di vahkanev1.DiscordInteraction
	if err := r.Client.Get(ctx, req.NamespacedName, &di); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileDiscordInteraction(ctx, &di); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiscordInteractionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vahkanev1.DiscordInteraction{}).
		Named("discordinteraction").
		Complete(r)
}

func (r *DiscordInteractionReconciler) reconcileDiscordInteraction(
	ctx context.Context,
	di *vahkanev1.DiscordInteraction,
) error {
	logger := log.FromContext(ctx)

	var guildIDUpdated, commandsUpdated bool

	guildID, ok := di.GetLabels()[labelKeyDiscordGuildID]
	if !ok || di.Spec.GuildID != guildID {
		guildIDUpdated = true
	}

	currentCommandsJSON, err := convertYAMLToJSON(di.Spec.Commands)
	if err != nil {
		return err
	}
	annotCommands, ok := di.GetAnnotations()[annotKeyCommands]
	if !ok || currentCommandsJSON != annotCommands {
		commandsUpdated = true
	}

	if guildIDUpdated || commandsUpdated {
		labels := di.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[labelKeyDiscordGuildID] = di.Spec.GuildID
		di.SetLabels(labels)

		annots := di.GetAnnotations()
		if annots == nil {
			annots = map[string]string{}
		}
		annots[annotKeyCommands] = string(currentCommandsJSON)
		di.SetAnnotations(annots)

		if err := r.Client.Update(ctx, di); err != nil {
			return fmt.Errorf("failed to update commands annot: %w", err)
		}
	}

	if commandsUpdated {
		logger.Info("register Discord guild commands", "guild_id", di.Spec.GuildID)
		if err := r.discordClient.RegisterGuildCommands(
			ctx,
			di.Spec.GuildID,
			currentCommandsJSON,
		); err != nil {
			return fmt.Errorf("failed to register Discord guild commands: %w", err)
		}
	}

	return nil
}

func convertYAMLToJSON(src string) (string, error) {
	var v interface{}
	if err := yaml.Unmarshal([]byte(src), &v); err != nil {
		return "", err
	}
	json, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(json), nil
}
