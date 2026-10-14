package evidence

import (
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// Transitions lists the activity events between two observations of one deployment, in a
// fixed order: health, collection, raised and changed findings in evaluation order, then
// resolved findings. Findings are compared by rule. A finding whose detail changed without
// a severity change records history but no event, so failure counters do not flood the
// activity feed.
func Transitions(previous, current fleet.Deployment, at time.Time) []fleet.Activity {
	events := []fleet.Activity{}
	event := func(kind fleet.ActivityKind, suffix string) fleet.Activity {
		id := current.ID + ":" + at.Format(time.RFC3339Nano) + ":" + string(kind)
		if suffix != "" {
			id += ":" + suffix
		}
		return fleet.Activity{
			ID:             id,
			DeploymentID:   current.ID,
			DeploymentName: current.Name,
			ConnectionID:   current.ConnectionID,
			Namespace:      current.Namespace,
			RecordedAt:     at,
			Kind:           kind,
		}
	}
	if current.Health != previous.Health {
		change := event(fleet.ActivityHealthChanged, "")
		change.Health = &fleet.HealthChange{From: previous.Health, To: current.Health}
		events = append(events, change)
	}
	if current.Evidence.Collection != previous.Evidence.Collection {
		change := event(fleet.ActivityCollectionChanged, "")
		change.Collection = &fleet.CollectionChange{From: previous.Evidence.Collection, To: current.Evidence.Collection}
		events = append(events, change)
	}
	before := make(map[fleet.Rule]fleet.Issue, len(previous.Issues))
	for _, issue := range previous.Issues {
		before[issue.Rule] = issue
	}
	after := make(map[fleet.Rule]fleet.Issue, len(current.Issues))
	for _, issue := range current.Issues {
		after[issue.Rule] = issue
	}
	for _, issue := range current.Issues {
		old, existed := before[issue.Rule]
		switch {
		case !existed:
			change := event(fleet.ActivityFindingRaised, string(issue.Rule))
			change.Finding = &fleet.FindingChange{Rule: issue.Rule, Severity: issue.Severity}
			events = append(events, change)
		case old.Severity != issue.Severity:
			change := event(fleet.ActivityFindingSeverityChanged, string(issue.Rule))
			previousSeverity := old.Severity
			change.Finding = &fleet.FindingChange{Rule: issue.Rule, Severity: issue.Severity, PreviousSeverity: &previousSeverity}
			events = append(events, change)
		}
	}
	for _, issue := range previous.Issues {
		if _, remains := after[issue.Rule]; !remains {
			change := event(fleet.ActivityFindingResolved, string(issue.Rule))
			change.Finding = &fleet.FindingChange{Rule: issue.Rule, Severity: issue.Severity}
			events = append(events, change)
		}
	}
	return events
}
