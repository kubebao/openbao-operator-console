package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

func checkConnections(ctx context.Context, enrollment config.Config) error {
	results := []store.Batch{}
	failed := false
	for _, connection := range enrollment.Connections {
		policy := evidence.Policy{MaxBackupAge: time.Duration(enrollment.MaxBackupAgeHours) * time.Hour, StartupGrace: time.Duration(enrollment.StartupGraceMinutes) * time.Minute}
		reader, err := collector.New(connection, policy)
		if err != nil {
			reader = collector.Unconfigured(connection, policy)
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		batch := reader.Collect(bounded, time.Now().UTC())
		cancel()
		results = append(results, batch)
		failed = failed || batch.Connection.Collection != "current"
		for _, operator := range batch.Connection.Operators {
			failed = failed || operator.Collection != "current"
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return err
	}
	if failed {
		return errors.New("connection check incomplete; inspect collection and capability states")
	}
	return nil
}
