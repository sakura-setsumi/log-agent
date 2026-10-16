package main

import (
	"context"
	"strings"
	"testing"
)

func TestDatabaseEditableSelect(t *testing.T) {
	for _, test := range []struct {
		sql, engine, table string
		editable           bool
	}{
		{"SELECT * FROM `sys_user` WHERE `phonenumber` LIKE '13700312596' LIMIT 100;", "mysql", "sys_user", true},
		{"SELECT u.user_id AS id, u.nick_name FROM biz_db.sys_user AS u WHERE u.user_id=1", "mysql", "sys_user", true},
		{`SELECT o.* FROM "public"."orders" o ORDER BY id LIMIT 100`, "postgres", "orders", true},
		{`SELECT ID AS Row_ID FROM Public.Orders`, "postgres", "orders", true},
		{`SELECT "ID" FROM "Orders"`, "postgres", "Orders", true},
		{`-- SELECT FROM ignored\nSELECT * FROM orders`, "sqlite", "", false},
		{"-- ignored\nSELECT * FROM orders WHERE note='FROM JOIN; SELECT'", "sqlite", "orders", true},
		{`SELECT "字 段" FROM "表" WHERE id=1`, "postgres", "表", true},
		{`SELECT [odd]]field] FROM [odd]]table]`, "sqlite", "odd]table", true},
		{"SELECT * FROM orders o JOIN users u ON u.id=o.id", "mysql", "", false},
		{"SELECT * FROM orders, users", "mysql", "", false},
		{"SELECT DISTINCT id FROM orders", "mysql", "", false},
		{"SELECT id, COUNT(*) FROM orders GROUP BY id", "mysql", "", false},
		{"SELECT id, amount+1 AS amount FROM orders", "sqlite", "", false},
		{"SELECT id FROM orders UNION SELECT id FROM users", "sqlite", "", false},
		{"SELECT * FROM (SELECT * FROM orders) o", "sqlite", "", false},
		{"SELECT * FROM orders WHERE id IN (SELECT id FROM users)", "sqlite", "", false},
		{"SELECT * FROM orders /*! JOIN users */", "mysql", "", false},
		{"SELECT * FROM orders; DELETE FROM orders", "mysql", "", false},
		{"SELECT id, FROM orders", "sqlite", "", false},
		{"SELECT 1 FROM orders", "sqlite", "", false},
		{"SELECT * FROM orders()", "postgres", "", false},
		{"SELECT * FROM orders WHERE id=1--x UNION SELECT * FROM users", "mysql", "", false},
		{"SELECT * FROM orders /*M! UNION SELECT * FROM users */", "mysql", "", false},
		{"SELECT * FROM orders\u00a0WHERE id=1", "sqlite", "", false},
		{"SELECT * FROM one.two.three", "postgres", "", false},
	} {
		t.Run(test.sql, func(t *testing.T) {
			_, table, _, ok := databaseEditableSelect(test.sql, test.engine)
			if ok != test.editable || ok && table != test.table {
				t.Fatalf("table=%q editable=%v", table, ok)
			}
		})
	}
}

func TestDatabaseQueryResultEditing(t *testing.T) {
	east, west := databaseTestSetup(t)
	query := func(text string, targets ...databaseTarget) databaseQueryResponse {
		return databaseDecode[databaseQueryResponse](t, databaseRequest(t, "query", nil, map[string]any{"sql": text, "targets": targets, "mode": "run"}))
	}
	result := query("SELECT o.id AS row_id, o.amount AS cost, o.note AS detail FROM orders o WHERE id=1", east, west)
	if len(result.Rows) != 2 {
		t.Fatalf("rows=%+v", result.Rows)
	}
	for i, source := range result.Sources {
		if source.Edit == nil || source.Edit.Target.ID != []string{"east", "west"}[i] || source.Edit.Target.Table != "orders" || source.Edit.Columns["row_id"].Name != "id" || !source.Edit.Columns["row_id"].Primary || source.Edit.Columns["detail"].Name != "note" {
			t.Fatalf("source edit mapping=%+v", source.Edit)
		}
	}
	edit := result.Sources[1].Edit
	change := databaseRowChange{Operation: "update", Key: map[string]any{"id": "1"}, Original: map[string]any{"note": nil}, Values: map[string]any{"note": "结果编辑"}}
	databaseDecode[map[string]any](t, databaseRequest(t, "changes", &edit.Target, map[string]any{"changes": []databaseRowChange{change}}))
	after := query("SELECT * FROM orders WHERE id=1", east, west)
	if after.Rows[0].Values[2] != nil || after.Rows[1].Values[2] != "结果编辑" {
		t.Fatalf("edit crossed sources: %+v", after.Rows)
	}
	// Reusing stale original data must fail and retain the first committed value.
	change.Values["note"] = "stale overwrite"
	if w := databaseRequest(t, "changes", &edit.Target, map[string]any{"changes": []databaseRowChange{change}}); w.Code == 200 || !strings.Contains(w.Body.String(), "record changed") {
		t.Fatalf("stale result accepted: %s", w.Body.String())
	}
	for _, sql := range []string{"SELECT note FROM orders", "SELECT id, id FROM orders", "SELECT id AS repeated, amount AS repeated FROM orders", "SELECT COUNT(*) AS id FROM orders", "SELECT a.* FROM orders a JOIN orders b ON a.id=b.id"} {
		if got := query(sql, east); got.Sources[0].Edit != nil {
			t.Fatalf("ambiguous result editable: %s %+v", sql, got.Sources[0].Edit)
		}
	}
	pool, err := databases.pool(context.Background(), east)
	if err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		"CREATE TABLE no_key (id INTEGER, value TEXT)",
		"CREATE VIEW orders_view AS SELECT * FROM orders",
		"CREATE TABLE compound (a INTEGER, b INTEGER, value TEXT, PRIMARY KEY(a,b))",
		"CREATE TABLE computed (id INTEGER PRIMARY KEY, value INTEGER, doubled INTEGER GENERATED ALWAYS AS(value*2) STORED)",
	} {
		if _, err = pool.db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{"SELECT * FROM no_key", "SELECT * FROM orders_view", "SELECT a,value FROM compound"} {
		if got := query(sql, east); got.Sources[0].Edit != nil {
			t.Fatalf("result without complete primary key editable: %s", sql)
		}
	}
	if got := query("SELECT * FROM compound", east); got.Sources[0].Edit == nil || !got.Sources[0].Edit.Columns["a"].Primary || !got.Sources[0].Edit.Columns["b"].Primary {
		t.Fatal("composite key mapping missing")
	}
	if got := query("SELECT * FROM computed", east); got.Sources[0].Edit == nil || !got.Sources[0].Edit.Columns["doubled"].Generated {
		t.Fatal("generated column was not identified")
	}
}
