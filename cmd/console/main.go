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
	"github.com/kubebao/openbao-operator-console/internal/buildinfo"
	"github.com/kubebao/openbao-operator-console/internal/clientip"
	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/demo"
	"github.com/kubebao/openbao-operator-console/internal/httpapi"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
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
	demoAllowExternal           bool
	historyRetention            time.Duration
	version                     bool
	recoverAccessRules          string
	trustedProxies              string
	recoverLocal                string
	clientIP                    clientip.Resolver
}

func parseOptions() (options, error) {
	var o options
	retention, err := historyRetentionDefault(os.Getenv("CONSOLE_HISTORY_RETENTION"))
	if err != nil {
		return o, err
	}
	flag.StringVar(&o.mode, "mode", "demo", "demo or live")
	flag.StringVar(&o.address, "listen", "127.0.0.1:8090", "HTTP listen address")
	flag.BoolVar(
		&o.demoAllowExternal,
		"demo-allow-external",
		false,
		"let demo mode listen on a non-loopback address, such as inside a container; demo mode has no login",
	)
	flag.StringVar(&o.configPath, "config", "", "optional first-start enrollment YAML file")
	flag.BoolVar(
		&o.recoverIdentity,
		"recover-identity",
		false,
		"replace stored identity and enroll administrators from bootstrap environment; exit without serving",
	)
	flag.StringVar(
		&o.recoverLocal,
		"recover-local-administrator",
		"",
		"create or reset this local administrator, enable local sign-in, print a one-time enrollment link, and exit",
	)
	flag.StringVar(
		&o.recoverAccessRules,
		"recover-access-rules",
		"",
		"carryOver or remove existing user and group access when the recovery issuer differs from the stored issuer",
	)
	flag.BoolVar(&o.check, "check", false, "check enrolled Kubernetes reads and print approved projections; no database or OIDC required")
	flag.StringVar(&o.printRBAC, "print-rbac", "", "print reader RBAC for this connection ID; does not apply manifests")
	flag.StringVar(&o.readerName, "reader-name", "console-reader", "local reader ServiceAccount name for RBAC manifests")
	flag.StringVar(&o.readerNamespace, "reader-namespace", "console-system", "local reader ServiceAccount namespace for RBAC manifests")
	flag.StringVar(&o.tlsCertificate, "tls-cert", "", "optional HTTPS server certificate PEM file")
	flag.StringVar(&o.tlsKey, "tls-key", "", "optional HTTPS server private key PEM file")
	flag.DurationVar(
		&o.historyRetention,
		"history-retention",
		retention,
		"how long observation history and activity are kept, from 1h to 8760h; default from CONSOLE_HISTORY_RETENTION or 168h",
	)
	flag.StringVar(
		&o.trustedProxies,
		"trusted-proxies",
		environment("CONSOLE_TRUSTED_PROXIES"),
		"comma-separated proxy addresses or CIDR prefixes whose X-Forwarded-For is used for rate limits and logs",
	)
	flag.BoolVar(&o.version, "version", false, "print the version and commit, then exit")
	flag.Parse()
	if (o.tlsCertificate == "") != (o.tlsKey == "") {
		return o, errors.New("tls-cert and tls-key must be supplied together")
	}
	if o.demoAllowExternal && o.mode != "demo" {
		return o, errors.New("demo-allow-external requires demo mode")
	}
	if o.recoverLocal != "" && (o.recoverIdentity || o.mode != "live") {
		return o, errors.New("recover-local-administrator requires live mode and excludes recover-identity")
	}
	if o.recoverAccessRules != "" && !o.recoverIdentity {
		return o, errors.New("recover-access-rules requires recover-identity")
	}
	if rules := config.AccessRules(o.recoverAccessRules); !rules.Valid() {
		return o, errors.New("recover-access-rules must be carryOver or remove")
	}
	if o.clientIP, err = clientip.Parse(strings.Split(o.trustedProxies, ",")); err != nil {
		return o, err
	}
	return o, validHistoryRetention(o.historyRetention)
}

