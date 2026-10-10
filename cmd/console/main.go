package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/demo"
	"github.com/kubebao/openbao-operator-console/internal/httpapi"
	"github.com/kubebao/openbao-operator-console/internal/renewal"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
	"github.com/kubebao/openbao-operator-console/internal/view"
)

// options holds the command-line flags.
type options struct {
	mode, address, configPath   string
	readerName, readerNamespace string
	tlsCertificate, tlsKey      string
	printRBAC                   string
	check, recoverIdentity      bool
}

func parseOptions() (options, error) {
	var o options
	flag.StringVar(&o.mode, "mode", "demo", "demo or live")
	flag.StringVar(&o.address, "listen", "127.0.0.1:8090", "HTTP listen address")
	flag.StringVar(&o.configPath, "config", "", "optional first-start enrollment YAML file")
	flag.BoolVar(
		&o.recoverIdentity,
		"recover-identity",
		false,
		"replace stored identity and enroll administrators from bootstrap environment; exit without serving",
	)
	flag.BoolVar(&o.check, "check", false, "check enrolled Kubernetes reads and print approved projections; no database or OIDC required")
	flag.StringVar(&o.printRBAC, "print-rbac", "", "print reader RBAC for this connection ID; does not apply manifests")
	flag.StringVar(&o.readerName, "reader-name", "console-reader", "local reader ServiceAccount name for RBAC manifests")
	flag.StringVar(&o.readerNamespace, "reader-namespace", "console-system", "local reader ServiceAccount namespace for RBAC manifests")
	flag.StringVar(&o.tlsCertificate, "tls-cert", "", "optional HTTPS server certificate PEM file")
	flag.StringVar(&o.tlsKey, "tls-key", "", "optional HTTPS server private key PEM file")
	flag.Parse()
	if (o.tlsCertificate == "") != (o.tlsKey == "") {
		return o, errors.New("tls-cert and tls-key must be supplied together")
	}
	return o, nil
}

func run() error {
	o, err := parseOptions()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	switch {
	case o.check || o.printRBAC != "":
		return runCheck(ctx, o)
	case o.mode == "demo":
		server, err := demoServer(o)
		if err != nil {
			return err
		}
		return serve(ctx, o, server.Handler(), logger)
	case o.mode == "live":
		return runLive(ctx, o, logger)
	default:
		return errors.New("mode must be demo or live")
	}
}

// runCheck reads the enrollment file without a database or identity provider.
func runCheck(ctx context.Context, o options) error {
	enrollment, err := config.Load(o.configPath)
	if err != nil {
		return err
	}
	if o.printRBAC == "" {
		return checkConnections(ctx, enrollment)
	}
	for _, connection := range enrollment.Connections {
		if connection.ID == o.printRBAC {
			return config.WriteRBAC(os.Stdout, connection, o.readerName, o.readerNamespace)
		}
	}
	return errors.New("connection is not enrolled")
}

// demoServer serves fictional fixtures on a loopback address only.
func demoServer(o options) (*httpapi.Server, error) {
	if o.recoverIdentity {
		return nil, errors.New("recover-identity requires live mode")
	}
	host, _, err := net.SplitHostPort(o.address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("demo mode must listen on a loopback IP")
	}
	fixtures := demo.New(time.Now().UTC())
	return &httpapi.Server{
		Fleet:           fixtures,
		Auth:            auth.NewDemo(fixtures.Viewer()),
		Demo:            true,
		ReaderName:      o.readerName,
		ReaderNamespace: o.readerNamespace,
	}, nil
}

