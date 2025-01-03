package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
	discord "github.com/ushitora-anqou/vahkane/internal/discord"
)

const (
	annotKeyCommands            = "vahkane.anqou.net/commands"
	LabelKeyDiscordGuildID      = "vahkane.anqou.net/discord-guild-id"
	finalizerDiscordInteraction = "vahkane.anqou.net/discord-interaction"
)

var errRequeue = errors.New("requeue")

// DiscordInteractionReconciler reconciles a DiscordInteraction object
type DiscordInteractionReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	namespace     string
	discordClient discord.Client
}

func NewDiscordInteractionReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	namespace string,
	discordClient discord.Client,
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
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.doReconcile(ctx, &di); err != nil {
		if errors.Is(err, errRequeue) {
			return ctrl.Result{Requeue: true}, nil
		}
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

func (r *DiscordInteractionReconciler) doReconcile(
	ctx context.Context,
	di *vahkanev1.DiscordInteraction,
) error {
	logger := log.FromContext(ctx)

	if err := r.handleFinalizer(ctx, di); err != nil {
		return err
	}

	var guildIDUpdated, commandsUpdated bool

	guildID, ok := di.GetLabels()[LabelKeyDiscordGuildID]
	if !ok || di.Spec.GuildID != guildID {
		guildIDUpdated = true
	}

	// Check whether commands are updated or not
	var commandsConcatenated bytes.Buffer
	for _, command := range di.Spec.Commands {
		commandsConcatenated.WriteString(command)
		commandsConcatenated.WriteByte(0)
	}
	currentCommandsHashRaw := sha256.Sum224(commandsConcatenated.Bytes())
	currentCommandsHash := hex.EncodeToString(currentCommandsHashRaw[:])
	annotCommandsHash, ok := di.GetAnnotations()[annotKeyCommands]
	if !ok || currentCommandsHash != annotCommandsHash {
		commandsUpdated = true
	}

	if guildIDUpdated || commandsUpdated {
		labels := di.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[LabelKeyDiscordGuildID] = di.Spec.GuildID
		di.SetLabels(labels)

		annots := di.GetAnnotations()
		if annots == nil {
			annots = map[string]string{}
		}
		annots[annotKeyCommands] = currentCommandsHash
		di.SetAnnotations(annots)

		if err := r.Client.Update(ctx, di); err != nil {
			return fmt.Errorf("failed to update commands annot: %w", err)
		}
	}

	if commandsUpdated {
		logger.Info("register Discord guild commands", "guild_id", di.Spec.GuildID)
		if err := deleteAllGuildCommands(ctx, r.discordClient, di.Spec.GuildID); err != nil {
			return err
		}
		for _, command := range di.Spec.Commands {
			json, err := convertYAMLToJSON(command)
			if err != nil {
				return fmt.Errorf("failed to convert guild command YAML to JSON: %w", err)
			}
			if err := r.discordClient.RegisterGuildCommand(
				ctx,
				di.Spec.GuildID,
				json,
			); err != nil {
				return fmt.Errorf("failed to register Discord guild commands: %w", err)
			}
		}
	}

	return nil
}

func (r *DiscordInteractionReconciler) handleFinalizer(
	ctx context.Context,
	di *vahkanev1.DiscordInteraction,
) error {
	logger := log.FromContext(ctx)

	if !di.GetDeletionTimestamp().IsZero() {
		logger.Info("unregister Discord guild commands", "guild_id", di.Spec.GuildID)
		if err := deleteAllGuildCommands(ctx, r.discordClient, di.Spec.GuildID); err != nil {
			return fmt.Errorf("failed to delete guild commands: %w", err)
		}

		controllerutil.RemoveFinalizer(di, finalizerDiscordInteraction)
		if err := r.Client.Update(ctx, di); err != nil {
			return fmt.Errorf("failed to attach finalizer: %w", err)
		}
		return errRequeue
	}

	if !controllerutil.ContainsFinalizer(di, finalizerDiscordInteraction) {
		controllerutil.AddFinalizer(di, finalizerDiscordInteraction)
		if err := r.Client.Update(ctx, di); err != nil {
			return fmt.Errorf("failed to attach finalizer: %w", err)
		}
		return errRequeue
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

func deleteAllGuildCommands(ctx context.Context, client discord.Client, guildID string) error {
	commands, err := client.GetGuildCommands(ctx, guildID)
	if err != nil {
		return fmt.Errorf("failed to fetch guild commands: %w", err)
	}
	for _, command := range commands {
		id, ok1 := command["id"]
		idParsed, ok2 := id.(string)
		if !ok1 || !ok2 {
			return fmt.Errorf("failed to get command id")
		}
		if err := client.DeleteGuildCommand(ctx, guildID, idParsed); err != nil {
			return fmt.Errorf("failed to delete guild commands: %w", err)
		}
	}
	return nil
}
