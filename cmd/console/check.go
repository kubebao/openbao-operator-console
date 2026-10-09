package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

func checkConnections(ctx context.Context, enrollment config.Config) error {
	results := []store.Batch{}
	failed := false
	policy := enrollment.Policy()
	for _, connection := range enrollment.Connections {
		reader, err := collector.New(connection, policy)
		if err != nil {
			reader = collector.Unconfigured(connection, policy)
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		batch := reader.Collect(bounded, time.Now().UTC())
		cancel()
		results = append(results, batch)
		failed = failed || batch.Connection.Collection != fleet.ConnectionCollectionCurrent
		for _, operator := range batch.Connection.Operators {
			failed = failed || operator.Collection != fleet.OperatorCollectionCurrent
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
