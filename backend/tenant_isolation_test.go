package main

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// Regression tests for the multi-tenant isolation fixes in RuntimeSkillStore.
//
// Before this change the HTTP handlers called the unscoped Get/List/Search
// methods, so any authenticated user could read and delete every other
// tenant's runtime skills (which embed prompts, steps and env var names).

// Schema mirrors db_stores.go's production DDL exactly. The DATETIME column
// types matter: go-sqlite3 only parses them into time.Time when declared, and
// the store scans created_at/updated_at/last_used/last_patched into time.Time.
func newRuntimeSkillTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE app_runtime_skills (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			trigger_text TEXT DEFAULT '',
			steps TEXT DEFAULT '[]',
			tools TEXT DEFAULT '[]',
			env_vars TEXT DEFAULT '{}',
			prompt TEXT DEFAULT '',
			state TEXT DEFAULT 'active',
			version INTEGER DEFAULT 1,
			created_by TEXT DEFAULT 'runtime',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			use_count INTEGER DEFAULT 0,
			patch_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			last_used DATETIME,
			last_patched DATETIME,
			source_session TEXT DEFAULT ''
		);
		CREATE TABLE app_users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE,
			password TEXT,
			nickname TEXT,
			avatar TEXT,
			role TEXT,
			created_at TEXT
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRuntimeSkillStore_GetForUser_IsolatesTenants(t *testing.T) {
	store := NewRuntimeSkillStore(newRuntimeSkillTestDB(t))

	for _, sk := range []*RuntimeSkill{
		{ID: "rtskill-alice", UserID: "user-alice", Name: "alice skill", State: "active", Trigger: "alice trigger", Prompt: "alice secret prompt"},
		{ID: "rtskill-bob", UserID: "user-bob", Name: "bob skill", State: "active", Trigger: "bob trigger", Prompt: "bob secret prompt"},
	} {
		if err := store.Create(sk); err != nil {
			t.Fatalf("create %s: %v", sk.ID, err)
		}
	}

	// Owner can read their own skill.
	if got := store.GetForUser("rtskill-alice", "user-alice"); got == nil {
		t.Fatal("owner should be able to read their own skill")
	}

	// A different tenant must get nil — this is the IDOR the fix closes.
	if got := store.GetForUser("rtskill-alice", "user-bob"); got != nil {
		t.Fatalf("cross-tenant read leak: user-bob got alice's skill %q (prompt=%q)", got.Name, got.Prompt)
	}
	if got := store.GetForUser("rtskill-bob", "user-alice"); got != nil {
		t.Fatalf("cross-tenant read leak: user-alice got bob's skill %q", got.Name)
	}

	// Unknown id must stay nil.
	if got := store.GetForUser("rtskill-nope", "user-alice"); got != nil {
		t.Fatal("unknown id should return nil")
	}
}

func TestRuntimeSkillStore_ListByUser_ExcludesOtherTenants(t *testing.T) {
	store := NewRuntimeSkillStore(newRuntimeSkillTestDB(t))

	for _, sk := range []*RuntimeSkill{
		{ID: "rtskill-a1", UserID: "user-alice", Name: "a1", State: "active"},
		{ID: "rtskill-a2", UserID: "user-alice", Name: "a2", State: "pinned"},
		{ID: "rtskill-b1", UserID: "user-bob", Name: "b1", State: "active"},
	} {
		if err := store.Create(sk); err != nil {
			t.Fatalf("create %s: %v", sk.ID, err)
		}
	}

	alice := store.ListByUser("user-alice")
	if len(alice) != 2 {
		t.Fatalf("expected 2 skills for alice, got %d", len(alice))
	}
	for _, sk := range alice {
		if sk.UserID != "user-alice" {
			t.Fatalf("ListByUser leaked another tenant's skill: %s owned by %s", sk.ID, sk.UserID)
		}
	}

	// The unscoped List still returns everything (used by the internal engine
	// activation path) — document that so nobody swaps it back in by accident.
	if all := store.List(); len(all) != 3 {
		t.Fatalf("expected 3 skills unscoped, got %d", len(all))
	}
}

