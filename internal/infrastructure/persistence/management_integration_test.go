package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	m "olixops-permission-center/internal/domain/management"
)

// This read-only test needs the documented seven-table schema and can run
// against an isolated native PostgreSQL fixture. It never creates/drops tables.
func TestNativeManagementListQueries(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_DSN to an isolated PostgreSQL fixture")
	}
	db, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal("cannot connect to test PostgreSQL")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := NewManagementStore(db)
	err = store.Read(ctx, func(tx m.Tx) error {
		f := m.Filter{PageNum: 1, PageSize: 20}
		scope := m.Scope{Global: true}
		if _, e := tx.Users(f); e != nil {
			return e
		}
		if _, e := tx.Tenants(f, scope); e != nil {
			return e
		}
		if _, e := tx.Systems(f, scope); e != nil {
			return e
		}
		if _, e := tx.Audits(f, scope); e != nil {
			return e
		}
		f.TenantID = "ffffffff-ffff-4fff-8fff-ffffffffffff"
		f.SystemID = f.TenantID
		members, e := tx.Members(f)
		if e != nil {
			return e
		}
		if members.List == nil {
			t.Error("empty member list must be allocated")
		}
		credentials, e := tx.Credentials(f)
		if e != nil {
			return e
		}
		if credentials.List == nil {
			t.Error("empty credential list must be allocated")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("management list/count query failed: %v", err)
	}
}
