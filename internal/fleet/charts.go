package fleet

import (
	"reflect"
	"slices"
	"time"
)

// ChartHistory contains every chart-relevant change in one complete time window.
// It does not contain observation-list metadata or a pagination cursor.
type ChartHistory struct {
	Start        time.Time
	End          time.Time
	Observations []ChartObservation
	Baseline     *ChartObservation
}

type ChartObservation struct {
	RecordedAt time.Time       `json:"recordedAt"`
	Deployment ChartDeployment `json:"deployment"`
}

type ChartDeployment struct {
	Health   Health        `json:"health"`
	Evidence ChartEvidence `json:"evidence"`
	Backup   *ChartBackup  `json:"backup"`
}

type ChartEvidence struct {
	Collection EvidenceCollection `json:"collection"`
}

type ChartBackup struct {
	LastSuccessAt       *time.Time `json:"lastSuccessAt"`
	ConsecutiveFailures *int32     `json:"consecutiveFailures"`
	SizeBytes           *int64     `json:"sizeBytes"`
}

// Chart projects only the fields needed by state and backup charts.
func (o Observation) Chart() ChartObservation {
	projection := ChartDeployment{Health: o.Deployment.Health, Evidence: ChartEvidence{Collection: o.Deployment.Evidence.Collection}}
	if backup := o.Deployment.Backup; backup != nil {
		projection.Backup = &ChartBackup{
			LastSuccessAt:       backup.LastSuccessAt,
			ConsecutiveFailures: backup.ConsecutiveFailures,
			SizeBytes:           backup.SizeBytes,
		}
	}
	return ChartObservation{RecordedAt: o.RecordedAt, Deployment: projection}
}

// CompleteCharts accepts oldest-first points and preserves the first point of every
// chart-relevant change, including timestamp ties. Its output is newest first.
func CompleteCharts(start, end time.Time, baseline *ChartObservation, points []ChartObservation) ChartHistory {
	result := ChartHistory{Start: start, End: end, Baseline: baseline, Observations: []ChartObservation{}}
	previous := baseline
	for _, point := range points {
		if point.RecordedAt.Before(start) || point.RecordedAt.After(end) {
			continue
		}
		if previous != nil && reflect.DeepEqual(previous.Deployment, point.Deployment) {
			continue
		}
		result.Observations = append(result.Observations, point)
		previous = &result.Observations[len(result.Observations)-1]
	}
	slices.Reverse(result.Observations)
	return result
}
