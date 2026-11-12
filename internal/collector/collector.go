package collector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	clientcollect "github.com/kubebao/openbao-operator-client/collect"
	"github.com/kubebao/openbao-operator-client/diagnose"
	shared "github.com/kubebao/openbao-operator-client/evidence"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	shared     *clientcollect.Client
	transport  *observationTransport
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

// observationTransport retains only transient error classification for check previews.
// Neither raw provider errors nor response bodies enter observations or HTTP output.
type observationTransport struct {
	base    http.RoundTripper
	status  int
	failure error
}

func (t *observationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("User-Agent", buildinfo.UserAgent())
	response, err := t.base.RoundTrip(request)
	t.status, t.failure = 0, err
	if response != nil {
		t.status = response.StatusCode
	}
	return response, err
}

func New(connection config.Connection, policy fleet.Policy) (*Reader, error) {
	if contractForGroup(connection.EffectiveAPIGroup()) == "" {
		return nil, errors.New("unsupported enrolled operator API group")
	}
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
	transport := &observationTransport{base: httpClient.Transport}
	sharedClient, err := clientcollect.NewClient(restConfig.Host, transport)
	if err != nil {
		return nil, fmt.Errorf("configure bounded reader: %w", err)
	}
	return &Reader{client: client, shared: sharedClient, transport: transport, connection: connection, policy: policy}, nil
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

func capabilityOutcome(outcome shared.CollectionOutcome) fleet.CapabilityState {
	switch outcome {
	case shared.CollectionComplete:
		return fleet.CapabilityStateAvailable
	case shared.CollectionDenied:
		return fleet.CapabilityStateDenied
	case shared.CollectionUnsupported:
		return fleet.CapabilityStateUnsupported
	default:
		return fleet.CapabilityStateUnreachable
	}
}

func (r *Reader) request(namespace, resource string) clientcollect.Request {
	limits := clientcollect.DefaultLimits()
	limits.MaxPages, limits.MaxCandidates, limits.PageSize = 64, 10000, 200
	limits.MaxBytes, limits.MaxResponseBytes, limits.MaxRetainedBytes = 64*1024*1024, 8*1024*1024, 64*1024*1024
	limits.Timeout = 10 * time.Second
	projection := adapter.DefaultProjectionLimits()
	projection.MaxBytes = limits.MaxResponseBytes
	return clientcollect.Request{Scope: shared.Scope{
		Origin: r.connection.ID, Group: string(r.connection.EffectiveAPIGroup()), Version: "v1alpha1", Resource: resource,
		Namespace: namespace, Action: shared.ReadList,
	}, Contract: contractForGroup(r.connection.EffectiveAPIGroup()), Limits: limits, Projection: projection}
}

func (r *Reader) readError(outcome shared.CollectionOutcome) error {
	if r.transport != nil && r.transport.failure != nil {
		return r.transport.failure
	}
	switch outcome {
	case shared.CollectionDenied:
		if r.transport != nil && r.transport.status == http.StatusUnauthorized {
			return apierrors.NewUnauthorized("reader credential rejected")
		}
		return apierrors.NewForbidden(
			schema.GroupResource{Group: string(r.connection.EffectiveAPIGroup()), Resource: "openbaoclusters"},
			"",
			nil,
		)
	case shared.CollectionUnsupported:
		return apierrors.NewNotFound(schema.GroupResource{Group: string(r.connection.EffectiveAPIGroup()), Resource: "openbaoclusters"}, "")
	case shared.CollectionTimeout:
		return context.DeadlineExceeded
	case shared.CollectionCancelled:
		return context.Canceled
	default:
		return errors.New("bounded resource collection failed")
	}
}

func (r *Reader) pending(now time.Time) fleet.Connection {
	result := fleet.Connection{
		ID: r.connection.ID, Name: r.connection.Name, Region: r.connection.Region, Collection: fleet.ConnectionCollectionCurrent,
		LastAttemptAt: now, Namespaces: []string{}, Operators: []fleet.Operator{}, Capabilities: []fleet.Capability{},
	}
	for _, namespace := range r.connection.Namespaces {
		result.Namespaces = append(result.Namespaces, namespace.Name)
		resources := []fleet.CapabilityResource{
			fleet.CapabilityResourceClusters, fleet.CapabilityResourceRestores, fleet.CapabilityResourceRecoveries,
		}
		for _, resource := range resources {
			state := fleet.CapabilityStateNotChecked
			// The pinned contract has no native-recovery API. This is adapter support,
			// not an attempted read or a claim about API discovery.
			if resource == fleet.CapabilityResourceRecoveries {
				state = fleet.CapabilityStateUnsupported
			}
			result.Capabilities = append(result.Capabilities, fleet.Capability{Namespace: namespace.Name, Resource: resource, State: state})
		}
	}
	return result
}

func capability(connection *fleet.Connection, namespace string, resource fleet.CapabilityResource, read shared.Collection) {
	for i := range connection.Capabilities {
		value := &connection.Capabilities[i]
		if value.Namespace == namespace && value.Resource == resource {
			value.State = capabilityOutcome(read.Outcome)
			if !read.Interval.CompletedAt.IsZero() {
				at := read.Interval.CompletedAt
				value.CheckedAt = &at
			}
		}
	}
}

func (r *Reader) Collect(ctx context.Context, now time.Time) store.Batch {
	return r.collect(ctx, now, nil)
}

func (r *Reader) collect(ctx context.Context, now time.Time, diagnostic *CheckDiagnostic) store.Batch {
	batch := store.Batch{Connection: r.pending(now), Deployments: []fleet.Deployment{}, Unresolved: map[string][]fleet.Rule{}}
	fail := func(err error) store.Batch {
		if diagnostic != nil {
			*diagnostic = checkDiagnostic(err)
		}
		batch.Connection.Collection = collectionState(err)
		batch.Deployments = nil
		r.client, r.shared = nil, nil
		batch.CompletedAt = time.Now().UTC()
		return batch
	}
	if r.client == nil || r.shared == nil {
		configured, err := New(r.connection, r.policy)
		if err != nil {
			for _, operator := range r.connection.Operators {
				batch.Connection.Operators = append(
					batch.Connection.Operators,
					enrolledOperator(operator, fleet.OperatorCollectionUnreachable, now),
				)
			}
			return fail(err)
		}
		r.client, r.shared, r.transport = configured.client, configured.shared, configured.transport
	}
	for _, operator := range r.connection.Operators {
		batch.Connection.Operators = append(batch.Connection.Operators, r.operator(ctx, operator, now))
	}
	for _, namespace := range r.connection.Namespaces {
		deployments, unresolved, err := r.namespace(ctx, &batch.Connection, namespace, now)
		if err != nil {
			return fail(err)
		}
		batch.Deployments = append(batch.Deployments, deployments...)
		for id, rules := range unresolved {
			batch.Unresolved[id] = rules
		}
	}
	batch.CompletedAt = time.Now().UTC()
	batch.Connection.LastSuccessAt = &batch.CompletedAt
	return batch
}

func (r *Reader) namespace(ctx context.Context, connection *fleet.Connection, namespace config.Namespace, attempted time.Time) (
	[]fleet.Deployment, map[string][]fleet.Rule, error,
) {
	request := r.request(namespace.Name, "openbaoclusters")
	initial, err := r.shared.Clusters(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	capability(connection, namespace.Name, fleet.CapabilityResourceClusters, initial.Collection)
	if initial.Collection.Outcome != shared.CollectionComplete {
		return nil, nil, r.readError(initial.Collection.Outcome)
	}
	restores, err := r.shared.Restores(ctx, r.request(namespace.Name, "openbaorestores"))
	if err != nil {
		return nil, nil, err
	}
	capability(connection, namespace.Name, fleet.CapabilityResourceRestores, restores.Collection)
	if restores.Collection.Outcome == shared.CollectionDenied && r.transport != nil && r.transport.status == http.StatusUnauthorized {
		return nil, nil, r.readError(restores.Collection.Outcome)
	}
	final, err := r.shared.Clusters(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	if final.Collection.Outcome != shared.CollectionComplete {
		return nil, nil, r.readError(final.Collection.Outcome)
	}
	if int64(len(initial.Observations))*int64(len(final.Observations)+len(restores.Observations)+1) > 1000000 {
		return nil, nil, errors.New("connector analysis limit exceeded")
	}
	result, unresolved := []fleet.Deployment{}, map[string][]fleet.Rule{}
	reference := time.Now().UTC()
	for _, cluster := range initial.Observations {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		verification, err := shared.NewTargetVerification(cluster, final.Collection, final.Observations)
		if err != nil {
			return nil, nil, fmt.Errorf("verify collected target: %w", err)
		}
		if verification.State == shared.VerificationDeleted || verification.State == shared.VerificationReplaced {
			continue
		}
		input := diagnose.ClusterInput{
			Subject:         cluster.Reference.Identity,
			AdapterContract: cluster.AdapterContract,
			Observation:     &cluster,
			Collection:      initial.Collection,
			Verification:    &verification,
			Restores: &diagnose.ResourceInput[shared.Restore]{
				AdapterContract: cluster.AdapterContract,
				Collection:      restores.Collection,
				Observations:    restores.Observations,
			},
		}
		policy := analysisPolicy(reference, r.policy, contractForGroup(r.connection.EffectiveAPIGroup()))
		policy.Rules = evidence.Rules()
		// Console restore-test presentation exists only for configured backups.
		if cluster.Backup.Configuration == shared.ObjectAbsent {
			selected := policy.Rules[:0]
			for _, rule := range policy.Rules {
				if rule != diagnose.RuleRestoreTest {
					selected = append(selected, rule)
				}
			}
			policy.Rules = selected
		}
		report, err := diagnose.Analyze(
			diagnose.Capture{SchemaVersion: shared.SchemaVersion, Clusters: []diagnose.ClusterInput{input}},
			policy,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("analyze collected evidence: %w", err)
		}
		deployment := projectCluster(cluster, r.connection, namespace)
		deployment.Evidence.LastAttemptAt = attempted
		if restores.Collection.Outcome == shared.CollectionComplete {
			attachRecovery(&deployment, cluster, restores.Observations)
		}
		applyRestoreTestPresentation(&deployment, cluster, report)
		fleet.AssignOperator(&deployment, connection.Operators)
		unresolved[deployment.ID] = evidence.Apply(&deployment, report)
		result = append(result, deployment)
	}
	return result, unresolved, nil
}

// Sink persists collection batches and forgets connections that left enrollment.
type Sink interface {
	Apply(context.Context, store.Batch) error
	RetainConnections(context.Context, []string) error
}
