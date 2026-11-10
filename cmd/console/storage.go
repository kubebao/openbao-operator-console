package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/httpapi"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
	"github.com/kubebao/openbao-operator-console/internal/view"
)

type liveStore interface {
	settings.Repository
	localauth.Repository
	collector.Sink
	view.Observations
	httpapi.Database
	RunPruning(context.Context, *slog.Logger)
	Close()
}

type storageOptions struct{ engine, location string }

func storageConfiguration() (storageOptions, error) {
	engine, dsn, path := environment("CONSOLE_STORAGE"), environment("CONSOLE_DATABASE_URL"), environment("CONSOLE_SQLITE_PATH")
	if engine == "" {
		if dsn != "" {
			engine = "postgres"
		} else {
			engine = "sqlite"
		}
	}
	switch engine {
	case "sqlite":
		if dsn != "" {
			return storageOptions{}, errors.New("CONSOLE_DATABASE_URL conflicts with CONSOLE_STORAGE=sqlite")
		}
		if path == "" {
			path = "data/console.db"
		}
		return storageOptions{engine, path}, nil
	case "postgres":
		if dsn == "" {
			return storageOptions{}, errors.New("CONSOLE_DATABASE_URL is required for PostgreSQL")
		}
		if path != "" {
			return storageOptions{}, errors.New("CONSOLE_SQLITE_PATH conflicts with PostgreSQL storage")
		}
		return storageOptions{engine, dsn}, nil
	default:
		return storageOptions{}, errors.New("CONSOLE_STORAGE must be sqlite or postgres")
	}
}

func openStorage(ctx context.Context, retention time.Duration) (liveStore, error) {
	config, err := storageConfiguration()
	if err != nil {
		return nil, err
	}
	if config.engine == "postgres" {
		return store.Open(ctx, config.location, retention)
	}
	return store.OpenSQLite(ctx, config.location, retention)
}

func runSchema(ctx context.Context, o options) error {
	config, err := storageConfiguration()
	if err != nil {
		return err
	}
	var plan store.SchemaPlan
	if config.engine == "postgres" {
		plan, err = store.MaintainPostgres(ctx, config.location, o.schema, o.backup)
	} else {
		plan, err = store.MaintainSQLite(ctx, config.location, o.schema, o.backup)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(plan)
}
