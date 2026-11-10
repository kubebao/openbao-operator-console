package httpapi

import (
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
)

const origin = "https://console.example"

func (c localConsole) request(method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	return setupRequest(c.handler, method, path, payload, origin, cookies...)
}

func (c localConsole) revision(t *testing.T) int64 {
	t.Helper()
	_, revision := c.service.Current()
	return revision
}

// enroll opens link and completes enrollment as a new user of the console.
func (c localConsole) enroll(t *testing.T, link contract.LocalAccountEnrollment, password string) (*http.Cookie, []byte) {
	t.Helper()
	token, found := strings.CutPrefix(link.EnrollmentUrl, origin+"/enroll#token=")
	if !found {
		t.Fatalf("unexpected enrollment URL %q", link.EnrollmentUrl)
	}
	opened := c.request("POST", "/api/v1/enrollment", contract.EnrollmentToken{Token: token})
	if opened.Code != http.StatusOK {
		t.Fatalf("open enrollment = %d: %s", opened.Code, opened.Body.String())
	}
	start := decodeJSON[contract.EnrollmentStart](t, opened.Body)
	secret, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(start.Authenticator.Key)
	if !strings.Contains(start.Authenticator.Uri, "Console:"+start.Username+"@console.example") {
		t.Fatalf("authenticator label: %s", start.Authenticator.Uri)
	}
	completed := c.request("POST", "/api/v1/enrollment/complete", contract.EnrollmentInput{
		Token: token, Password: password, Code: localauth.TOTPCode(secret, c.clock.Now()),
	})
	if completed.Code != http.StatusOK {
		t.Fatalf("complete enrollment = %d: %s", completed.Code, completed.Body.String())
	}
	if again := c.request("POST", "/api/v1/enrollment", contract.EnrollmentToken{Token: token}); again.Code != http.StatusUnauthorized {
		t.Fatalf("reused link = %d", again.Code)
	}
	return sessionCookie(completed.Result()), secret
}

