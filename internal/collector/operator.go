package collector

import (
	"context"
	"regexp"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// OCI references cannot contain credentials, query strings, or whitespace.
var imageReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:-]*(?:@sha256:[a-f0-9]{64})?$`)

// enrolledOperator carries the enrollment identity of a controller before it is read.
func enrolledOperator(enrolled config.Operator, collection fleet.OperatorCollection, now time.Time) fleet.Operator {
	return fleet.Operator{
		ID:            enrolled.ID,
		Namespace:     enrolled.Namespace,
		Name:          enrolled.Deployment,
		Container:     enrolled.Container,
		Namespaces:    append([]string{}, enrolled.Namespaces...),
		Collection:    collection,
		LastAttemptAt: now,
		Health:        fleet.OperatorHealthUnknown,
	}
}

func operatorCollection(err error) fleet.OperatorCollection {
	switch {
	case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
		return fleet.OperatorCollectionDenied
	case apierrors.IsNotFound(err):
		return fleet.OperatorCollectionNotFound
	default:
		return fleet.OperatorCollectionUnreachable
	}
}

func (r *Reader) operator(ctx context.Context, enrolled config.Operator, now time.Time) fleet.Operator {
	result := enrolledOperator(enrolled, fleet.OperatorCollectionCurrent, now)
	workload, err := r.client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).
		Namespace(enrolled.Namespace).Get(ctx, enrolled.Deployment, metav1.GetOptions{})
	if err != nil {
		result.Collection = operatorCollection(err)
		return result
	}
	result.LastSuccessAt = &now
	uid := string(workload.GetUID())
	result.UID = &uid
	result.Generation = workload.GetGeneration()
	if generation, found, err := unstructured.NestedInt64(workload.Object, "status", "observedGeneration"); found && err == nil {
		result.ObservedGeneration = &generation
	}
	result.Version = optionalText(workload.Object, "metadata", "labels", "app.kubernetes.io/version")
	result.DesiredReplicas = count(workload.Object, "spec", "replicas")
	result.ReadyReplicas = count(workload.Object, "status", "readyReplicas")
	result.UpdatedReplicas = count(workload.Object, "status", "updatedReplicas")
	containers, _, _ := unstructured.NestedSlice(workload.Object, "spec", "template", "spec", "containers")
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok || text(container, "name") != enrolled.Container {
			continue
		}
		if image, found, err := unstructured.NestedString(
			container,
			"image",
		); err == nil && found && len(image) <= 512 &&
			imageReference.MatchString(image) {
			result.Image = &image
		}
	}
	if result.Image == nil {
		result.Collection = fleet.OperatorCollectionInvalid
		return result
	}
	if result.ObservedGeneration == nil || *result.ObservedGeneration < result.Generation || result.DesiredReplicas == nil {
		return result
	}
	// Kubernetes omits zero counts. Treat them as zero only after observation.
	zero := int32(0)
	if result.ReadyReplicas == nil {
		result.ReadyReplicas = &zero
	}
	if result.UpdatedReplicas == nil {
		result.UpdatedReplicas = &zero
	}
	result.Health = fleet.OperatorHealthProgressing
	if *result.DesiredReplicas == 0 || *result.ReadyReplicas == 0 {
		result.Health = fleet.OperatorHealthUnavailable
	}
	desired := *result.DesiredReplicas
	rolledOut := desired > 0 && *result.ReadyReplicas >= desired && *result.UpdatedReplicas >= desired
	conditions, _, _ := unstructured.NestedSlice(workload.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if ok && rolledOut && text(condition, "type") == "Available" && text(condition, "status") == "True" {
			result.Health = fleet.OperatorHealthAvailable
		}
	}
	return result
}
