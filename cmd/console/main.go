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
)

func run() error {
	mode := flag.String("mode", "demo", "demo or live")
	address := flag.String("listen", "127.0.0.1:8090", "HTTP listen address")
	path := flag.String("config", "", "optional first-start enrollment YAML file")
	recoverIdentity := flag.Bool("recover-identity", false, "replace stored identity and enroll administrators from bootstrap environment; exit without serving")
	check := flag.Bool("check", false, "check enrolled Kubernetes reads and print approved projections; no database or OIDC required")
	printRBAC := flag.String("print-rbac", "", "print reader RBAC for this connection ID; does not apply manifests")
	readerName := flag.String("reader-name", "console-reader", "local reader ServiceAccount name for RBAC manifests")
	readerNamespace := flag.String("reader-namespace", "console-system", "local reader ServiceAccount namespace for RBAC manifests")
	tlsCertificate := flag.String("tls-cert", "", "optional HTTPS server certificate PEM file")
	tlsKey := flag.String("tls-key", "", "optional HTTPS server private key PEM file")
	flag.Parse()
	if (*tlsCertificate == "") != (*tlsKey == "") {
		return fmt.Errorf("tls-cert and tls-key must be supplied together")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if *check || *printRBAC != "" {
		enrollment, err := config.Load(*path)
		if err != nil {
			return err
		}
		if *printRBAC != "" {
			for _, connection := range enrollment.Connections {
				if connection.ID == *printRBAC {
					return config.WriteRBAC(os.Stdout, connection, *readerName, *readerNamespace)
				}
			}
			return fmt.Errorf("connection is not enrolled")
		}
		return checkConnections(ctx, enrollment)
	}
	var repository store.Repository
	var authentication *auth.Authenticator
	var enrollment config.Config
	var configuration *settings.Service
	var browserSetup *httpapi.Setup
	origin := ""
	var collectorStopped chan struct{}
	var renewalStopped chan struct{}
	var shutdownDone chan struct{}
	switch *mode {
	case "demo":
		if *recoverIdentity {
			return fmt.Errorf("recover-identity requires live mode")
		}
		host, _, err := net.SplitHostPort(*address)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("demo mode must listen on a loopback IP")
		}
		repository = demo.New(time.Now().UTC())
		defer repository.Close()
		authentication = auth.NewDemo()
	case "live":
		dsn := os.Getenv("CONSOLE_DATABASE_URL")
		if dsn == "" {
			return fmt.Errorf("CONSOLE_DATABASE_URL is required in live mode")
		}
		startup, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		pg, err := store.Open(startup, dsn, 7*24*time.Hour)
		if err != nil {
			return fmt.Errorf("database startup failed")
		}
		repository = pg
		defer pg.Close()
		key, err := os.ReadFile(os.Getenv("CONSOLE_CONFIG_KEY_FILE"))
		if err != nil {
			return fmt.Errorf("CONSOLE_CONFIG_KEY_FILE must reference a readable 32-byte key")
		}
		if _, loadErr := pg.LoadSettings(startup); errors.Is(loadErr, settings.ErrMissing) {
			if *recoverIdentity {
				return fmt.Errorf("identity recovery requires stored configuration")
			}
			enrollment, err = bootstrapSeed(*path)
			if err != nil {
				return err
			}
		} else if loadErr != nil {
			return fmt.Errorf("read stored configuration")
		}
		var seed *config.Config
		if enrollment.Identity != nil {
			seed = &enrollment
		}
		configuration, err = settings.Open(startup, pg, key, seed)
		if err != nil {
			return fmt.Errorf("initialize console settings: %w", err)
		}
		if *recoverIdentity {
			return recoverConfiguration(startup, configuration)
		}
		callback := os.Getenv("CONSOLE_OIDC_REDIRECT_URL")
		parsed, err := url.Parse(callback)
		if err != nil {
			return err
		}
		origin = parsed.Scheme + "://" + parsed.Host
		if configuration.Initialized() {
			enrollment, _ = configuration.Current()
			if enrollment.Identity == nil {
				return fmt.Errorf("identity provider is not configured")
			}
			identity := enrollment.Identity
			providerContext, err := auth.ProviderContextPEM(startup, []byte(identity.CA))
			if err != nil {
				return err
			}
			authentication, err = auth.NewOIDC(providerContext, identity.Issuer, identity.ClientID, identity.ClientSecret, callback, enrollment.Users)
			if err != nil {
				return fmt.Errorf("authentication startup failed")
			}
			authentication.SetGroupsClaim(identity.GroupsClaim)
		} else {
			token, err := readSetupToken(os.Getenv("CONSOLE_SETUP_TOKEN_FILE"))
			if err != nil {
				return err
			}
			authentication = auth.NewPending()
			browserSetup, err = httpapi.NewSetup(configuration, authentication, token, callback, enrollment)
			clear(token)
			if err != nil {
				return fmt.Errorf("initialize browser setup: %w", err)
			}
		}
		authentication.SetResolver(configuration.Resolve)
		collectorStopped = make(chan struct{})
		go func() { defer close(collectorStopped); collector.RunManaged(ctx, configuration, repository, logger) }()
		renewalStopped = make(chan struct{})
		go func() {
			defer close(renewalStopped)
			(renewal.Manager{Repository: configuration, Issuer: renewal.KubernetesIssuer{}, Logger: logger}).Run(ctx)
		}()
	default:
		return fmt.Errorf("mode must be demo or live")
	}
	defer func() {
		stop()
		if shutdownDone != nil {
			<-shutdownDone
		}
		if collectorStopped != nil {
			<-collectorStopped
		}
		if renewalStopped != nil {
			<-renewalStopped
		}
	}()
	application := &httpapi.Server{Repository: repository, Auth: authentication, Config: enrollment, Settings: configuration, Setup: browserSetup, Demo: *mode == "demo", Origin: origin, ReaderName: *readerName, ReaderNamespace: *readerNamespace}
	server := &http.Server{
		Addr: *address, Handler: application.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	shutdownDone = make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("Console started", "mode", *mode, "listen", *address)
	var err error
	if *tlsCertificate != "" {
		err = server.ListenAndServeTLS(*tlsCertificate, *tlsKey)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
