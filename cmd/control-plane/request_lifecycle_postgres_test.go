package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/access"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

// The lock is released only AFTER the still-connected client receives a
// non-success response. pg_stat_activity proves a real SQL lock wait, rather
// than guessing that the request reached PostgreSQL from elapsed wall time.
// Local PostgreSQL and ephemeral mTLS only; the watchdog is not a host SLO.
func TestControlLifecycleServiceStopCancelsActualPostgresWait(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_URL is not configured")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	commandOK(t, err)
	host := cfg.ConnConfig.Host
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("lifecycle fault requires a loopback PostgreSQL test server")
	}
	setup, cancelSetup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelSetup()
	admin, err := pgxpool.NewWithConfig(setup, cfg.Copy())
	commandOK(t, err)
	defer admin.Close()
	var nonce [12]byte
	_, err = rand.Read(nonce[:])
	commandOK(t, err)
	schema := "ep_http_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	_, err = admin.Exec(setup, "CREATE SCHEMA "+quoted)
	commandOK(t, err)
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := admin.Exec(clean, "DROP SCHEMA "+quoted+" CASCADE")
		if err != nil {
			t.Errorf("clean isolated lifecycle schema: %v", err)
		}
	}()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["application_name"] = schema
	pool, err := pgxpool.NewWithConfig(setup, cfg)
	commandOK(t, err)
	store := pgstore.New(pool)
	defer store.Close()
	commandOK(t, store.ApplyCoreMigration(setup))
	lock, err := admin.Begin(setup)
	commandOK(t, err)
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = lock.Rollback(clean)
	}()
	_, err = lock.Exec(setup, "LOCK TABLE "+quoted+".work_items IN ACCESS EXCLUSIVE MODE")
	commandOK(t, err)
	const subject = "urn:engineering-platform:engineer:service-stop"
	policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{{Subject: subject, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate}}}})
	commandOK(t, err)
	pki := testsupport.NewPKI(t)
	server, err := assembleServer(configuration{policy: policy, tls: pki.ServerTLS()}, store)
	commandOK(t, err)
	serving, stop := context.WithCancel(setup)
	defer stop()
	listener := lifecycleListener(t)
	done := make(chan error, 1)
	go func() { done <- runControlServer(serving, server, listener, false, nil) }()
	joined := false
	defer func() {
		stop()
		if !joined {
			_ = server.Close()
			_ = lifecycleResult(t, done)
		}
	}()
	client := pki.Client(t, subject)
	clientCtx, cancelClient := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelClient()
	raw := []byte(`{"work_item_id":"service-stop-work","title":"blocked work","human_owner":"` + subject + `"}`)
	request, err := http.NewRequestWithContext(clientCtx, http.MethodPost, "https://"+listener.Addr().String()+"/api/v1/work-items", bytes.NewReader(raw))
	commandOK(t, err)
	request.Header.Set("Content-Type", "application/json")
	type result struct {
		status int
		err    error
	}
	response := make(chan result, 1)
	go func() {
		r, err := client.Do(request)
		if err != nil {
			response <- result{err: err}
			return
		}
		_, err = io.Copy(io.Discard, r.Body)
		response <- result{r.StatusCode, errors.Join(err, r.Body.Close())}
	}()
	observe, stopObserve := context.WithTimeout(setup, 5*time.Second)
	defer stopObserve()
	for {
		var blocked bool
		err := admin.QueryRow(observe, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND state='active')", schema).Scan(&blocked)
		commandOK(t, err)
		if blocked {
			break
		}
		select {
		case <-observe.Done():
			t.Fatal("request never reached actual PostgreSQL lock wait")
		case <-time.After(5 * time.Millisecond):
		}
	}
	stop()
	var got result
	select {
	case got = <-response:
	case <-time.After(2 * time.Second):
		t.Error("service cancellation did not stop PostgreSQL wait while its lock remained held")
		cancelClient()
		select {
		case got = <-response:
		case <-time.After(5 * time.Second):
			t.Fatal("client cancellation could not release failed fixture")
		}
	}
	if got.err != nil || got.status < 400 {
		t.Errorf("no complete non-success response before lock release: %+v", got)
	}
	if err := lifecycleResult(t, done); err != nil {
		t.Errorf("service failed to drain cancelled DB request: %v", err)
	}
	joined = true
	commandOK(t, lock.Rollback(setup))
	var works, events, operations int
	query := "SELECT (SELECT count(*) FROM " + quoted + ".work_items),(SELECT count(*) FROM " + quoted + ".audit_events),(SELECT count(*) FROM " + quoted + ".external_operations)"
	commandOK(t, admin.QueryRow(setup, query).Scan(&works, &events, &operations))
	if works != 0 || events != 0 || operations != 0 {
		t.Fatalf("cancelled locked request left authority: work=%d audit=%d operations=%d", works, events, operations)
	}
}
