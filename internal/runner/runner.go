package runner

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
	"github.com/ushitora-anqou/vahkane/internal/controller"
	"github.com/ushitora-anqou/vahkane/internal/discord"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	annotKeyDiscordInteraction = "vahkane.anqou.net/discord-interaction"
	annotKeyAction             = "vahkane.anqou.net/action"
)

type DiscordWebhookServerRunner struct {
	k8sClient             client.Client
	discordClient         *discord.Client
	logger                logr.Logger
	publicKey             ed25519.PublicKey
	listenAddr, namespace string
}

func NewDiscordWebhookServerRunner(
	k8sClient client.Client,
	discordClient *discord.Client,
	logger logr.Logger,
	publicKey ed25519.PublicKey,
	listenAddr, namespace string,
) *DiscordWebhookServerRunner {
	return &DiscordWebhookServerRunner{
		k8sClient:     k8sClient,
		discordClient: discordClient,
		logger:        logger,
		publicKey:     publicKey,
		listenAddr:    listenAddr,
		namespace:     namespace,
	}
}

func (r *DiscordWebhookServerRunner) verifyRequest(header http.Header, body []byte) (bool, error) {
	signatureEncoded := header.Get("X-Signature-Ed25519")
	timestamp := header.Get("X-Signature-Timestamp")

	message := make([]byte, len(timestamp)+len(body))
	copy(message[0:], timestamp)
	copy(message[len(timestamp):], body)

	signature, err := hex.DecodeString(signatureEncoded)
	if err != nil {
		return false, err
	}

	return ed25519.Verify(r.publicKey, message, signature), nil
}

type requestApplicationCommand struct {
	Data      interface{} `json:"data"`
	GuildID   string      `json:"guild_id"`
	ChannelID string      `json:"channel_id"`
	Token     string      `json:"token"`
	ID        string      `json:"id"`
}

func (r *DiscordWebhookServerRunner) handleApplicationCommand(
	w http.ResponseWriter,
	body []byte,
) error {
	var req requestApplicationCommand
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		msg := ":ok: successfully queued your job"
		if err := queueJobByRequest(ctx, r.k8sClient, r.namespace, &req); err != nil {
			r.logger.Error(err, "failed to queue job: "+string(body))
			msg = ":x: failed to queue your job"
		}
		if err := r.discordClient.SendFollowupMessage(ctx, req.Token, msg); err != nil {
			r.logger.Error(err, "failed to send followup message", "message", msg)
		}
	}()

	return respondDeferred(w)
}

func (r *DiscordWebhookServerRunner) handleWebhook(
	w http.ResponseWriter,
	req *http.Request,
) error {
	// cf. https://discord.com/developers/docs/interactions/overview

	defer func() {
		_ = req.Body.Close()
	}()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	r.logger.Info("discord webhook request: " + string(body))

	verified, err := r.verifyRequest(req.Header, body)
	if err != nil {
		return err
	}
	if !verified {
		w.WriteHeader(http.StatusUnauthorized)
		return nil
	}

	var root map[string]interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return err
	}
	requestType, ok1 := root["type"]
	requestTypeParsed, ok2 := requestType.(float64)
	if !ok1 || !ok2 {
		return errors.New("type not found in the request")
	}

	switch int(requestTypeParsed) {
	case 1: // PING
		if err := respondJSON(w, map[string]int{"type": 1 /* PONG */}); err != nil {
			return err
		}

	case 2: // APPLICATION_COMMAND
		if err := r.handleApplicationCommand(w, body); err != nil {
			return err
		}

	default:
		r.logger.Info("unexpected request", "body", body)
		w.WriteHeader(http.StatusNoContent)
	}

	return nil
}

func (r *DiscordWebhookServerRunner) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, req *http.Request) {
		if err := r.handleWebhook(w, req); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			r.logger.Error(err, "failed to handle webhook request")
			return
		}
	})

	srv := http.Server{
		Addr:           r.listenAddr,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		Handler:        mux,
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()
		r.logger.Info("starting discord webhook server", "addr", r.listenAddr)
		if err := srv.ListenAndServe(); err != nil {
			r.logger.Error(err, "failed to start http server")
		}
	}()

	<-ctx.Done()

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		r.logger.Error(err, "failed to shutdown http server")
	}

	return nil
}

