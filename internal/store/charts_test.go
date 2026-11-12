package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestChartsReadTheCompleteWindowAndProjectOnlyChartFields(t *testing.T) {
	storageBackends(t, chartsReadTheCompleteWindowAndProjectOnlyChartFields)
}

func chartsReadTheCompleteWindowAndProjectOnlyChartFields(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Microsecond).Add(-5 * time.Hour)
	end := start.Add(250 * time.Minute)
	for i := -1; i <= 251; i++ {
		at := start.Add(time.Duration(i) * time.Minute)
		deployment := chartFixture(start, i)
		payload, err := json.Marshal(deployment)
		if err != nil {
			t.Fatal(err)
		}
		payload = append(payload[:len(payload)-1], []byte(`,"rawProviderData":"must-not-reach-chart-output"}`)...)
		if err := execFixture(
			ctx,
			p,
			"INSERT INTO console_observation(deployment_id, connection_id, recorded_at, projection) VALUES ($1,$2,$3,$4)",
			"bao",
			"cluster",
			at,
			string(payload),
		); err != nil {
			t.Fatal(err)
		}
	}
	data, err := p.Charts(ctx, "bao", start, end)
	if err != nil {
		t.Fatal(err)
	}
	if data.Baseline == nil || !data.Baseline.RecordedAt.Equal(start.Add(-time.Minute)) {
		t.Fatal("chart window lost its pre-window baseline")
	}
	if len(data.Observations) != 6 {
		t.Fatalf("chart-relevant changes were lost or unchanged projections were retained: %d", len(data.Observations))
	}
	if !data.Observations[0].RecordedAt.Equal(start.Add(220*time.Minute)) ||
		data.Observations[0].Deployment.Backup.ConsecutiveFailures == nil {
		t.Fatal("charts lost changes beyond the first observation page")
	}
	if !data.Observations[1].RecordedAt.Equal(start.Add(200*time.Minute)) ||
		data.Observations[1].Deployment.Backup.ConsecutiveFailures != nil {
		t.Fatal("unknown backup counters became known")
	}
	if data.Observations[5].Deployment.Health != fleet.HealthAttention ||
		!data.Observations[5].RecordedAt.Equal(start.Add(25*time.Minute)) {
		t.Fatal("brief attention interval was lost")
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-reach") || strings.Contains(string(encoded), "resourceVersion") {
		t.Fatal("chart output includes fields outside its projection")
	}
	page, err := p.History(ctx, "bao", fleet.HistoryQuery{Since: &start, Page: fleet.Page{Limit: 100}})
	if err != nil || len(page.Observations) != 100 || page.Next == nil {
		t.Fatalf("observation pagination changed: %+v, %v", page.Next, err)
	}
	missing, err := p.Charts(ctx, "missing", start, end)
	if err != nil || len(missing.Observations) != 0 || missing.Baseline != nil {
		t.Fatal("empty chart window invented evidence")
	}
}

func chartFixture(start time.Time, index int) fleet.Deployment {
	deployment := fleet.Deployment{Health: fleet.HealthHealthy, Evidence: fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent}}
	if index == 25 {
		deployment.Health = fleet.HealthAttention
	}
	if index >= 100 {
		completed := start.Add(100 * time.Minute)
		deployment.Backup = &fleet.Backup{LastSuccessAt: &completed, SizeBytes: new(int64(1024))}
		if index >= 150 && index < 200 {
			deployment.Backup.ConsecutiveFailures = new(int32(2))
		}
		if index >= 220 {
			deployment.Backup.ConsecutiveFailures = new(int32(3))
		}
	}
	return deployment
}
