package secret

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestColumnBackfillTargetsNonEmpty(t *testing.T) {
	targets := ColumnBackfillTargets()
	if len(targets) < 5 {
		t.Fatalf("targets=%d", len(targets))
	}
	arr := ArrKeyBackfillTargets()
	if len(arr) != 2 {
		t.Fatalf("arr=%d", len(arr))
	}
	empty := BackfillTarget{}
	if empty.keyExpr() != "id" || empty.aadExpr() != "id" {
		t.Fatalf("defaults: key=%q aad=%q", empty.keyExpr(), empty.aadExpr())
	}
	withKey := BackfillTarget{KeyExpr: "provider_name"}
	if withKey.aadExpr() != "provider_name" {
		t.Fatalf("aad fallback: %q", withKey.aadExpr())
	}
	withAAD := BackfillTarget{KeyExpr: "id::text", AADExpr: "custom"}
	if withAAD.aadExpr() != "custom" {
		t.Fatalf("aad explicit: %q", withAAD.aadExpr())
	}
}

type errQueryExec struct{}

func (errQueryExec) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("query boom")
}
func (errQueryExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("exec boom")
}

type scanFailRows struct{ fakeRows }

func (s *scanFailRows) Scan(...any) error { return errors.New("scan boom") }

type scanFailExec struct{}

func (scanFailExec) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &scanFailRows{fakeRows: fakeRows{rows: [][]string{{"k", "a", "v"}}}}, nil
}
func (scanFailExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type execFailExec struct {
	rows [][]string
}

func (e *execFailExec) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &fakeRows{rows: e.rows}, nil
}
func (e *execFailExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("update boom")
}

type resolveFailExec struct {
	rows [][]string
}

func (e *resolveFailExec) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &fakeRows{rows: e.rows}, nil
}
func (e *resolveFailExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func TestBackfillErrorPaths(t *testing.T) {
	c := newTestCipher(t)
	target := BackfillTarget{Table: "t", Column: "c", KeyExpr: "id"}

	n, err := BackfillColumns(context.Background(), errQueryExec{}, c, []BackfillTarget{target})
	if n != 0 || err == nil {
		t.Fatalf("query fail: n=%d err=%v", n, err)
	}
	n, err = BackfillColumns(context.Background(), scanFailExec{}, c, []BackfillTarget{target})
	if n != 0 || err == nil {
		t.Fatalf("scan fail: n=%d err=%v", n, err)
	}
	n, err = BackfillColumns(context.Background(), &execFailExec{rows: [][]string{{"1", "1", "plain"}}}, c, []BackfillTarget{target})
	if n != 0 || err == nil {
		t.Fatalf("exec fail: n=%d err=%v", n, err)
	}

	n, err = BackfillReferencedColumns(context.Background(), errQueryExec{}, c, func(context.Context, string) (string, error) {
		return "", nil
	}, []BackfillTarget{target})
	if n != 0 || err == nil {
		t.Fatalf("ref query fail: n=%d err=%v", n, err)
	}

	n, err = BackfillReferencedColumns(context.Background(), &resolveFailExec{rows: [][]string{{"1", "ref"}}}, c,
		func(context.Context, string) (string, error) { return "", errors.New("resolve boom") },
		[]BackfillTarget{target},
	)
	if n != 0 || err == nil {
		t.Fatalf("resolve fail: n=%d err=%v", n, err)
	}

	n, err = BackfillReferencedColumns(context.Background(), &execFailExec{rows: [][]string{{"1", "literal"}}}, c,
		func(context.Context, string) (string, error) { return "", nil },
		[]BackfillTarget{target},
	)
	if n != 0 || err == nil {
		t.Fatalf("ref exec fail: n=%d err=%v", n, err)
	}
}

func TestDecryptMalformedBase64(t *testing.T) {
	c := newTestCipher(t)
	if _, err := c.Decrypt("enc:v1:!!!", "aad"); err == nil {
		t.Fatal("expected decode error")
	}
}
