package runner

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DiscordWebhookServerRunner struct {
	k8sClient client.Client
	logger    logr.Logger
	publicKey ed25519.PublicKey
}

func NewDiscordWebhookServerRunner(
	k8sClient client.Client,
	logger logr.Logger,
	publicKey ed25519.PublicKey,
) *DiscordWebhookServerRunner {
	return &DiscordWebhookServerRunner{
		k8sClient: k8sClient,
		logger:    logger,
		publicKey: publicKey,
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

func (r *DiscordWebhookServerRunner) handleApplicationCommand(
	w http.ResponseWriter,
	body []byte,
) error {
	var req struct {
		Data      interface{} `json:"data"`
		GuildID   string      `json:"guild_id"`
		ChannelID string      `json:"channel_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}

	var resp struct {
		Type int `json:"type"`
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	resp.Type = 4
	resp.Data.Content = "foobar"

	if err := respondJSON(w, &resp); err != nil {
		return err
	}

	return nil
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

	addr := "0.0.0.0:38000"
	srv := http.Server{
		Addr:           addr,
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
		r.logger.Info("starting discord webhook server", "addr", addr)
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
