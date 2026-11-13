package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type restoreQualification struct {
	kubeconfig  string
	contextName string
	group       config.APIGroup
	namespace   string
	target      string
	targetUID   string
	request     string
	requestUID  string
	phase       string
	finding     string
}

func restoreQualificationSettings(getenv func(string) string) (restoreQualification, bool, error) {
	settings := restoreQualification{
		kubeconfig: getenv("CONSOLE_TEST_KUBECONFIG"), contextName: getenv("CONSOLE_TEST_CONTEXT"),
		group: config.APIGroup(getenv("CONSOLE_TEST_API_GROUP")), namespace: getenv("CONSOLE_TEST_RESTORE_NAMESPACE"),
		target: getenv("CONSOLE_TEST_RESTORE_TARGET"), targetUID: getenv("CONSOLE_TEST_RESTORE_TARGET_UID"),
		request: getenv("CONSOLE_TEST_RESTORE_REQUEST"), requestUID: getenv("CONSOLE_TEST_RESTORE_UID"),
		phase: getenv("CONSOLE_TEST_RESTORE_PHASE"), finding: getenv("CONSOLE_TEST_RESTORE_FINDING"),
	}
	if settings.namespace == "" && settings.target == "" && settings.targetUID == "" && settings.request == "" &&
		settings.requestUID == "" && settings.phase == "" && settings.finding == "" {
		return settings, false, nil
	}
	for _, field := range []struct{ name, value string }{
		{"CONSOLE_TEST_KUBECONFIG", settings.kubeconfig},
		{"CONSOLE_TEST_CONTEXT", settings.contextName},
		{"CONSOLE_TEST_API_GROUP", string(settings.group)},
		{"CONSOLE_TEST_RESTORE_NAMESPACE", settings.namespace},
		{"CONSOLE_TEST_RESTORE_TARGET", settings.target},
		{"CONSOLE_TEST_RESTORE_TARGET_UID", settings.targetUID},
		{"CONSOLE_TEST_RESTORE_REQUEST", settings.request},
		{"CONSOLE_TEST_RESTORE_UID", settings.requestUID},
		{"CONSOLE_TEST_RESTORE_PHASE", settings.phase},
		{"CONSOLE_TEST_RESTORE_FINDING", settings.finding},
	} {
		if field.value == "" {
			return settings, true, fmt.Errorf("%s must be set for restore qualification", field.name)
		}
	}
	if settings.group != config.APIGroupKubeBao && settings.group != config.APIGroupOpenBao {
		return settings, true, errors.New("CONSOLE_TEST_API_GROUP must select kubebao.org or openbao.org")
	}
	if !slices.Contains([]string{"Pending", "Validating", "Running", "Completed", "Failed", "Unknown"}, settings.phase) {
		return settings, true, errors.New("CONSOLE_TEST_RESTORE_PHASE must select a supported reported phase")
	}
	if !slices.Contains([]string{"none", "info", "high", "critical"}, settings.finding) {
		return settings, true, errors.New("CONSOLE_TEST_RESTORE_FINDING must select none, info, high, or critical")
	}
	for _, identity := range []shared.Identity{
		{
			Origin: "restore-qualification", Group: string(settings.group), Kind: "OpenBaoCluster", Namespace: settings.namespace,
			Name: settings.target, UID: settings.targetUID,
		},
		{
			Origin: "restore-qualification", Group: string(settings.group), Kind: "OpenBaoRestore", Namespace: settings.namespace,
			Name: settings.request, UID: settings.requestUID,
		},
	} {
		if identity.Validate() != nil {
			return settings, true, errors.New("restore qualification requires valid declared identities")
		}
	}
	return settings, true, nil
}

