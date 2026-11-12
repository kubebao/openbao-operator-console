package httpapi

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func (s *Server) readCharts(w http.ResponseWriter, r *http.Request, user config.User) error {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := s.Fleet.Snapshot(ctx)
	if err != nil {
		return unavailable(err, "Chart evidence is temporarily unavailable.")
	}
	id := r.PathValue("id")
	if !slices.ContainsFunc(snapshot.Deployments, func(d fleet.Deployment) bool {
		return d.ID == id && user.CanView(d.ConnectionID, d.Namespace)
	}) {
		return &problemError{status: http.StatusNotFound, code: "not_found", message: "Deployment not found."}
	}
	// Both stores persist timestamps at microsecond precision.
	end := snapshot.GeneratedAt.Truncate(time.Microsecond)
	data, err := s.Fleet.Charts(ctx, id, end.Add(-7*24*time.Hour), end)
	if err != nil {
		return unavailable(err, "Chart evidence is temporarily unavailable.")
	}
	jsonResponse(w, http.StatusOK, contract.ChartHistory{
		Start: data.Start, End: data.End,
		Observations: mapSlice(data.Observations, chartContract),
		Baseline:     optional(data.Baseline, chartContract),
	})
	return nil
}

func chartContract(o fleet.ChartObservation) contract.ChartObservation {
	return contract.ChartObservation{RecordedAt: o.RecordedAt, Deployment: contract.ChartDeployment{
		Health:   contract.ChartDeploymentHealth(o.Deployment.Health),
		Evidence: contract.ChartEvidence{Collection: contract.ChartEvidenceCollection(o.Deployment.Evidence.Collection)},
		Backup: optional(o.Deployment.Backup, func(b fleet.ChartBackup) contract.ChartBackup {
			return contract.ChartBackup{
				LastSuccessAt:       b.LastSuccessAt,
				ConsecutiveFailures: optional(b.ConsecutiveFailures, func(value int32) int { return int(value) }),
				SizeBytes:           b.SizeBytes,
			}
		}),
	}}
}
