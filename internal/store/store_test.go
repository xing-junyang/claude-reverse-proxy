package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/local/claude-relay/internal/cryptox"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	vault, err := cryptox.NewVault(dir, "test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	data, err := Open(dir+"/test.db", vault)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	return data
}

func TestAccountKeyAndUsageLifecycle(t *testing.T) {
	ctx := context.Background()
	data := testStore(t)
	created, err := data.BootstrapAdmin(ctx, "admin", "strong bootstrap password")
	if err != nil || !created {
		t.Fatalf("bootstrap admin: created=%v err=%v", created, err)
	}
	user, err := data.AuthenticateAdmin(ctx, "admin", "strong bootstrap password")
	if err != nil || user.Username != "admin" {
		t.Fatalf("authenticate admin: user=%+v err=%v", user, err)
	}

	account, err := data.CreateAccount(ctx, AccountInput{
		Name: "primary", BaseURL: "https://api.anthropic.com", APIKey: "sk-ant-upstream-secret",
		Enabled: true, Weight: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(account.KeyHint, "upstream") {
		t.Fatalf("key hint leaks secret: %q", account.KeyHint)
	}
	secret, err := data.GetAccountSecret(ctx, account.ID)
	if err != nil || secret.APIKey != "sk-ant-upstream-secret" {
		t.Fatalf("account secret: secret=%q err=%v", secret.APIKey, err)
	}
	var encrypted string
	if err := data.db.QueryRow(`SELECT api_key_enc FROM accounts WHERE id = ?`, account.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, "sk-ant-upstream-secret") {
		t.Fatal("upstream API key stored in plaintext")
	}

	key, err := data.CreateAPIKey(ctx, APIKeyInput{Name: "client", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := data.AuthenticateAPIKey(ctx, key.Secret)
	if err != nil || authenticated.ID != key.ID {
		t.Fatalf("authenticate key: key=%+v err=%v", authenticated, err)
	}
	if _, err := data.AuthenticateAPIKey(ctx, "invalid"); err != ErrNotFound {
		t.Fatalf("invalid key error = %v", err)
	}

	err = data.RecordUsage(ctx, UsageInsert{
		RequestID: "req_test", APIKeyID: key.ID, AccountID: account.ID, Model: "claude-sonnet-test",
		Endpoint: "messages", StatusCode: 200, InputTokens: 100, OutputTokens: 50,
		CostUSD: .001, DurationMS: 120, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	dashboard, err := data.Dashboard(ctx, 24)
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Requests.Value != 1 || dashboard.Tokens.Value != 150 {
		t.Fatalf("unexpected dashboard: %+v", dashboard)
	}
	logs, err := data.ListUsage(ctx, UsageFilter{Limit: 10})
	if err != nil || logs.Total != 1 || logs.Items[0].APIKeyName != "client" {
		t.Fatalf("unexpected logs: %+v err=%v", logs, err)
	}
}