// runLive serves live mode. Collection and renewal stop before the database closes.
func runLive(ctx context.Context, o options, logger *slog.Logger) error {
	dsn := os.Getenv("CONSOLE_DATABASE_URL")
	if dsn == "" {
		return errors.New("CONSOLE_DATABASE_URL is required in live mode")
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pg, err := store.Open(startup, dsn, 7*24*time.Hour)
	if err != nil {
		return fmt.Errorf("database startup failed: %w", err)
	}
	defer pg.Close()
	configuration, enrollment, err := openSettings(startup, pg, o)
	if err != nil {
		return err
	}
	if o.recoverIdentity {
		return recoverConfiguration(startup, configuration)
	}
	server, err := liveServer(startup, o, pg, configuration, enrollment)
	if err != nil {
		return err
	}
	workers, stopWorkers := context.WithCancel(ctx)
	var running sync.WaitGroup
	defer func() {
		stopWorkers()
		running.Wait()
	}()
	running.Go(func() { collector.RunManaged(workers, configuration, pg, logger) })
	running.Go(func() {
		(renewal.Manager{Repository: configuration, Issuer: renewal.KubernetesIssuer{}, Logger: logger}).Run(workers)
	})
	return serve(ctx, o, server.Handler(), logger)
}

// openSettings opens stored configuration. An empty database is seeded from the
// enrollment file and bootstrap environment; without an identity it waits for browser setup.
func openSettings(ctx context.Context, pg *store.Postgres, o options) (*settings.Service, config.Config, error) {
	key, err := os.ReadFile(os.Getenv("CONSOLE_CONFIG_KEY_FILE"))
	if err != nil {
		return nil, config.Config{}, errors.New("CONSOLE_CONFIG_KEY_FILE must reference a readable 32-byte key")
	}
	var enrollment config.Config
	_, loadErr := pg.LoadSettings(ctx)
	switch {
	case errors.Is(loadErr, settings.ErrMissing):
		if o.recoverIdentity {
			return nil, enrollment, errors.New("identity recovery requires stored configuration")
		}
		if enrollment, err = bootstrapSeed(o.configPath); err != nil {
			return nil, enrollment, err
		}
	case loadErr != nil:
		return nil, enrollment, errors.New("read stored configuration")
	}
	var seed *config.Config
	if enrollment.Identity != nil {
		seed = &enrollment
	}
	configuration, err := settings.Open(ctx, pg, key, seed)
	if err != nil {
		return nil, enrollment, fmt.Errorf("initialize console settings: %w", err)
	}
	return configuration, enrollment, nil
}

func liveServer(
	ctx context.Context,
	o options,
	pg *store.Postgres,
	configuration *settings.Service,
	enrollment config.Config,
) (*httpapi.Server, error) {
	callback := os.Getenv("CONSOLE_OIDC_REDIRECT_URL")
	parsed, err := url.Parse(callback)
	if err != nil {
		return nil, err
	}
	authentication, setup, err := liveAuthentication(ctx, configuration, callback, enrollment)
	if err != nil {
		return nil, err
	}
	return &httpapi.Server{
		Fleet:           view.Live{Observations: pg, Enrollment: configuration},
		Auth:            authentication,
		Settings:        configuration,
		Setup:           setup,
		Origin:          parsed.Scheme + "://" + parsed.Host,
		ReaderName:      o.readerName,
		ReaderNamespace: o.readerNamespace,
	}, nil
}

// liveAuthentication uses the stored identity provider, or opens browser setup until a
// verified administrator configures one.
func liveAuthentication(
	ctx context.Context,
	configuration *settings.Service,
	callback string,
	enrollment config.Config,
) (*auth.Authenticator, *httpapi.Setup, error) {
	if configuration.Initialized() {
		current, _ := configuration.Current()
		if current.Identity == nil {
			return nil, nil, errors.New("identity provider is not configured")
		}
		provider, err := auth.Discover(ctx, *current.Identity, callback)
		if err != nil {
			return nil, nil, fmt.Errorf("authentication startup failed: %w", err)
		}
		return auth.New(provider, configuration.Resolve), nil, nil
	}
	token, err := readSetupToken(os.Getenv("CONSOLE_SETUP_TOKEN_FILE"))
	if err != nil {
		return nil, nil, err
	}
	authentication := auth.NewPending(configuration.Resolve)
	setup, err := httpapi.NewSetup(configuration, authentication, token, callback, enrollment)
	clear(token)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize browser setup: %w", err)
	}
	return authentication, setup, nil
}

// serve listens until ctx ends, then shuts down within ten seconds.
func serve(ctx context.Context, o options, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Addr:              o.address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		// The serving context has ended; shutdown needs its own deadline.
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("Console started", "mode", o.mode, "listen", o.address)
	var err error
	if o.tlsCertificate != "" {
		err = server.ListenAndServeTLS(o.tlsCertificate, o.tlsKey)
	} else {
		err = server.ListenAndServe()
	}
	cancel()
	<-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
