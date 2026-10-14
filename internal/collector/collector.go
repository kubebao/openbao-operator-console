package collector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubebao/openbao-operator-console/internal/buildinfo"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/kube"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

type Reader struct {
	client     dynamic.Interface
	connection config.Connection
	policy     fleet.Policy
}

// Unconfigured keeps a broken enrollment visible and retries configuration each round.
func Unconfigured(connection config.Connection, policy fleet.Policy) *Reader {
	return &Reader{connection: connection, policy: policy}
}

func clientConfig(connection config.Connection) (*rest.Config, error) {
	access := connection.Access
	switch access.Mode {
	case config.AccessRemote:
		if access.Credential == nil {
			return nil, errors.New("remote connector has no reader credential")
		}
		if err := access.Credential.Validate(); err != nil {
			return nil, err
		}
		return kube.RESTConfig(*access.Credential), nil
	case config.AccessKubeconfig:
		loading := &clientcmd.ClientConfigLoadingRules{ExplicitPath: access.Kubeconfig}
		overrides := &clientcmd.ConfigOverrides{CurrentContext: access.Context}
		return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides).ClientConfig()
	case config.AccessInCluster:
		return rest.InClusterConfig()
	default:
		return nil, fmt.Errorf("unknown access mode %q", access.Mode)
	}
}

func New(connection config.Connection, policy fleet.Policy) (*Reader, error) {
	restConfig, err := clientConfig(connection)
	if err != nil {
		return nil, fmt.Errorf("configure connector %q: %w", connection.ID, err)
	}
	restConfig.Timeout = 10 * time.Second
	restConfig.UserAgent = buildinfo.UserAgent()
	httpClient, err := kube.HTTPClient(restConfig)
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfigAndClient(restConfig, httpClient)
	if err != nil {
		return nil, err
	}
	return &Reader{client: client, connection: connection, policy: policy}, nil
}

func (r *Reader) list(ctx context.Context, namespace, resource string) ([]unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{Group: "openbao.org", Version: "v1alpha1", Resource: resource}
	result := []unstructured.Unstructured{}
	continuation := ""
	for {
		page, err := r.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: 200, Continue: continuation})
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if len(result) > 10000 {
			return nil, errors.New("connector resource limit exceeded")
		}
		continuation = page.GetContinue()
		if continuation == "" {
			break
		}
	}
	return result, nil
}

func collectionState(err error) fleet.ConnectionCollection {
	switch {
	case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
		return fleet.ConnectionCollectionDenied
	case apierrors.IsNotFound(err):
		return fleet.ConnectionCollectionUnsupported
	default:
		return fleet.ConnectionCollectionUnreachable
	}
}

func capabilityState(err error) fleet.CapabilityState {
	switch {
	case err == nil:
		return fleet.CapabilityStateAvailable
	case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
		return fleet.CapabilityStateDenied
	case apierrors.IsNotFound(err):
		return fleet.CapabilityStateUnsupported
	default:
		return fleet.CapabilityStateUnreachable
	}
}

func (r *Reader) Collect(ctx context.Context, now time.Time) store.Batch {
	return r.collect(ctx, now, nil)
}

// pending lists every enrolled namespace and API before any read succeeds.
func (r *Reader) pending(now time.Time) fleet.Connection {
	connection := fleet.Connection{
		ID:            r.connection.ID,
		Name:          r.connection.Name,
		Region:        r.connection.Region,
		Collection:    fleet.ConnectionCollectionCurrent,
		LastAttemptAt: now,
		Namespaces:    []string{},
		Operators:     []fleet.Operator{},
		Capabilities:  []fleet.Capability{},
	}
	for _, namespace := range r.connection.Namespaces {
		connection.Namespaces = append(connection.Namespaces, namespace.Name)
		for _, resource := range []fleet.CapabilityResource{
			fleet.CapabilityResourceClusters,
			fleet.CapabilityResourceRestores,
			fleet.CapabilityResourceRecoveries,
		} {
			connection.Capabilities = append(
				connection.Capabilities,
				fleet.Capability{Namespace: namespace.Name, Resource: resource, State: fleet.CapabilityStateNotChecked},
			)
		}
	}
	return connection
}