func (r DiscordWebhookServerRunner) NeedLeaderElection() bool {
	return true
}

func fetchDiscordInteractionByGuildID(
	ctx context.Context,
	k8sClient client.Client,
	guildID string,
) (*vahkanev1.DiscordInteraction, error) {
	var diList vahkanev1.DiscordInteractionList
	if err := k8sClient.List(
		ctx,
		&diList,
		&client.ListOptions{
			LabelSelector: labels.SelectorFromSet(
				map[string]string{controller.LabelKeyDiscordGuildID: guildID},
			),
		},
	); err != nil {
		return nil, err
	}
	if len(diList.Items) != 1 {
		return nil, fmt.Errorf("unexpected number of DiscordInteractions: %d", len(diList.Items))
	}
	return &diList.Items[0], nil
}

func respondJSON(w http.ResponseWriter, v interface{}) error {
	json, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Add("Content-Type", "application/json")
	if _, err := w.Write(json); err != nil {
		return err
	}
	return nil
}

func respondDeferred(w http.ResponseWriter) error {
	var resp struct {
		Type int `json:"type"`
	}
	resp.Type = 5
	return respondJSON(w, &resp)
}

func respondAlreadyRunning(w http.ResponseWriter) error {
	var resp struct {
		Type int `json:"type"`
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	resp.Type = 4
	resp.Data.Content = "already running"
	return respondJSON(w, &resp)
}

func doesJobAlreadyExist(
	ctx context.Context,
	k8sClient client.Client,
	action *vahkanev1.DiscordInteractionAction,
	diName, namespace string,
) (bool, error) {
	var job batchv1.Job
	if err := k8sClient.Get(
		ctx,
		types.NamespacedName{Name: makeJobName(diName, action), Namespace: namespace},
		&job,
	); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get Job: %w", err)
	}
	return true, nil
}

func createJobForAction(
	ctx context.Context,
	k8sClient client.Client,
	action *vahkanev1.DiscordInteractionAction,
	diName, namespace, interactionToken string,
) error {
	var job batchv1.Job

	job.Spec = action.ActionInline.JobTemplate.Spec
	job.ObjectMeta = action.ActionInline.JobTemplate.ObjectMeta
	job.ObjectMeta.Namespace = namespace
	job.ObjectMeta.Name = makeJobName(diName, action)
	job.Spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever

	labels := job.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[controller.LabelKeyJob] = "true"
	job.SetLabels(labels)

	annots := job.GetAnnotations()
	if annots == nil {
		annots = map[string]string{}
	}
	annots[annotKeyDiscordInteraction] = diName
	annots[annotKeyAction] = action.Name
	annots[controller.AnnotKeyDiscordInteractionToken] = interactionToken
	job.SetAnnotations(annots)

	if err := k8sClient.Create(ctx, &job); err != nil {
		return fmt.Errorf(
			"failed to create job: %s: %s: %w",
			job.ObjectMeta.Name,
			job.ObjectMeta.Namespace,
			err,
		)
	}
	return nil
}

func queueJobByRequest(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	req *requestApplicationCommand,
) error {
	di, err := fetchDiscordInteractionByGuildID(ctx, k8sClient, req.GuildID)
	if err != nil {
		return fmt.Errorf("failed to fetch DiscordInteraction by guild id: %w", err)
	}

	action, err := matchActions(di.Spec.Actions, req.Data)
	if err != nil {
		return fmt.Errorf("failed to match actions: %w", err)
	}

	exist, err := doesJobAlreadyExist(ctx, k8sClient, action, di.Name, namespace)
	if err != nil {
		return fmt.Errorf("failed to check if Job already exists: %w", err)
	}
	if exist {
		return errors.New("already running")
	}

	if err := createJobForAction(ctx, k8sClient, action, di.Name, namespace, req.Token); err != nil {
		return fmt.Errorf("failed to create Job for Action: %w", err)
	}

	return nil
}