func TestRuntimeSkillStore_UserScopedFilters(t *testing.T) {
	store := NewRuntimeSkillStore(newRuntimeSkillTestDB(t))

	for _, sk := range []*RuntimeSkill{
		{ID: "rtskill-a-active", UserID: "user-alice", Name: "deploy", State: "active", Tools: []string{"exec"}},
		{ID: "rtskill-a-arch", UserID: "user-alice", Name: "deploy old", State: "archived", Tools: []string{"exec"}},
		{ID: "rtskill-b-active", UserID: "user-bob", Name: "deploy", State: "active", Tools: []string{"exec"}},
	} {
		if err := store.Create(sk); err != nil {
			t.Fatalf("create %s: %v", sk.ID, err)
		}
	}

	// State filter
	got := store.ListByStateForUser("active", "user-alice")
	if len(got) != 1 || got[0].ID != "rtskill-a-active" {
		t.Fatalf("ListByStateForUser(alice, active) = %v, want only rtskill-a-active", ids(got))
	}

	// Tool filter
	got = store.ListByToolForUser("exec", "user-alice")
	if len(got) != 2 {
		t.Fatalf("ListByToolForUser(alice, exec) returned %v, want 2", ids(got))
	}
	for _, sk := range got {
		if sk.UserID != "user-alice" {
			t.Fatalf("ListByToolForUser leaked %s owned by %s", sk.ID, sk.UserID)
		}
	}

	// Search
	got = store.SearchForUser("deploy", "user-alice")
	if len(got) != 2 {
		t.Fatalf("SearchForUser(alice, deploy) returned %v, want 2", ids(got))
	}
	for _, sk := range got {
		if sk.UserID != "user-alice" {
			t.Fatalf("SearchForUser leaked %s owned by %s", sk.ID, sk.UserID)
		}
	}
}

func TestUserStore_CreateInternal_NeverClaimsAdminSlot(t *testing.T) {
	db := newRuntimeSkillTestDB(t)
	store := &UserStore{db: db}

	// A gateway-provisioned account arrives first — it must NOT become admin.
	svc, err := store.CreateInternal("feishu_attacker", "random-secret", "Attacker")
	if err != nil {
		t.Fatalf("CreateInternal: %v", err)
	}
	if svc.Role != "user" {
		t.Fatalf("gateway account must never be admin, got role=%q", svc.Role)
	}

	// The first real web signup must still get the admin bootstrap slot even
	// though a service account already exists.
	admin, err := store.Create("realowner", "hunter2", "Real Owner")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if admin.Role != "admin" {
		t.Fatalf("first web user should be admin, got role=%q", admin.Role)
	}

	// A second web user must not also become admin.
	second, err := store.Create("member", "hunter2", "Member")
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if second.Role != "user" {
		t.Fatalf("second web user should be plain user, got role=%q", second.Role)
	}

	// And a later gateway account still must not.
	svc2, err := store.CreateInternal("telegram_x", "random2", "X")
	if err != nil {
		t.Fatalf("CreateInternal second: %v", err)
	}
	if svc2.Role != "admin" && svc2.Role != "user" {
		t.Fatalf("unexpected role %q", svc2.Role)
	}
	if svc2.Role == "admin" {
		t.Fatal("gateway account must never be admin")
	}
}

func TestUserStore_CreateInternal_DuplicateStillRejected(t *testing.T) {
	db := newRuntimeSkillTestDB(t)
	store := &UserStore{db: db}

	if _, err := store.CreateInternal("feishu_x", "s1", "X"); err != nil {
		t.Fatalf("first CreateInternal: %v", err)
	}
	if _, err := store.CreateInternal("feishu_x", "s2", "X"); err == nil {
		t.Fatal("duplicate username must still be rejected")
	}
}

func ids(skills []*RuntimeSkill) []string {
	out := make([]string, 0, len(skills))
	for _, s := range skills {
		out = append(out, s.ID)
	}
	return out
}