func TestAdministratorsManageLocalAccountsThroughEnrollmentLinks(t *testing.T) {
	console := localSetup(t)
	created := console.request("POST", "/api/v1/settings/local-accounts", contract.LocalAccountInput{
		Revision: console.revision(t), Username: "reader", Name: "Reader", Scopes: []contract.Scope{},
	}, console.session)
	if created.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	link := decodeJSON[contract.LocalAccountEnrollment](t, created.Body)
	view := decodeJSON[contract.Settings](t, console.request("GET", "/api/v1/settings", nil, console.session).Body)
	if len(view.LocalAccounts) != 2 || view.LocalAccounts[1].State != contract.EnrollmentPending ||
		view.LocalAccounts[1].EnrollmentExpiresAt == nil || !view.LocalSignIn {
		t.Fatalf("settings view: %+v", view.LocalAccounts)
	}
	session, secret := console.enroll(t, link, "copper kettle morning tide")
	if session == nil {
		t.Fatal("enrollment did not sign the account in")
	}
	snapshot := console.request("GET", "/api/v1/snapshot", nil, session)
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"localAccount":true`) ||
		strings.Contains(snapshot.Body.String(), `"admin":true`) {
		t.Fatalf("snapshot as the new reader = %d: %s", snapshot.Code, snapshot.Body.String())
	}
	if forbidden := console.request("GET", "/api/v1/settings", nil, session); forbidden.Code != http.StatusForbidden {
		t.Fatalf("reader read settings = %d", forbidden.Code)
	}
	updated := console.request("PUT", "/api/v1/settings/local-accounts/"+link.AccountId, contract.LocalAccountUpdate{
		Revision: console.revision(t), Name: "Promoted", Admin: true, Scopes: []contract.Scope{},
	}, console.session)
	if updated.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", updated.Code, updated.Body.String())
	}
	if settings := console.request("GET", "/api/v1/settings", nil, session); settings.Code != http.StatusOK {
		t.Fatal("promotion did not apply to the open session")
	}
	// A reset revokes the password, authenticator, and open sessions at once.
	reset := console.request("POST", "/api/v1/settings/local-accounts/"+link.AccountId+"/enrollment", nil, console.session)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset = %d: %s", reset.Code, reset.Body.String())
	}
	if after := console.request("GET", "/api/v1/snapshot", nil, session); after.Code != http.StatusUnauthorized {
		t.Fatalf("session after reset = %d", after.Code)
	}
	console.clock.step()
	if old := console.signIn(
		"reader",
		"copper kettle morning tide",
		localauth.TOTPCode(secret, console.clock.Now()),
		origin,
	); old.Code != http.StatusUnauthorized {
		t.Fatalf("old credential after reset = %d", old.Code)
	}
	console.enroll(t, decodeJSON[contract.LocalAccountEnrollment](t, reset.Body), "silver orchard winter path")
	deleted := console.request(
		"DELETE",
		"/api/v1/settings/local-accounts/"+link.AccountId+"?revision="+strconv.FormatInt(console.revision(t), 10),
		nil,
		console.session,
	)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", deleted.Code, deleted.Body.String())
	}
	if _, err := console.local.Store.Load(t.Context(), link.AccountId); err == nil {
		t.Fatal("the removed account kept its credential")
	}
	history := decodeJSON[contract.Settings](t, console.request("GET", "/api/v1/settings", nil, console.session).Body).Changes
	summaries := []string{}
	for _, change := range history {
		summaries = append(summaries, change.Actor+": "+change.Summary)
	}
	joined := strings.Join(summaries, "\n")
	for _, want := range []string{
		"local:admin: Added local account reader",
		"local:admin: Updated local account reader",
		"local:admin: Removed local account reader",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("change log lacks %q:\n%s", want, joined)
		}
	}
}

func TestLocalAccountManagementRejectsInvalidRequests(t *testing.T) {
	console := localSetup(t)
	for name, response := range map[string]*httptest.ResponseRecorder{
		"duplicate username": console.request("POST", "/api/v1/settings/local-accounts", contract.LocalAccountInput{
			Revision: console.revision(t), Username: "admin", Scopes: []contract.Scope{},
		}, console.session),
		"uppercase username": console.request("POST", "/api/v1/settings/local-accounts", contract.LocalAccountInput{
			Revision: console.revision(t), Username: "Reader", Scopes: []contract.Scope{},
		}, console.session),
		"stale revision": console.request("POST", "/api/v1/settings/local-accounts", contract.LocalAccountInput{
			Revision: console.revision(t) - 1, Username: "reader", Scopes: []contract.Scope{},
		}, console.session),
		"last administrator": console.request("DELETE", "/api/v1/settings/local-accounts/"+accountID(t, console, "admin")+
			"?revision="+strconv.FormatInt(console.revision(t), 10), nil, console.session),
		"disable without provider": console.request("PUT", "/api/v1/settings/local-sign-in", contract.LocalSignInUpdate{
			Revision: console.revision(t), Enabled: false,
		}, console.session),
		"unknown reset": console.request("POST", "/api/v1/settings/local-accounts/missing/enrollment", nil, console.session),
		"unauthenticated": console.request("POST", "/api/v1/settings/local-accounts", contract.LocalAccountInput{
			Revision: console.revision(t), Username: "reader", Scopes: []contract.Scope{},
		}),
		"forged link": console.request("POST", "/api/v1/enrollment", contract.EnrollmentToken{
			Token: accountID(t, console, "admin") + "." + strings.Repeat("A", 43),
		}),
	} {
		if response.Code < 400 || response.Code >= 500 {
			t.Fatalf("%s = %d: %s", name, response.Code, response.Body.String())
		}
	}
}

func accountID(t *testing.T, console localConsole, username string) string {
	t.Helper()
	configuration, _ := console.service.Current()
	account, found := configuration.FindLocalAccount(username)
	if !found {
		t.Fatalf("account %s is not enrolled", username)
	}
	return account.ID
}

func TestLocalAccountsChangeTheirPasswordAndRecoveryCodes(t *testing.T) {
	console := localSetup(t)
	console.clock.step()
	input := contract.AccountPasswordInput{
		CurrentPassword: "wrong password value", NewPassword: "copper kettle morning tide",
		Code: localauth.TOTPCode(console.secret, console.clock.Now()),
	}
	if wrong := console.request("POST", "/api/v1/account/password", input, console.session); wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong current password = %d", wrong.Code)
	}
	input.CurrentPassword = localPassword
	other := console.signIn("admin", localPassword, console.codes[0], origin)
	otherSession := sessionCookie(other.Result())
	if changed := console.request("POST", "/api/v1/account/password", input, console.session); changed.Code != http.StatusNoContent {
		t.Fatalf("change password = %d: %s", changed.Code, changed.Body.String())
	}
	if kept := console.request("GET", "/api/v1/snapshot", nil, console.session); kept.Code != http.StatusOK {
		t.Fatal("the current session ended after a password change")
	}
	if ended := console.request("GET", "/api/v1/snapshot", nil, otherSession); ended.Code != http.StatusUnauthorized {
		t.Fatal("another session survived a password change")
	}
	replaced := console.request("POST", "/api/v1/account/recovery-codes", contract.AccountVerification{
		Password: "copper kettle morning tide", Code: console.codes[1],
	}, console.session)
	if replaced.Code != http.StatusOK ||
		len(decodeJSON[contract.LocalEnrollmentResult](t, replaced.Body).RecoveryCodes) != localauth.RecoveryCodeCount {
		t.Fatalf("replace recovery codes = %d: %s", replaced.Code, replaced.Body.String())
	}
	if old := console.signIn("admin", "copper kettle morning tide", console.codes[2], origin); old.Code != http.StatusUnauthorized {
		t.Fatal("a replaced recovery code still signed in")
	}
}

func TestDisabledLocalSignInClosesLocalEntryPoints(t *testing.T) {
	console := localSetup(t)
	configuration, revision := console.service.Current()
	configuration.SeedIdentity(config.Identity{Issuer: "https://idp.example", ClientID: "console"})
	configuration.GrantAdministrator("oidc-admin")
	configuration.LocalSignInDisabled = true
	if _, err := console.service.Save(t.Context(), revision, configuration, "test", "disable"); err != nil {
		t.Fatal(err)
	}
	if session := console.request("GET", "/api/v1/snapshot", nil, console.session); session.Code != http.StatusUnauthorized {
		t.Fatalf("local session after disabling = %d", session.Code)
	}
	if options := decodeJSON[contract.SignInOptions](t, console.request("GET", "/api/v1/sign-in", nil).Body); options.LocalAccounts {
		t.Fatal("sign-in options offered local accounts")
	}
	if signIn := console.signIn("admin", localPassword, console.codes[0], origin); signIn.Code != http.StatusForbidden {
		t.Fatalf("local sign-in while disabled = %d", signIn.Code)
	}
	if enrollment := console.request(
		"POST",
		"/api/v1/enrollment",
		contract.EnrollmentToken{Token: "x.y"},
	); enrollment.Code != http.StatusForbidden {
		t.Fatalf("enrollment while disabled = %d", enrollment.Code)
	}
}
