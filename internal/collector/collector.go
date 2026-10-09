package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

type Reader struct {
	client     dynamic.Interface
	connection config.Connection
	policy     evidence.Policy
}

// Unconfigured keeps a broken enrollment visible and retries configuration each round.
func Unconfigured(connection config.Connection, policy evidence.Policy) *Reader {
	return &Reader{connection: connection, policy: policy}
}

func New(connection config.Connection, policy evidence.Policy) (*Reader, error) {
	var restConfig *rest.Config
	var err error
	if connection.Credential != nil {
		credential := connection.Credential
		if err := credential.Validate(); err != nil {
			return nil, err
		}
		restConfig = &rest.Config{Host: credential.Server, BearerToken: credential.Token, TLSClientConfig: rest.TLSClientConfig{CAData: []byte(credential.CA), CertData: []byte(credential.Certificate), KeyData: []byte(credential.Key)}}
	} else if connection.Kubeconfig == "" && connection.Context == "" {
		if connection.Remote {
			return nil, errors.New("remote connector has no reader credential")
		}
		restConfig, err = rest.InClusterConfig()
	} else {
		loading := &clientcmd.ClientConfigLoadingRules{ExplicitPath: connection.Kubeconfig}
		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, &clientcmd.ConfigOverrides{CurrentContext: connection.Context}).ClientConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("configure connector %q: %w", connection.ID, err)
	}
	restConfig.Timeout = 10 * time.Second
	restConfig.UserAgent = "openbao-operator-console/0.1"
	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, err
	}
	// Reader credentials must never follow a redirect to another endpoint.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
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

func collectionState(err error) contract.ConnectionCollection {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return "denied"
	}
	if apierrors.IsNotFound(err) {
		return "unsupported"
	}
	return "unreachable"
}

func (r *Reader) Collect(ctx context.Context, now time.Time) store.Batch {
	return r.collect(ctx, now, nil)
}

func (r *Reader) collect(ctx context.Context, now time.Time, diagnostic *CheckDiagnostic) store.Batch {
	connection := contract.Connection{Id: r.connection.ID, Name: r.connection.Name, Region: r.connection.Region, Collection: "current", LastAttemptAt: now, Namespaces: []string{}, Operators: []contract.Operator{}, Capabilities: []contract.ApiCapability{}}
	batch := store.Batch{Connection: connection, Deployments: []contract.Deployment{}}
	for _, namespace := range r.connection.Namespaces {
		batch.Connection.Namespaces = append(batch.Connection.Namespaces, namespace.Name)
		for _, resource := range []contract.ApiCapabilityResource{"openbaoclusters", "openbaorestores", "openbaorecoveries"} {
			batch.Connection.Capabilities = append(batch.Connection.Capabilities, contract.ApiCapability{Namespace: namespace.Name, Resource: resource, State: "notChecked"})
		}
	}
	if r.client == nil {
		configured, err := New(r.connection, r.policy)
		if err == nil {
			r.client = configured.client
		} else {
			if diagnostic != nil {
				*diagnostic = checkDiagnostic(err)
			}
			batch.Connection.Collection = "unreachable"
			for _, operator := range r.connection.Operators {
				batch.Connection.Operators = append(batch.Connection.Operators, contract.Operator{Id: operator.ID, Namespace: operator.Namespace, Name: operator.Deployment, Container: operator.Container, Namespaces: append([]string{}, operator.Namespaces...), Collection: "unreachable", Health: "unknown", LastAttemptAt: now})
			}
			return batch
		}
	}
	for _, operator := range r.connection.Operators {
		batch.Connection.Operators = append(batch.Connection.Operators, r.operator(ctx, operator, now))
	}
	list := func(namespace, resource string, optional bool) ([]unstructured.Unstructured, error) {
		items, err := r.list(ctx, namespace, resource)
		for i := range batch.Connection.Capabilities {
			capability := &batch.Connection.Capabilities[i]
			if capability.Namespace == namespace && string(capability.Resource) == resource {
				capability.CheckedAt = &now
				capability.State = "available"
				if err != nil {
					capability.State = contract.ApiCapabilityState(collectionState(err))
				}
			}
		}
		if optional && (apierrors.IsNotFound(err) || apierrors.IsForbidden(err)) {
			return []unstructured.Unstructured{}, nil
		}
		return items, err
	}
	for _, namespace := range r.connection.Namespaces {
		clusters, err := list(namespace.Name, "openbaoclusters", false)
		if err != nil {
			if diagnostic != nil {
				*diagnostic = checkDiagnostic(err)
			}
			batch.Connection.Collection = collectionState(err)
			batch.Deployments = nil
			r.client = nil
			return batch
		}
		restores, err := list(namespace.Name, "openbaorestores", true)
		if err != nil {
			if diagnostic != nil {
				*diagnostic = checkDiagnostic(err)
			}
			batch.Connection.Collection = collectionState(err)
			batch.Deployments = nil
			r.client = nil
			return batch
		}
		recoveries, err := list(namespace.Name, "openbaorecoveries", true)
		if err != nil {
			if diagnostic != nil {
				*diagnostic = checkDiagnostic(err)
			}
			batch.Connection.Collection = collectionState(err)
			batch.Deployments = nil
			r.client = nil
			return batch
		}
		for _, cluster := range clusters {
			deployment := Project(cluster, r.connection, namespace, r.policy, now)
			AssociateOperators(&deployment, batch.Connection.Operators)
			var newest time.Time
			for _, request := range append(restores, recoveries...) {
				mechanism := contract.RecoveryMechanismSnapshot
				name := text(request.Object, "spec", "cluster")
				uid := text(request.Object, "status", "execution", "targetUID")
				if request.GetKind() == "OpenBaoRecovery" {
					mechanism = "native"
					name = text(request.Object, "spec", "clusterRef", "name")
					uid = text(request.Object, "status", "clusterRef", "uid")
				}
				if name != deployment.Name || (uid != "" && uid != deployment.Uid) || request.GetCreationTimestamp().Time.Before(newest) {
					continue
				}
				newest = request.GetCreationTimestamp().Time
				deployment.Recovery = ProjectRecovery(request, mechanism)
			}
			evidence.Evaluate(&deployment, cluster.GetCreationTimestamp().Time, boolean(cluster.Object, "spec", "paused"), r.policy, now)
			batch.Deployments = append(batch.Deployments, deployment)
		}
	}
	batch.Connection.LastSuccessAt = &now
	return batch
}

func Run(ctx context.Context, readers []*Reader, repository store.Repository, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		collectRound(ctx, readers, repository, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// A bounded pool prevents one slow connection from delaying the whole fleet.
// Rounds do not overlap, so each connection has at most one in-flight collection.
func collectRound(ctx context.Context, readers []*Reader, repository store.Repository, logger *slog.Logger) {
	jobs := make(chan *Reader)
	var workers sync.WaitGroup
	for range min(4, len(readers)) {
		workers.Go(func() {
			for reader := range jobs {
				if ctx.Err() != nil {
					continue
				}
				collectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				batch := reader.Collect(collectCtx, time.Now().UTC())
				cancel()
				writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
				if err := repository.Apply(writeCtx, batch); err != nil && !errors.Is(err, settings.ErrConflict) && ctx.Err() == nil {
					logger.Error("Persist collection failed", "connection", reader.connection.ID)
				}
				writeCancel()
			}
		})
	}
	for _, reader := range readers {
		select {
		case jobs <- reader:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}
