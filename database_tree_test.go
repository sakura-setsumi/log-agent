package main

import (
	"ariga.io/atlas/sql/mysql"
	"ariga.io/atlas/sql/schema"
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestDatabaseTreeMetadata(t *testing.T) {
	target, _ := databaseTestSetup(t)
	pool, err := databases.pool(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"CREATE TABLE parent (id INTEGER PRIMARY KEY)",
		"CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id), amount INTEGER CONSTRAINT positive CHECK(amount>0))",
		"CREATE INDEX child_parent ON child(parent_id)",
		"CREATE TRIGGER child_insert AFTER INSERT ON child BEGIN UPDATE child SET amount=amount WHERE id=NEW.id; END",
	} {
		if _, err := pool.db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	target.Table = "child"
	meta := databaseDecode[databaseStructure](t, databaseRequest(t, "structure", &target, nil))
	if len(meta.Columns) != 3 || len(meta.Indexes) < 1 || len(meta.ForeignKeys) != 1 || len(meta.Checks) != 1 || len(meta.Triggers) != 1 || meta.Triggers[0] != "child_insert" || meta.TriggerError != "" {
		t.Fatalf("incomplete tree metadata: %+v", meta)
	}
	if len(meta.ForeignKeyDetails) != 1 || meta.ForeignKeyDetails[0].Table != "parent" || len(meta.CheckDetails) != 1 || !meta.CheckDetails[0].Enforced || len(meta.TriggerDetails) != 1 || !strings.Contains(meta.TriggerDetails[0].Definition, "CREATE TRIGGER") {
		t.Fatalf("missing detail rows: %+v", meta)
	}
	if response := databaseRequest(t, "schema", &target, map[string]any{"action": "setComment", "comment": "备注"}); response.Code == 200 {
		t.Fatal("SQLite silently accepted a native comment")
	}
}

// Plan generation only needs MySQL's server variables, supplied by an isolated
// SQLite row source; no user connection is contacted by this test.
type mysqlCommentPlanSource struct{ *sql.DB }

func (s mysqlCommentPlanSource) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.DB.QueryContext(ctx, "SELECT '8.0.36', 'utf8mb4_0900_ai_ci', 'utf8mb4', 0")
}
func TestDatabaseMySQLCommentPlan(t *testing.T) {
	target, _ := databaseTestSetup(t)
	pool, err := databases.pool(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	drv, err := mysql.Open(mysqlCommentPlanSource{pool.db})
	if err != nil {
		t.Fatal(err)
	}
	from := schema.NewTable("items").AddColumns(schema.NewIntColumn("id", "int")).AddAttrs(&schema.Comment{Text: "old"})
	to := schema.NewTable("items").AddColumns(schema.NewIntColumn("id", "int")).AddAttrs(&schema.Comment{Text: "中文备注 'quoted'"})
	schema.New("test").AddTables(from)
	schema.New("test").AddTables(to)
	diff, err := drv.TableDiff(from, to)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := drv.PlanChanges(context.Background(), "comment", []schema.Change{&schema.ModifyTable{T: to, Changes: diff}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 1 || !strings.Contains(plan.Changes[0].Cmd, "COMMENT") || !strings.Contains(plan.Changes[0].Cmd, "中文备注") {
		t.Fatalf("invalid comment SQL: %+v", plan.Changes)
	}
	meta := describeDatabaseTable("mysql", to)
	if !meta.CommentSupported || meta.Comment != "中文备注 'quoted'" {
		t.Fatalf("comment not read: %+v", meta)
	}
}
