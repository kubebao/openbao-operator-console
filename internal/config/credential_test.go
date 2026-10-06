package config

import "testing"

func TestImportedKubeconfigRejectsExecutableAndFileCredentials(t *testing.T) {
	base := `apiVersion: v1
kind: Config
current-context: reader
clusters:
- name: cluster
  cluster:
    server: https://api.example
contexts:
- name: reader
  context:
    cluster: cluster
    user: reader
users:
- name: reader
  user:
    token: reader-token
`
	credential, err := ImportKubeconfig([]byte(base), "")
	if err != nil || credential.Token != "reader-token" {
		t.Fatal("embedded reader rejected")
	}
	for _, extra := range []string{"    exec:\n      apiVersion: client.authentication.k8s.io/v1\n      command: malicious\n", "    tokenFile: /etc/passwd\n", "    client-key: /private/key\n", "    as: administrator\n"} {
		if _, err := ImportKubeconfig([]byte(base+extra), ""); err == nil {
			t.Fatal("unsafe reader configuration accepted")
		}
	}
	if _, err := ImportKubeconfig([]byte(base+"- name: unused\n  user:\n    exec:\n      command: malicious\n"), ""); err == nil {
		t.Fatal("unused executable context accepted")
	}
}
func TestGroupBindingsNeverGrantUnenrolledNamespaces(t *testing.T) {
	c := Config{Connections: []Connection{{ID: "cluster", Namespaces: []Namespace{{Name: "team"}}}}, Groups: []Group{{Name: "platform", Scopes: []Scope{{ConnectionID: "cluster", Namespaces: []string{"team", "hidden"}}}}}}
	user, allowed := c.Resolve("subject", []string{"platform"})
	if !allowed || len(user.Scopes) != 1 || len(user.Scopes[0].Namespaces) != 1 || user.Scopes[0].Namespaces[0] != "team" {
		t.Fatal("group access escaped enrollment")
	}
	if _, allowed := c.Resolve("subject", []string{"unmapped"}); allowed {
		t.Fatal("unmapped identity enrolled")
	}
}