// This opt-in test only reads a caller-provisioned target and request. The caller
// supplies a reader credential and exact expected identities; the test never
// creates a backup, restore request, or Kubernetes resource.
func TestNonemptyRestoreProjection(t *testing.T) {
	settings, enabled, err := restoreQualificationSettings(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Skip("set the CONSOLE_TEST_RESTORE_* identity tuple for nonempty restore qualification")
	}
	reader, err := New(config.Connection{
		ID: "restore-qualification", APIGroup: settings.group,
		Namespaces: []config.Namespace{{Name: settings.namespace, Environment: fleet.EnvironmentDevelopment}},
		Access:     config.Access{Mode: config.AccessKubeconfig, Kubeconfig: settings.kubeconfig, Context: settings.contextName},
	}, fleet.Policy{MaxBackupAge: 24 * time.Hour, StartupGrace: 10 * time.Minute})
	if err != nil {
		t.Fatal("could not configure the declared restore reader")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	verifyLiveRestoreAssociation(t, ctx, reader, settings)
	batch := reader.Collect(ctx, time.Now().UTC())
	if batch.Connection.Collection != fleet.ConnectionCollectionCurrent {
		t.Fatal("the declared restore namespace was not collected")
	}
	path := filepath.Join(t.TempDir(), "console.db")
	repository := openRestoreHistory(t, path)
	if err := repository.Apply(ctx, batch); err != nil {
		repository.Close()
		t.Fatal("could not persist the collected restore projection")
	}
	repository.Close()
	repository = openRestoreHistory(t, path)
	defer repository.Close()
	snapshot, err := repository.Snapshot(ctx)
	if err != nil {
		t.Fatal("could not read the retained restore projection after restart")
	}
	for _, deployment := range snapshot.Deployments {
		if deployment.Name != settings.target {
			continue
		}
		if deployment.UID != settings.targetUID || deployment.Recovery == nil || deployment.Recovery.RequestName != settings.request ||
			deployment.Recovery.Mechanism != fleet.RecoveryMechanismSnapshot ||
			deployment.Recovery.Phase != fleet.RecoveryPhase(strings.ToLower(settings.phase)) {
			t.Fatal("console presentation did not retain the declared UID-confirmed restore")
		}
		findings := 0
		for _, issue := range deployment.Issues {
			if issue.Rule == fleet.RuleRecoveryInvestigate && strings.HasPrefix(issue.Detail, "Restore "+settings.request+":") {
				findings++
				if settings.finding != "none" && issue.Severity != fleet.Severity(settings.finding) {
					t.Fatal("the declared restore finding has an unexpected console severity")
				}
			}
		}
		want := 1
		if settings.finding == "none" {
			want = 0
		}
		if findings != want {
			t.Fatal("console restore findings did not match the declared expectation")
		}
		return
	}
	t.Fatal("the declared restore target was not observed")
}

func verifyLiveRestoreAssociation(t *testing.T, ctx context.Context, reader *Reader, settings restoreQualification) {
	t.Helper()
	clusters, err := reader.shared.Clusters(ctx, reader.request(settings.namespace, "openbaoclusters"))
	if err != nil || clusters.Collection.Outcome != shared.CollectionComplete {
		t.Fatal("could not verify the declared target through its enrolled cluster LIST")
	}
	requests, err := reader.shared.Restores(ctx, reader.request(settings.namespace, "openbaorestores"))
	if err != nil || requests.Collection.Outcome != shared.CollectionComplete {
		t.Fatal("could not verify the declared request through its enrolled restore LIST")
	}
	for _, cluster := range clusters.Observations {
		if cluster.Reference.Identity.Name != settings.target || cluster.Reference.Identity.UID != settings.targetUID {
			continue
		}
		for _, request := range requests.Observations {
			if request.Reference.Identity.Name != settings.request || request.Reference.Identity.UID != settings.requestUID {
				continue
			}
			if request.Phase.State != shared.ValueReported || *request.Phase.Value != settings.phase {
				t.Fatal("the declared restore request does not report the expected phase")
			}
			association, err := adapter.AssociateRestore(cluster, request)
			if err != nil || association.State != shared.AssociationConfirmed {
				t.Fatal("the declared request has no confirmed UID association with the declared target")
			}
			return
		}
	}
	t.Fatal("the declared restore target and request identities were not observed together")
}

func TestRestoreQualificationRequiresCompleteDeclaredIdentities(t *testing.T) {
	values := map[string]string{
		"CONSOLE_TEST_KUBECONFIG": "reader.yaml", "CONSOLE_TEST_CONTEXT": "qualification", "CONSOLE_TEST_API_GROUP": "kubebao.org",
		"CONSOLE_TEST_RESTORE_NAMESPACE": "team", "CONSOLE_TEST_RESTORE_TARGET": "bao", "CONSOLE_TEST_RESTORE_TARGET_UID": "target-uid",
		"CONSOLE_TEST_RESTORE_REQUEST": "restore", "CONSOLE_TEST_RESTORE_UID": "request-uid", "CONSOLE_TEST_RESTORE_PHASE": "Unknown",
		"CONSOLE_TEST_RESTORE_FINDING": "high",
	}
	getenv := func(name string) string { return values[name] }
	if _, enabled, err := restoreQualificationSettings(getenv); !enabled || err != nil {
		t.Fatal("complete declared identities were rejected")
	}
	for _, field := range []string{
		"CONSOLE_TEST_CONTEXT", "CONSOLE_TEST_API_GROUP", "CONSOLE_TEST_RESTORE_UID", "CONSOLE_TEST_RESTORE_TARGET_UID",
		"CONSOLE_TEST_RESTORE_FINDING",
	} {
		value := values[field]
		delete(values, field)
		if _, enabled, err := restoreQualificationSettings(getenv); !enabled || err == nil {
			t.Fatalf("missing %s did not fail the opt-in identity guard", field)
		}
		values[field] = value
	}
	for _, field := range []string{"CONSOLE_TEST_API_GROUP", "CONSOLE_TEST_RESTORE_PHASE", "CONSOLE_TEST_RESTORE_FINDING"} {
		value := values[field]
		values[field] = "unsupported"
		if _, enabled, err := restoreQualificationSettings(getenv); !enabled || err == nil {
			t.Fatalf("unsupported %s did not fail the opt-in guard", field)
		}
		values[field] = value
	}
	if _, enabled, err := restoreQualificationSettings(func(string) string { return "" }); enabled || err != nil {
		t.Fatal("an unconfigured qualification did not remain opt-in")
	}
}