func run() error {
	o, err := parseOptions()
	if err != nil {
		return err
	}
	if o.version {
		version, commit := buildinfo.Current()
		if commit == "" {
			commit = "unknown"
		}
		_, err := fmt.Fprintf(os.Stdout, "openbao-operator-console %s (commit %s)\n", version, commit)
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

// demoServer serves fictional fixtures without a login. It listens on a loopback address
// unless the operator explicitly allows another address, such as in a container whose
// published port stays on the host's loopback interface.
func demoServer(o options) (*httpapi.Server, error) {
	if o.recoverIdentity {
		return nil, errors.New("recover-identity requires live mode")
	}
	host, _, err := net.SplitHostPort(o.address)
	if err != nil {
		return nil, errors.New("listen must be a host and port")
	}
	if !o.demoAllowExternal && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return nil, errors.New("demo mode must listen on a loopback IP; set --demo-allow-external to override")
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
	dsn := environment("CONSOLE_DATABASE_URL")
	if dsn == "" {
		return errors.New("CONSOLE_DATABASE_URL is required in live mode")
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pg, err := store.Open(startup, dsn, o.historyRetention)
	if err != nil {
		return fmt.Errorf("database startup failed: %w", err)
	}
	defer pg.Close()
	key, err := readConfigKey(os.Getenv("CONSOLE_CONFIG_KEY_FILE"))
	if err != nil {
		return err
	}
	configuration, enrollment, err := openSettings(startup, pg, key, o)
	if err != nil {
		return err
	}
	if o.recoverIdentity {
		return recoverConfiguration(startup, configuration, config.AccessRules(o.recoverAccessRules))
	}
	local, err := localAccounts(startup, pg, key, configuration)
	if err != nil {
		return err
	}
	if o.recoverLocal != "" {
		return recoverLocalAdministrator(startup, configuration, local, o.recoverLocal, os.Stdout)
	}
	server, err := liveServer(startup, o, pg, configuration, enrollment, local, logger)
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
	running.Go(func() { pg.RunPruning(workers, logger) })
	running.Go(func() {
		(renewal.Manager{Repository: configuration, Issuer: renewal.KubernetesIssuer{}, Logger: logger}).Run(workers)
	})
	return serve(ctx, o, server.Handler(), logger)
}

// openSettings opens stored configuration. An empty database is seeded from the
// enrollment file and bootstrap environment; without an identity it waits for browser setup.
func openSettings(ctx context.Context, pg *store.Postgres, key []byte, o options) (*settings.Service, config.Config, error) {
	var err error
	var enrollment config.Config
	_, loadErr := pg.LoadSettings(ctx)
	switch {
	case errors.Is(loadErr, settings.ErrMissing):
		if o.recoverIdentity || o.recoverLocal != "" {
			return nil, enrollment, errors.New("recovery requires stored configuration")
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

// localAccounts prepares local account sign-in. On a configured console, it removes
// credentials whose account no longer exists, such as one left by a lost setup claim.
func localAccounts(
	ctx context.Context,
	pg *store.Postgres,
	key []byte,
	configuration *settings.Service,
) (*localauth.Authenticator, error) {
	credentials, err := localauth.NewStore(pg, key)
	if err != nil {
		return nil, err
	}
	if configuration.Initialized() {
		current, _ := configuration.Current()
		if err := credentials.Prune(ctx, current.LocalAccountIDs()); err != nil {
			return nil, fmt.Errorf("remove credentials of deleted local accounts: %w", err)
		}
	}
	hasher, err := localauth.NewHasher()
	if err != nil {
		return nil, err
	}
	return localauth.NewAuthenticator(credentials, hasher, localauth.NewThrottle()), nil
}

func liveServer(
	ctx context.Context,
	o options,
	pg *store.Postgres,
	configuration *settings.Service,
	enrollment config.Config,
	local *localauth.Authenticator,
	logger *slog.Logger,
) (*httpapi.Server, error) {
	callback := environment("CONSOLE_OIDC_REDIRECT_URL")
	parsed, err := url.Parse(callback)
	if err != nil {
		return nil, err
	}
	authentication, setup, err := liveAuthentication(ctx, configuration, callback, enrollment)
	if err != nil {
		return nil, err
	}
	authentication.Logger = logger
	if setup != nil {
		setup.Logger = logger
		setup.Local = local
	}
	return &httpapi.Server{
		Fleet:           view.Live{Observations: pg, Enrollment: configuration},
		Database:        pg,
		Logger:          logger,
		Auth:            authentication,
		Settings:        configuration,
		Setup:           setup,
		Origin:          parsed.Scheme + "://" + parsed.Host,
		ReaderName:      o.readerName,
		ReaderNamespace: o.readerNamespace,
		ClientIP:        o.clientIP,
		Local:           local,
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
			// The console uses local accounts until an administrator adds a provider.
			return auth.NewLocal(configuration.Resolve), nil, nil
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
	version, commit := buildinfo.Current()
	logger.Info("Console started", "mode", o.mode, "listen", o.address, "version", version, "commit", commit)
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