func (r *Reader) collect(ctx context.Context, now time.Time, diagnostic *CheckDiagnostic) store.Batch {
	batch := store.Batch{Connection: r.pending(now), Deployments: []fleet.Deployment{}}
	diagnose := func(err error) {
		if diagnostic != nil {
			*diagnostic = checkDiagnostic(err)
		}
	}
	if r.client == nil {
		configured, err := New(r.connection, r.policy)
		if err != nil {
			diagnose(err)
			batch.Connection.Collection = fleet.ConnectionCollectionUnreachable
			for _, operator := range r.connection.Operators {
				batch.Connection.Operators = append(
					batch.Connection.Operators,
					enrolledOperator(operator, fleet.OperatorCollectionUnreachable, now),
				)
			}
			return batch
		}
		r.client = configured.client
	}
	for _, operator := range r.connection.Operators {
		batch.Connection.Operators = append(batch.Connection.Operators, r.operator(ctx, operator, now))
	}
	for _, namespace := range r.connection.Namespaces {
		deployments, err := r.namespace(ctx, &batch.Connection, namespace, now)
		if err != nil {
			diagnose(err)
			batch.Connection.Collection = collectionState(err)
			batch.Deployments = nil
			r.client = nil
			return batch
		}
		batch.Deployments = append(batch.Deployments, deployments...)
	}
	batch.Connection.LastSuccessAt = &now
	return batch
}

// read lists one API in one namespace and records the result as a capability.
// Optional APIs that are absent or forbidden yield no items instead of an error.
func (r *Reader) read(
	ctx context.Context,
	connection *fleet.Connection,
	namespace string,
	resource fleet.CapabilityResource,
	optional bool,
	now time.Time,
) ([]unstructured.Unstructured, error) {
	items, err := r.list(ctx, namespace, string(resource))
	for i := range connection.Capabilities {
		capability := &connection.Capabilities[i]
		if capability.Namespace == namespace && capability.Resource == resource {
			capability.CheckedAt = &now
			capability.State = capabilityState(err)
		}
	}
	if optional && (apierrors.IsNotFound(err) || apierrors.IsForbidden(err)) {
		return []unstructured.Unstructured{}, nil
	}
	return items, err
}

func (r *Reader) namespace(
	ctx context.Context,
	connection *fleet.Connection,
	namespace config.Namespace,
	now time.Time,
) ([]fleet.Deployment, error) {
	clusters, err := r.read(ctx, connection, namespace.Name, fleet.CapabilityResourceClusters, false, now)
	if err != nil {
		return nil, err
	}
	restores, err := r.read(ctx, connection, namespace.Name, fleet.CapabilityResourceRestores, true, now)
	if err != nil {
		return nil, err
	}
	recoveries, err := r.read(ctx, connection, namespace.Name, fleet.CapabilityResourceRecoveries, true, now)
	if err != nil {
		return nil, err
	}
	requests := slices.Concat(restores, recoveries)
	result := []fleet.Deployment{}
	for _, cluster := range clusters {
		deployment := Project(cluster, r.connection, namespace, now)
		fleet.AssignOperator(&deployment, connection.Operators)
		deployment.Recovery = latestRecovery(requests, deployment, cluster.GetCreationTimestamp().Time, now, r.policy)
		evidence.Evaluate(&deployment, cluster.GetCreationTimestamp().Time, boolean(cluster.Object, "spec", "paused"), r.policy, now)
		result = append(result, deployment)
	}
	return result, nil
}

// latestRecovery selects the newest restore or recovery request that targets the
// deployment. A request bound to another UID never applies to a replacement. A request
// without a bound UID applies only when it was created at or after the cluster, so a
// request for an earlier cluster with the same name never attaches to its replacement.
func latestRecovery(
	requests []unstructured.Unstructured,
	deployment fleet.Deployment,
	clusterCreated, now time.Time,
	policy fleet.Policy,
) *fleet.Recovery {
	var result *fleet.Recovery
	var newest time.Time
	for _, request := range requests {
		mechanism := fleet.RecoveryMechanismSnapshot
		name := text(request.Object, "spec", "cluster")
		uid := text(request.Object, "status", "execution", "targetUID")
		if request.GetKind() == "OpenBaoRecovery" {
			mechanism = fleet.RecoveryMechanismNative
			name = text(request.Object, "spec", "clusterRef", "name")
			uid = text(request.Object, "status", "clusterRef", "uid")
		}
		created := request.GetCreationTimestamp().Time
		if name != deployment.Name || (uid != "" && uid != deployment.UID) || (uid == "" && created.Before(clusterCreated)) ||
			created.Before(newest) {
			continue
		}
		newest = created
		result = ProjectRecovery(request, mechanism, now, policy.StartupGrace)
	}
	return result
}

// Sink persists collection batches and forgets connections that left the enrollment.
type Sink interface {
	Apply(context.Context, store.Batch) error
	RetainConnections(context.Context, []string) error
}
