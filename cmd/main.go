package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/go-logr/logr"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	vahkaneanqounetv1 "github.com/ushitora-anqou/vahkane/api/v1"
	"github.com/ushitora-anqou/vahkane/internal/controller"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(vahkaneanqounetv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	discordApplicationPublicKey, ok := os.LookupEnv("DISCORD_APPLICATION_PUBLIC_KEY")
	if !ok {
		msg := "set DISCORD_APPLICATION_PUBLIC_KEY"
		setupLog.Error(errors.New(msg), msg)
		os.Exit(1)
	}
	discordApplicationPublicKeyParsed, err := hex.DecodeString(discordApplicationPublicKey)
	if err != nil {
		setupLog.Error(err, "failed to parse DISCORD_APPLICATION_PUBLIC_KEY")
		os.Exit(1)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization

		// TODO(user): If CertDir, CertName, and KeyName are not specified, controller-runtime will automatically
		// generate self-signed certificates for the metrics server. While convenient for development and testing,
		// this setup is not recommended for production.
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "99e56057.vahkane.anqou.net",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.DiscordInteractionReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DiscordInteraction")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	err = mgr.Add(
		NewDiscordWebhookServerRunner(
			mgr.GetClient(),
			mgr.GetLogger().WithName("DiscordWebhookServerRunner"),
			discordApplicationPublicKeyParsed,
		),
	)
	if err != nil {
		setupLog.Error(err, "unable to add DiscordWebhookServerRunner")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

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
