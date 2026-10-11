package postgres

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

// Forward a single local plaintext PostgreSQL session. The backend has
// already executed COMMIT when it emits CommandComplete("COMMIT"), so discard
// that actual server response and tear down the TCP connection before the
// client can receive a successful acknowledgement. This does not simulate
// the SQL operation or fabricate a backend commit.
func cutPostgresCommitAcknowledgement(listener net.Listener, backend string, outcome chan<- error) {
	client, err := listener.Accept()
	if err != nil {
		outcome <- fmt.Errorf("accept controlled PostgreSQL connection: %w", err)
		return
	}
	defer client.Close()
	server, err := net.DialTimeout("tcp", backend, 5*time.Second)
	if err != nil {
		outcome <- fmt.Errorf("dial isolated PostgreSQL backend: %w", err)
		return
	}
	defer server.Close()
	go func() {
		_, _ = io.Copy(server, client)
	}()
	for {
		var header [5]byte
		if _, err := io.ReadFull(server, header[:]); err != nil {
			outcome <- fmt.Errorf("read PostgreSQL backend frame: %w", err)
			return
		}
		length := int(binary.BigEndian.Uint32(header[1:]))
		if length < 4 || length > (16<<20) {
			outcome <- fmt.Errorf("unexpected PostgreSQL backend frame length %d", length)
			return
		}
		body := make([]byte, length-4)
		if _, err := io.ReadFull(server, body); err != nil {
			outcome <- fmt.Errorf("read PostgreSQL backend message: %w", err)
			return
		}
		if header[0] == 'C' && bytes.Equal(body, []byte("COMMIT\x00")) {
			// Fail precisely after the server's COMMIT CommandComplete,
			// before sending it to pgx. Both directions are closed on
			// return. There is no retry or fabricated rollback.
			outcome <- nil
			return
		}
		frame := append(header[:], body...)
		for len(frame) != 0 {
			n, err := client.Write(frame)
			if err != nil {
				outcome <- fmt.Errorf("forward PostgreSQL response: %w", err)
				return
			}
			if n == 0 {
				outcome <- io.ErrShortWrite
				return
			}
			frame = frame[n:]
		}
	}
}

func TestRealTCPDropAfterServerCommitRequiresAuthoritativeRecoveryReadback(t *testing.T) {
	// This creates a random private PostgreSQL schema and cleans it up.
	// CI uses localhost postgres:17 with sslmode=disable. Production
	// connections and external Providers are never accessed here.
	original := newIsolatedIntegrationStore(t)
	config := original.pool.Config()
	host := config.ConnConfig.Host
	ip := net.ParseIP(host)
	if config.ConnConfig.TLSConfig != nil ||
		(host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		t.Skip("real TCP ACK cut is restricted to a local plaintext PostgreSQL test server")
	}
	if !strings.HasPrefix(config.ConnConfig.RuntimeParams["search_path"], "ep_test_") {
		t.Fatal("refuse a lost-ACK fault outside a private PostgreSQL test schema")
	}
	backend := net.JoinHostPort(host, strconv.Itoa(int(config.ConnConfig.Port)))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	outcome := make(chan error, 1)
	go cutPostgresCommitAcknowledgement(listener, backend, outcome)

	// Use exactly the same isolated schema, only changing the client TCP
	// dial target. All original PostgreSQL audit/epoch checks still run.
	config.MaxConns = 1
	config.MinConns = 0
	config.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "tcp", listener.Addr().String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	throughProxy := New(pool)
	if _, err := throughProxy.BeginRecoveryContext(ctx, 0); !errors.Is(err, corestore.ErrCommitOutcomeUnknown) {
		t.Fatalf("real post-COMMIT TCP failure was not classified UNKNOWN: %v", err)
	}
	select {
	case err := <-outcome:
		if err != nil {
			t.Fatalf("proxy did not discard the real server COMMIT acknowledgement: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy never observed the backend's committed transaction")
	}

	observed, err := original.GetRecoveryContext(ctx)
	if err != nil || observed.Epoch != 1 || observed.Mode != recovery.RecoveryReconciliation {
		t.Fatalf("original transaction was not durably committed: state=%+v err=%v", observed, err)
	}
	var auditCount int
	if err := original.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='recovery.started' AND aggregate_id='singleton'",
	).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("original immutable audit was not retained once: count=%d err=%v", auditCount, err)
	}
	if _, err := original.BeginRecoveryContext(ctx, 0); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("old-epoch replay was not rejected after the lost COMMIT acknowledgement: %v", err)
	}
}
