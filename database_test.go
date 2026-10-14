package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func databaseTestSetup(t *testing.T) (databaseTarget, databaseTarget) {
	t.Helper()
	closeDatabasePools()
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	t.Setenv("LOG_AGENT_SETTINGS_FILE", filepath.Join(t.TempDir(), "settings.json"))
	t.Setenv("LOG_AGENT_ADMIN_TOKEN", "")
	*configDirCache() = configDirState{}
	t.Cleanup(func() { closeDatabasePools(); *configDirCache() = configDirState{} })
	profiles := []databaseProfile{
		{ID: "east", Name: "华东", Engine: "sqlite", File: filepath.Join(configDir(), "east.db")},
		{ID: "west", Name: "华西", Engine: "sqlite", File: filepath.Join(configDir(), "west.db")},
		{ID: "broken", Name: "Unavailable", Engine: "sqlite", File: filepath.Join(configDir(), "missing", "broken.db")},
	}
	if err := writeJSONFile(databaseProfilesPath(), profiles); err != nil {
		t.Fatal(err)
	}
	a, b := databaseTarget{ID: "east", Database: "main", Schema: "main", Table: "orders"}, databaseTarget{ID: "west", Database: "main", Schema: "main", Table: "orders"}
	for _, target := range []databaseTarget{a, b} {
		pool, err := databases.pool(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		ddl := "CREATE TABLE orders (id INTEGER PRIMARY KEY, amount NUMERIC, note TEXT)"
		if target.ID == "west" {
			ddl = "CREATE TABLE orders (note TEXT, id INTEGER PRIMARY KEY, amount NUMERIC, currency TEXT)"
		}
		if _, err = pool.db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.db.Exec("INSERT INTO orders(id, amount, note) VALUES(1, 12.5, NULL), (9223372036854775807, 25, 'second')"); err != nil {
			t.Fatal(err)
		}
	}
	return a, b
}

func databaseRequest(t *testing.T, action string, target *databaseTarget, body any) *httptest.ResponseRecorder {
	t.Helper()
	method := "GET"
	var data []byte
	if body != nil {
		method = "POST"
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/database/" + action
	if target != nil {
		path += "?id=" + target.ID + "&database=" + target.Database + "&schema=" + target.Schema + "&table=" + target.Table
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	(&server{}).handleDatabase(w, r)
	return w
}
func databaseDecode[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if response.Code != 200 {
		t.Fatalf("database API HTTP %d: %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDatabaseFederatedQuery(t *testing.T) {
	a, b := databaseTestSetup(t)
	query := func(sql string, targets []databaseTarget, limit int) databaseQueryResponse {
		return databaseDecode[databaseQueryResponse](t, databaseRequest(t, "query", nil, map[string]any{"targets": targets, "sql": sql, "limit": limit}))
	}
	result := query("SELECT * FROM orders ORDER BY id", []databaseTarget{a, b, {ID: "broken", Database: "main"}}, 100)
	if len(result.Rows) != 4 || len(result.Columns) != 4 || len(result.Sources) != 3 || result.Sources[2].Error == "" {
		t.Fatalf("merged result = %+v", result)
	}
	for i, name := range []string{"id", "amount", "note", "currency"} {
		if result.Columns[i].Name != name {
			t.Fatalf("column %d = %q", i, result.Columns[i].Name)
		}
	}
	if result.Rows[0].Source != 0 || result.Rows[2].Source != 1 || result.Rows[2].Values[0] != "1" || result.Rows[0].Values[2] != nil || result.Rows[0].Values[3] != nil {
		t.Fatalf("alignment, provenance or NULL lost: %+v", result.Rows)
	}
	if result.Rows[1].Values[0] != "9223372036854775807" {
		t.Fatalf("integer precision lost: %v", result.Rows[1].Values[0])
	}
	if result.Sources[0].RowCount != 2 || len(result.Sources[0].Result.Rows) != 0 {
		t.Fatal("source rows should be counted but not duplicated in the response")
	}
	limited := query("SELECT id FROM orders ORDER BY id", []databaseTarget{a, b}, 1)
	if len(limited.Rows) != 2 || !limited.Sources[0].Result.Truncated || !limited.Sources[1].Result.Truncated {
		t.Fatalf("per-source limits = %+v", limited)
	}
	duplicate := query("SELECT id, id FROM orders WHERE id=1", []databaseTarget{a, b}, 100)
	if len(duplicate.Columns) != 2 || len(duplicate.Rows) != 2 || duplicate.Rows[1].Values[1] != "1" {
		t.Fatalf("duplicate aliases were collapsed: %+v", duplicate)
	}
	comment := query("-- dashboard query\nSELECT ';' AS literal", []databaseTarget{a}, 100)
	if len(comment.Rows) != 1 || comment.Rows[0].Values[0] != ";" {
		t.Fatal("statement scanner rejected a comment or quoted semicolon")
	}
	for _, text := range []string{"DELETE FROM orders", "WITH x AS (SELECT 1) DELETE FROM orders", "PRAGMA query_only=OFF; DELETE FROM orders"} {
		response := databaseRequest(t, "query", nil, map[string]any{"targets": []databaseTarget{a, b}, "sql": text})
		if response.Code == 200 {
			var value databaseQueryResponse
			_ = json.Unmarshal(response.Body.Bytes(), &value)
			if len(value.Sources) != 2 || value.Sources[0].Error == "" || value.Sources[1].Error == "" {
				t.Fatalf("write query was not rejected: %s", response.Body.String())
			}
		}
	}
	if got := query("SELECT count(*) AS n FROM orders", []databaseTarget{a, b}, 100); got.Rows[0].Values[0] != "2" || got.Rows[1].Values[0] != "2" {
		t.Fatal("query mode modified data")
	}
	databaseDecode[databaseQueryResponse](t, databaseRequest(t, "query", nil, map[string]any{"targets": []databaseTarget{a}, "sql": "UPDATE orders SET amount=99 WHERE id=1", "mode": "execute"}))
	if got := query("SELECT amount FROM orders WHERE id=1", []databaseTarget{a, b}, 100); got.Rows[0].Values[0] != "99" || got.Rows[1].Values[0] != "12.5" {
		t.Fatalf("single-target write or read-only cleanup failed: %+v", got.Rows)
	}
}

func TestDatabaseFederatedStatements(t *testing.T) {
	a, b := databaseTestSetup(t)
	run := func(text string, targets ...databaseTarget) databaseQueryResponse {
		t.Helper()
		return databaseDecode[databaseQueryResponse](t, databaseRequest(t, "query", nil, map[string]any{"targets": targets, "sql": text, "mode": "run", "limit": 1}))
	}
	assertAffected := func(result databaseQueryResponse, expected int64) {
		t.Helper()
		if result.Mode != "execute" || result.Affected != expected || len(result.Sources) != 2 {
			t.Fatalf("execution report: %+v", result)
		}
		for _, source := range result.Sources {
			if source.Error != "" || source.Affected == nil || !source.Executed {
				t.Fatalf("source report: %+v", source)
			}
		}
	}
	assertAffected(run("INSERT INTO orders(id, amount, note) VALUES(7, 10, 'new; value')", a, b), 2)
	assertAffected(run("-- selected statement\nUPDATE orders SET amount=amount+1 WHERE id=7", a, b), 2)
	assertAffected(run("WITH ids AS (SELECT 7 AS id) UPDATE orders SET note='cte' WHERE id IN (SELECT id FROM ids)", a, b), 2)
	returning := run("UPDATE orders SET amount=amount+2 RETURNING id, amount", a, b)
	assertAffected(returning, 6)
	if len(returning.Rows) != 2 || !returning.Sources[0].Result.Truncated || !returning.Sources[1].Result.Truncated {
		t.Fatalf("RETURNING results/limits: %+v", returning)
	}
	// A failed destination must not hide successful changes, including DDL.
	ddl := run("CREATE TABLE cross_ddl(id INTEGER PRIMARY KEY)", a, b, databaseTarget{ID: "broken", Database: "main"})
	if len(ddl.Sources) != 3 || ddl.Sources[0].Error != "" || ddl.Sources[1].Error != "" || ddl.Sources[2].Error == "" || ddl.Affected != 0 {
		t.Fatalf("DDL execution report: %+v", ddl)
	}
	assertAffected(run("ALTER TABLE cross_ddl ADD name TEXT", a, b), 0)
	run("CREATE TABLE partially_existing(id INT)", a)
	partial := run("CREATE TABLE partially_existing(id INT)", a, b)
	if partial.Sources[0].Error == "" || partial.Sources[1].Error != "" {
		t.Fatalf("partial DDL report: %+v", partial)
	}
	assertAffected(run("DELETE FROM orders WHERE id=7", a, b), 2)
	for _, target := range []databaseTarget{a, b} {
		pool, _ := databases.pool(context.Background(), target)
		var count int
		var value string
		if err := pool.db.QueryRow("SELECT CAST(amount AS TEXT) FROM orders WHERE id=1").Scan(&value); err != nil || value != "14.5" {
			t.Fatalf("write lost on %s: %q, %v", target.ID, value, err)
		}
		if err := pool.db.QueryRow("SELECT count(*) FROM orders WHERE id=7").Scan(&count); err != nil || count != 0 {
			t.Fatalf("delete lost on %s", target.ID)
		}
		if _, err := pool.db.Exec("INSERT INTO partially_existing(id) VALUES(1)"); err != nil {
			t.Fatalf("successful DDL was rolled back on %s: %v", target.ID, err)
		}
	}
	assertAffected(run("DROP TABLE cross_ddl", a, b), 0)
	for _, targets := range [][]databaseTarget{{a, a}, {a, {ID: a.ID}}} {
		response := databaseRequest(t, "query", nil, map[string]any{"targets": targets, "sql": "UPDATE orders SET amount=amount+100", "mode": "run"})
		if response.Code != 400 {
			t.Fatalf("duplicate targets accepted: %s", response.Body.String())
		}
	}
	if response := databaseRequest(t, "query", nil, map[string]any{"targets": []databaseTarget{a, b}, "sql": "DELETE FROM orders; SELECT 1", "mode": "run"}); response.Code != 400 {
		t.Fatal("multiple statements accepted as one action")
	}
	transactions := run("BEGIN", a, b)
	if transactions.Sources[0].Error == "" || transactions.Sources[1].Error == "" {
		t.Fatal("transaction left open on pooled connections")
	}
	result := run("SELECT 'RETURNING' AS literal, count(*) AS n FROM orders", a, b)
	if result.Mode != "query" || len(result.Rows) != 2 || result.Rows[0].Values[1] != "2" {
		t.Fatalf("query after execution failures: %+v", result)
	}
}

func TestDatabaseStatementResultTransport(t *testing.T) {
	for _, test := range []struct {
		sql, engine, command string
		rows                 bool
	}{
		{"/* leading */ SELECT 'UPDATE; RETURNING'", "mysql", "SELECT", true},
		{"WITH ids AS (SELECT 1) UPDATE orders SET note='RETURNING'", "postgres", "UPDATE", false},
		{"WITH ids AS (SELECT 1) DELETE FROM orders RETURNING id", "postgres", "DELETE", true},
		{"UPDATE orders SET note=$tag$RETURNING; UPDATE$tag$", "postgres", "UPDATE", false},
		{`UPDATE orders SET note='\' RETURNING id`, "postgres", "UPDATE", true},
		{`UPDATE orders SET note=E'\' RETURNING inside string'`, "postgres", "UPDATE", false},
		{`UPDATE orders SET note='\' RETURNING inside string'`, "mysql", "UPDATE", false},
		{"INSERT INTO orders(note) VALUES('RETURNING')", "sqlite", "INSERT", false},
		{"CREATE TABLE demo(id INTEGER)", "sqlite", "CREATE", false},
		{"EXPLAIN SELECT * FROM orders", "sqlite", "EXPLAIN", true},
		{"PRAGMA table_info(orders)", "sqlite", "PRAGMA", true},
	} {
		command, rows := databaseStatementKind(test.sql, test.engine)
		if command != test.command || rows != test.rows {
			t.Errorf("%s %s: got %s, rows=%v", test.engine, test.sql, command, rows)
		}
	}
}

func TestDatabaseRowChangesRollbackAndPrimaryKeys(t *testing.T) {
	a, _ := databaseTestSetup(t)
	change := func(changes []databaseRowChange) *httptest.ResponseRecorder {
		return databaseRequest(t, "changes", &a, map[string]any{"changes": changes})
	}
	update := databaseRowChange{Operation: "update", Key: map[string]any{"id": "1"}, Original: map[string]any{"note": nil}, Values: map[string]any{"note": "saved"}}
	databaseDecode[map[string]any](t, change([]databaseRowChange{update}))
	pool, _ := databases.pool(context.Background(), a)
	var note string
	_ = pool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&note)
	if note != "saved" {
		t.Fatalf("note = %q", note)
	}
	update.Original["note"] = "saved"
	update.Values["note"] = "must roll back"
	stale := databaseRowChange{Operation: "update", Key: map[string]any{"id": "9223372036854775807"}, Original: map[string]any{"note": "stale value"}, Values: map[string]any{"note": "bad"}}
	if response := change([]databaseRowChange{update, stale}); response.Code == 200 {
		t.Fatal("stale batch was committed")
	}
	_ = pool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&note)
	if note != "saved" {
		t.Fatal("the first change was not rolled back")
	}
	if response := change([]databaseRowChange{{Operation: "delete", Key: map[string]any{}, Original: map[string]any{}}}); response.Code == 200 {
		t.Fatal("unqualified delete accepted")
	}
	databaseDecode[map[string]any](t, change([]databaseRowChange{{Operation: "insert", Values: map[string]any{"id": "3", "amount": "30", "note": "new"}}}))
	databaseDecode[map[string]any](t, change([]databaseRowChange{{Operation: "delete", Key: map[string]any{"id": "3"}, Original: map[string]any{"note": "new"}}}))
}

func TestDatabaseSchemaDesignerSQLite(t *testing.T) {
	a, _ := databaseTestSetup(t)
	target := a
	target.Table = "designed"
	apply := func(req databaseSchemaRequest) {
		t.Helper()
		preview := databaseDecode[struct {
			Hash string   `json:"hash"`
			SQL  []string `json:"sql"`
		}](t, databaseRequest(t, "schema", &target, req))
		if len(preview.SQL) == 0 {
			t.Fatal("expected planned statements")
		}
		req.Hash = preview.Hash
		databaseDecode[map[string]any](t, databaseRequest(t, "schema", &target, req))
	}
	apply(databaseSchemaRequest{Action: "createTable", Name: "designed", Columns: []databaseColumnSpec{{Name: "id", Type: "integer", Primary: true, Auto: true}, {Name: "name", Type: "text", Nullable: true}}})
	pool, _ := databases.pool(context.Background(), a)
	if _, err := pool.db.Exec("INSERT INTO designed(name) VALUES ('keep me'); CREATE TABLE audit(value TEXT); CREATE TRIGGER designed_audit AFTER INSERT ON designed BEGIN INSERT INTO audit(value) VALUES (NEW.name); END"); err != nil {
		t.Fatal(err)
	}
	apply(databaseSchemaRequest{Action: "addColumn", Column: databaseColumnSpec{Name: "amount", Type: "integer", Nullable: true}})
	apply(databaseSchemaRequest{Action: "createIndex", Index: databaseIndex{Name: "idx_designed_name", Columns: []string{"name"}}})
	apply(databaseSchemaRequest{Action: "alterColumn", OldName: "amount", Column: databaseColumnSpec{Name: "amount", Type: "text", Nullable: true}})
	var got string
	if err := pool.db.QueryRow("SELECT name FROM designed WHERE id=1").Scan(&got); err != nil || got != "keep me" {
		t.Fatalf("data lost after rebuild: %q %v", got, err)
	}
	if _, err := pool.db.Exec("INSERT INTO designed(name) VALUES ('trigger survived')"); err != nil {
		t.Fatal(err)
	}
	if err := pool.db.QueryRow("SELECT value FROM audit").Scan(&got); err != nil || got != "trigger survived" {
		t.Fatalf("trigger lost: %q %v", got, err)
	}
	apply(databaseSchemaRequest{Action: "renameColumn", OldName: "amount", Name: "total"})
	apply(databaseSchemaRequest{Action: "dropColumn", OldName: "total"})
	apply(databaseSchemaRequest{Action: "dropIndex", OldName: "idx_designed_name"})
	preview := databaseDecode[struct {
		Hash string `json:"hash"`
	}](t, databaseRequest(t, "schema", &target, databaseSchemaRequest{Action: "addColumn", Column: databaseColumnSpec{Name: "stale", Type: "text", Nullable: true}}))
	if _, err := pool.db.Exec("ALTER TABLE designed ADD COLUMN external TEXT"); err != nil {
		t.Fatal(err)
	}
	response := databaseRequest(t, "schema", &target, databaseSchemaRequest{Action: "addColumn", Column: databaseColumnSpec{Name: "stale", Type: "text", Nullable: true}, Hash: preview.Hash})
	if response.Code == 200 || !strings.Contains(response.Body.String(), "changed after preview") {
		t.Fatalf("stale preview accepted: %s", response.Body.String())
	}
	apply(databaseSchemaRequest{Action: "renameTable", Name: "renamed"})
	target.Table = "renamed"
	apply(databaseSchemaRequest{Action: "dropTable"})
}

func TestDatabaseProfilesAndLocalAccess(t *testing.T) {
	databaseTestSetup(t)
	password := "fixture-secret"
	p := map[string]any{"id": "mysql-fixture", "name": "Saved connection", "engine": "mysql", "host": "localhost", "port": 3306, "user": "tester", "password": password}
	result := databaseDecode[databaseProfile](t, databaseRequest(t, "profiles", nil, p))
	if result.Password != "" || !result.HasPassword {
		t.Fatal("profile API leaked or lost credentials")
	}
	delete(p, "password")
	p["name"] = "Renamed"
	databaseDecode[databaseProfile](t, databaseRequest(t, "profiles", nil, p))
	profiles, err := readDatabaseProfiles()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range profiles {
		if value.ID == "mysql-fixture" {
			found = value.Password == password
		}
	}
	if !found {
		t.Fatal("omitted password did not preserve the existing credential")
	}
	response := databaseRequest(t, "profiles", nil, nil)
	if strings.Contains(response.Body.String(), password) {
		t.Fatal("password returned in profile list")
	}
	r := httptest.NewRequest(http.MethodGet, "/api/database/profiles", nil)
	r.RemoteAddr = "203.0.113.1:1000"
	w := httptest.NewRecorder()
	(&server{}).handleDatabase(w, r)
	if w.Code != 403 {
		t.Fatalf("remote access = %d", w.Code)
	}
	for _, engine := range []string{"mysql", "postgres"} {
		cfg, err := databaseConfig(databaseProfile{Name: engine, Engine: engine, Host: "::1", User: "user@name", Password: "p:@ss/word", TLS: "disable"})
		if err != nil {
			t.Fatal(err)
		}
		db, err := openDatabase(cfg, "name with spaces")
		if err != nil {
			t.Fatalf("%s DSN: %v", engine, err)
		}
		_ = db.Close()
	}
}

func TestDatabaseStructureForSQLTemplates(t *testing.T) {
	a, _ := databaseTestSetup(t)
	pool, err := databases.pool(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	meta := databaseDecode[databaseStructure](t, databaseRequest(t, "structure", &a, nil))
	if len(meta.Columns) != 3 || meta.Columns[0].Name != "id" || !meta.Columns[0].Primary {
		t.Fatalf("template metadata lost columns or primary key: %+v", meta)
	}
	if _, err = pool.db.Exec(`CREATE VIEW order_names AS SELECT id, note AS display_name FROM orders`); err != nil {
		t.Fatal(err)
	}
	a.Table = "order_names"
	meta = databaseDecode[databaseStructure](t, databaseRequest(t, "structure", &a, nil))
	if len(meta.Columns) != 2 || meta.Columns[1].Name != "display_name" {
		t.Fatalf("view column metadata = %+v", meta)
	}
	a.Table = "missing"
	if response := databaseRequest(t, "structure", &a, nil); response.Code == 200 {
		t.Fatal("missing table returned usable metadata")
	}
}

func TestDatabaseCancellationRestoresSQLiteConnection(t *testing.T) {
	a, _ := databaseTestSetup(t)
	pool, err := databases.pool(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = databaseReadQuery(ctx, pool, "WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<100000000) SELECT sum(x) FROM n", 10)
	if err == nil {
		t.Fatal("long query ignored cancellation")
	}
	if _, err = pool.db.Exec("INSERT INTO orders(id,amount) VALUES(3,1)"); err != nil {
		t.Fatalf("connection remained read-only or unusable after cancellation: %v", err)
	}
}

func Example_mergeDatabaseResults() {
	sources := []databaseSourceResult{{Name: "east", Result: databaseResult{Columns: []databaseColumn{{Name: "id"}, {Name: "amount"}}, Rows: [][]any{{"1", "10"}}}}, {Name: "west", Result: databaseResult{Columns: []databaseColumn{{Name: "amount"}, {Name: "id"}}, Rows: [][]any{{"20", "2"}}}}}
	result := mergeDatabaseResults(sources)
	fmt.Println(result.Rows[1].Values)
	// Output: [2 20]
}
