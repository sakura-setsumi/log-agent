package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/mysql"
	"ariga.io/atlas/sql/postgres"
	"ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqlite"
)

func databaseAtlas(db schema.ExecQuerier, engine string) (migrate.Driver, error) {
	switch engine {
	case "mysql":
		return mysql.Open(db)
	case "postgres":
		return postgres.Open(db)
	case "sqlite":
		return sqlite.Open(db)
	}
	return nil, errors.New("unsupported engine")
}
func databaseType(engine string, t schema.Type) (string, error) {
	switch engine {
	case "mysql":
		return mysql.FormatType(t)
	case "postgres":
		return postgres.FormatType(t)
	default:
		return sqlite.FormatType(t)
	}
}
func databaseParseType(engine, text string) (schema.Type, error) {
	switch engine {
	case "mysql":
		return mysql.ParseType(text)
	case "postgres":
		return postgres.ParseType(text)
	default:
		return sqlite.ParseType(text)
	}
}
func databaseSchemaName(engine string, target databaseTarget) string {
	if engine == "mysql" {
		return target.Database
	}
	if engine == "sqlite" {
		return "main"
	}
	if target.Schema != "" {
		return target.Schema
	}
	return "public"
}
func inspectDatabaseTable(ctx context.Context, driver migrate.Driver, engine string, target databaseTarget) (*schema.Table, error) {
	s, err := driver.InspectSchema(ctx, databaseSchemaName(engine, target), &schema.InspectOptions{Tables: []string{target.Table}})
	if err != nil {
		return nil, err
	}
	table, ok := s.Table(target.Table)
	if !ok {
		return nil, fmt.Errorf("table %q does not exist (views are read-only)", target.Table)
	}
	return table, nil
}

type databaseIndex struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Columns []string `json:"columns"`
	Primary bool     `json:"primary"`
	Method  string   `json:"method,omitempty"`
	Comment string   `json:"comment,omitempty"`
}
type databaseForeignKey struct {
	Name              string   `json:"name"`
	Columns           []string `json:"columns"`
	Schema            string   `json:"schema"`
	Table             string   `json:"table"`
	ReferencedColumns []string `json:"referencedColumns"`
	OnDelete          string   `json:"onDelete"`
	OnUpdate          string   `json:"onUpdate"`
}
type databaseCheck struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
	Enforced   bool   `json:"enforced"`
}
type databaseTrigger struct {
	Name       string `json:"name"`
	Timing     string `json:"timing"`
	Event      string `json:"event"`
	Definition string `json:"definition"`
}
type databaseStructure struct {
	Columns           []databaseColumn     `json:"columns"`
	Indexes           []databaseIndex      `json:"indexes"`
	ForeignKeys       []string             `json:"foreignKeys"`
	Checks            []string             `json:"checks"`
	Triggers          []string             `json:"triggers"`
	TriggerError      string               `json:"triggerError,omitempty"`
	ForeignKeyDetails []databaseForeignKey `json:"foreignKeyDetails"`
	CheckDetails      []databaseCheck      `json:"checkDetails"`
	TriggerDetails    []databaseTrigger    `json:"triggerDetails"`
	Comment           string               `json:"comment"`
	CommentSupported  bool                 `json:"commentSupported"`
}

func describeDatabaseTable(engine string, table *schema.Table) databaseStructure {
	result := databaseStructure{Columns: []databaseColumn{}, Indexes: []databaseIndex{}, ForeignKeys: []string{}}
	result.CommentSupported = engine == "mysql" || engine == "postgres"
	primary := map[string]bool{}
	if table.PrimaryKey != nil {
		for _, part := range table.PrimaryKey.Parts {
			if part.C != nil {
				primary[part.C.Name] = true
			}
		}
	}
	for _, column := range table.Columns {
		typ, _ := databaseType(engine, column.Type.Type)
		c := databaseColumn{Name: column.Name, Type: typ, Nullable: column.Type.Null, Primary: primary[column.Name]}
		if column.Default != nil {
			var text string
			switch d := column.Default.(type) {
			case *schema.Literal:
				text = d.V
			case *schema.RawExpr:
				text = d.X
			}
			c.Default = &text
		}
		for _, attr := range column.Attrs {
			if _, ok := attr.(*schema.GeneratedExpr); ok {
				c.Generated = true
			}
		}
		result.Columns = append(result.Columns, c)
	}
	indexes := append([]*schema.Index{}, table.Indexes...)
	if table.PrimaryKey != nil {
		indexes = append([]*schema.Index{table.PrimaryKey}, indexes...)
	}
	for _, index := range indexes {
		i := databaseIndex{Name: index.Name, Unique: index.Unique, Primary: index == table.PrimaryKey, Columns: []string{}}
		if i.Primary && i.Name == "" {
			i.Name = "PRIMARY"
		}
		for _, attr := range index.Attrs {
			switch a := attr.(type) {
			case *schema.Comment:
				i.Comment = a.Text
			case *mysql.IndexType:
				i.Method = a.T
			case *postgres.IndexType:
				i.Method = a.T
			}
		}
		for _, part := range index.Parts {
			if part.C != nil {
				i.Columns = append(i.Columns, part.C.Name)
			} else {
				i.Columns = append(i.Columns, "(expression)")
			}
		}
		result.Indexes = append(result.Indexes, i)
	}
	for _, key := range table.ForeignKeys {
		result.ForeignKeys = append(result.ForeignKeys, key.Symbol)
		fk := databaseForeignKey{Name: key.Symbol, OnDelete: string(key.OnDelete), OnUpdate: string(key.OnUpdate), Columns: []string{}, ReferencedColumns: []string{}}
		for _, c := range key.Columns {
			fk.Columns = append(fk.Columns, c.Name)
		}
		for _, c := range key.RefColumns {
			fk.ReferencedColumns = append(fk.ReferencedColumns, c.Name)
		}
		if key.RefTable != nil {
			fk.Table = key.RefTable.Name
			if key.RefTable.Schema != nil {
				fk.Schema = key.RefTable.Schema.Name
			}
		}
		result.ForeignKeyDetails = append(result.ForeignKeyDetails, fk)
	}
	for _, attr := range table.Attrs {
		if comment, ok := attr.(*schema.Comment); ok {
			result.Comment = comment.Text
		}
		if check, ok := attr.(*schema.Check); ok {
			enforced := true
			for _, attr := range check.Attrs {
				if flag, ok := attr.(*mysql.Enforced); ok {
					enforced = flag.V
				}
			}
			result.CheckDetails = append(result.CheckDetails, databaseCheck{Name: check.Name, Expression: check.Expr, Enforced: enforced})
			label := check.Expr
			if check.Name != "" {
				label = check.Name + ": " + check.Expr
			}
			result.Checks = append(result.Checks, label)
		}
	}
	return result
}

func databaseTriggerDetails(ctx context.Context, pool *databasePool, target databaseTarget) ([]databaseTrigger, error) {
	query := "SELECT name, '', '', sql FROM sqlite_schema WHERE type='trigger' AND tbl_name=? ORDER BY name"
	args := []any{target.Table}
	switch pool.profile.Engine {
	case "mysql":
		query = "SELECT TRIGGER_NAME, ACTION_TIMING, EVENT_MANIPULATION, ACTION_STATEMENT FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=? AND EVENT_OBJECT_TABLE=? ORDER BY TRIGGER_NAME"
		args = []any{target.Database, target.Table}
	case "postgres":
		query = "SELECT t.tgname, CASE WHEN (t.tgtype & 2)<>0 THEN 'BEFORE' WHEN (t.tgtype & 64)<>0 THEN 'INSTEAD OF' ELSE 'AFTER' END, concat_ws(' OR ', CASE WHEN (t.tgtype & 4)<>0 THEN 'INSERT' END, CASE WHEN (t.tgtype & 8)<>0 THEN 'DELETE' END, CASE WHEN (t.tgtype & 16)<>0 THEN 'UPDATE' END, CASE WHEN (t.tgtype & 32)<>0 THEN 'TRUNCATE' END), pg_catalog.pg_get_triggerdef(t.oid) FROM pg_catalog.pg_trigger t JOIN pg_catalog.pg_class c ON c.oid=t.tgrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE NOT t.tgisinternal AND n.nspname=$1 AND c.relname=$2 ORDER BY t.tgname"
		args = []any{databaseSchemaName(pool.profile.Engine, target), target.Table}
	}
	rows, err := pool.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []databaseTrigger{}
	for rows.Next() {
		var name databaseTrigger
		if err := rows.Scan(&name.Name, &name.Timing, &name.Event, &name.Definition); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// Template pickers need column metadata without reading any table records.
func handleDatabaseStructure(w http.ResponseWriter, r *http.Request, pool *databasePool, target databaseTarget) error {
	if r.Method != http.MethodGet || target.Table == "" {
		return errors.New("GET and a table are required")
	}
	driver, err := databaseAtlas(pool.db, pool.profile.Engine)
	if err != nil {
		return err
	}
	table, err := inspectDatabaseTable(r.Context(), driver, pool.profile.Engine, target)
	if err == nil {
		meta := describeDatabaseTable(pool.profile.Engine, table)
		meta.TriggerDetails, err = databaseTriggerDetails(r.Context(), pool, target)
		meta.Triggers = []string{}
		for _, trigger := range meta.TriggerDetails {
			meta.Triggers = append(meta.Triggers, trigger.Name)
		}
		if err != nil {
			meta.TriggerError = err.Error()
		}
		writeJSON(w, 200, meta)
		return nil
	}
	// Views may not be exposed by the schema inspector. An empty SELECT still
	// returns their column names/types without loading records.
	result, err := queryDatabase(r.Context(), pool.db, "SELECT * FROM "+databaseTableName(pool.profile.Engine, target)+" LIMIT 0", 1)
	if err != nil {
		return err
	}
	writeJSON(w, 200, databaseStructure{Columns: result.Columns, Indexes: []databaseIndex{}, ForeignKeys: []string{}})
	return nil
}

func handleDatabaseTable(w http.ResponseWriter, r *http.Request, pool *databasePool, target databaseTarget) error {
	if target.Table == "" {
		return errors.New("choose a table")
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	if r.URL.Query().Get("last") == "1" {
		// Jump to the final page: the offset comes from the current row count.
		var total int64
		if err := pool.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM "+databaseTableName(pool.profile.Engine, target)).Scan(&total); err != nil {
			return err
		}
		offset = 0
		if total > 0 {
			offset = int((total - 1) / int64(limit) * int64(limit))
		}
	}
	meta := databaseStructure{Columns: []databaseColumn{}, Indexes: []databaseIndex{}, ForeignKeys: []string{}}
	driver, err := databaseAtlas(pool.db, pool.profile.Engine)
	if err != nil {
		return err
	}
	table, inspectionErr := inspectDatabaseTable(r.Context(), driver, pool.profile.Engine, target)
	order := ""
	if inspectionErr == nil {
		meta = describeDatabaseTable(pool.profile.Engine, table)
		keys := []string{}
		for _, c := range meta.Columns {
			if c.Primary {
				keys = append(keys, databaseQuote(pool.profile.Engine, c.Name))
			}
		}
		if len(keys) > 0 {
			order = " ORDER BY " + strings.Join(keys, ",")
		}
	}
	result, err := queryDatabase(r.Context(), pool.db, fmt.Sprintf("SELECT * FROM %s%s LIMIT %d OFFSET %d", databaseTableName(pool.profile.Engine, target), order, limit+1, offset), limit)
	if err != nil {
		return err
	}
	if inspectionErr != nil {
		meta.Columns = result.Columns
	}
	writeJSON(w, 200, map[string]any{"result": result, "structure": meta, "offset": offset, "limit": limit, "editable": inspectionErr == nil && table.PrimaryKey != nil, "inspectionError": func() string {
		if inspectionErr != nil {
			return inspectionErr.Error()
		}
		return ""
	}()})
	return nil
}

func databaseValue(value any) (any, error) {
	if object, ok := value.(map[string]any); ok {
		encoded, ok := object["blob"].(string)
		if !ok || len(object) != 1 {
			return nil, errors.New("invalid cell value")
		}
		return base64.StdEncoding.DecodeString(encoded)
	}
	switch value.(type) {
	case nil, string, bool, json.Number, float64:
		return value, nil
	}
	return nil, errors.New("invalid cell value")
}

type databaseRowChange struct {
	Operation string         `json:"operation"`
	Key       map[string]any `json:"key"`
	Original  map[string]any `json:"original"`
	Values    map[string]any `json:"values"`
}

type databaseChangeBatch struct {
	Target  databaseTarget      `json:"target"`
	Changes []databaseRowChange `json:"changes"`
}

func handleDatabaseChanges(w http.ResponseWriter, r *http.Request, pool *databasePool, target databaseTarget) error {
	var req struct {
		Changes []databaseRowChange   `json:"changes"`
		Batches []databaseChangeBatch `json:"batches"`
	}
	if err := decodeDatabaseRequest(w, r, &req); err != nil {
		return err
	}
	if len(req.Batches) != 0 && len(req.Changes) != 0 {
		return errors.New("choose changes or batches")
	}
	batches := req.Batches
	if len(batches) == 0 {
		batches = []databaseChangeBatch{{Target: target, Changes: req.Changes}}
	}
	count := 0
	for _, batch := range batches {
		count += len(batch.Changes)
	}
	if count == 0 || count > 500 || len(batches) > 500 {
		return errors.New("submit between 1 and 500 row changes")
	}
	driver, err := databaseAtlas(pool.db, pool.profile.Engine)
	if err != nil {
		return err
	}
	tables := make([]*schema.Table, len(batches))
	// Inspect every destination before opening a transaction (SQLite may use a
	// single connection). A batch cannot redirect writes to another connection.
	for i := range batches {
		dest := &batches[i].Target
		if dest.ID == "" {
			dest.ID = target.ID
		}
		if dest.Database == "" {
			dest.Database = target.Database
		}
		if dest.ID != target.ID || dest.Database != target.Database || dest.Table == "" {
			return errors.New("all batches must target tables in the selected database")
		}
		tables[i], err = inspectDatabaseTable(r.Context(), driver, pool.profile.Engine, *dest)
		if err != nil {
			return err
		}

		if pool.profile.Engine == "mysql" {
			var transactional string
			err = pool.db.QueryRowContext(r.Context(), "SELECT e.TRANSACTIONS FROM information_schema.TABLES t JOIN information_schema.ENGINES e ON e.ENGINE=t.ENGINE WHERE t.TABLE_SCHEMA=? AND t.TABLE_NAME=?", dest.Database, dest.Table).Scan(&transactional)
			if err != nil {
				return err
			}
			if !strings.EqualFold(transactional, "YES") {
				return fmt.Errorf("table %q does not support transactional edits", dest.Table)
			}
		}
	}
	tx, err := pool.db.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var affected int64
	for i, batch := range batches {
		target, table := batch.Target, tables[i]
		columns := map[string]*schema.Column{}
		for _, c := range table.Columns {
			columns[c.Name] = c
		}
		primary := []string{}
		if table.PrimaryKey != nil {
			for _, p := range table.PrimaryKey.Parts {
				if p.C != nil {
					primary = append(primary, p.C.Name)
				}
			}
		}
		for _, change := range batch.Changes {
			args := []any{}
			bind := func(value any) (string, error) {
				v, e := databaseValue(value)
				if e != nil {
					return "", e
				}
				args = append(args, v)
				return databasePlaceholder(pool.profile.Engine, len(args)), nil
			}
			names := make([]string, 0, len(change.Values))
			for name := range change.Values {
				if columns[name] == nil {
					return fmt.Errorf("unknown column %q", name)
				}
				for _, a := range columns[name].Attrs {
					if _, ok := a.(*schema.GeneratedExpr); ok {
						return fmt.Errorf("generated column %q is read-only", name)
					}
				}
				names = append(names, name)
			}
			sort.Strings(names)
			statement := ""
			qualified := databaseTableName(pool.profile.Engine, target)
			if change.Operation == "insert" {
				quoted := []string{}
				slots := []string{}
				for _, name := range names {
					slot, e := bind(change.Values[name])
					if e != nil {
						return e
					}
					quoted = append(quoted, databaseQuote(pool.profile.Engine, name))
					slots = append(slots, slot)
				}
				if len(names) == 0 {
					if pool.profile.Engine == "mysql" {
						statement = "INSERT INTO " + qualified + " () VALUES ()"
					} else {
						statement = "INSERT INTO " + qualified + " DEFAULT VALUES"
					}
				} else {
					statement = "INSERT INTO " + qualified + " (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(slots, ",") + ")"
				}
			} else {
				if len(primary) == 0 || len(change.Key) != len(primary) {
					return errors.New("editing and deleting rows requires a complete primary key")
				}
				set := []string{}
				if change.Operation == "update" {
					if len(names) == 0 {
						continue
					}
					for _, name := range names {
						if _, ok := change.Original[name]; !ok {
							return errors.New("missing original cell value")
						}
						slot, e := bind(change.Values[name])
						if e != nil {
							return e
						}
						set = append(set, databaseQuote(pool.profile.Engine, name)+"="+slot)
					}
				} else if change.Operation != "delete" {
					return errors.New("invalid row operation")
				}
				where := []string{}
				condition := func(name string, value any) error {
					if columns[name] == nil {
						return fmt.Errorf("unknown column %q", name)
					}
					name = databaseQuote(pool.profile.Engine, name)
					if value == nil {
						where = append(where, name+" IS NULL")
						return nil
					}
					slot, e := bind(value)
					if e != nil {
						return e
					}
					where = append(where, name+"="+slot)
					return nil
				}
				for _, name := range primary {
					value, ok := change.Key[name]
					if !ok || value == nil {
						return errors.New("missing primary key value")
					}
					if err = condition(name, value); err != nil {
						return err
					}
				}
				originalNames := make([]string, 0, len(change.Original))
				for name := range change.Original {
					originalNames = append(originalNames, name)
				}
				sort.Strings(originalNames)
				for _, name := range originalNames {
					if err = condition(name, change.Original[name]); err != nil {
						return err
					}
				}
				if change.Operation == "delete" {
					statement = "DELETE FROM " + qualified + " WHERE " + strings.Join(where, " AND ")
				} else {
					statement = "UPDATE " + qualified + " SET " + strings.Join(set, ",") + " WHERE " + strings.Join(where, " AND ")
				}
			}
			res, e := tx.ExecContext(r.Context(), statement, args...)
			if e != nil {
				return e
			}
			n, e := res.RowsAffected()
			if e != nil {
				return e
			}
			if n != 1 {
				return errors.New("record changed or no longer exists; reload the table (the entire batch was rolled back)")
			}
			affected += n
		}

	}
	if err = tx.Commit(); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"ok": true, "affected": affected})
	return nil
}

type databaseColumnSpec struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
	Primary  bool    `json:"primary"`
	Auto     bool    `json:"auto"`
}
type databaseSchemaRequest struct {
	Comment string               `json:"comment"`
	Action  string               `json:"action"`
	Name    string               `json:"name"`
	OldName string               `json:"oldName"`
	Columns []databaseColumnSpec `json:"columns"`
	Column  databaseColumnSpec   `json:"column"`
	Index   databaseIndex        `json:"index"`
	Hash    string               `json:"hash"`
}

func databaseNewColumn(engine string, spec databaseColumnSpec) (*schema.Column, error) {
	if spec.Name == "" || strings.ContainsRune(spec.Name, 0) {
		return nil, errors.New("column name is required")
	}
	if strings.ContainsAny(spec.Type, ";\x00") {
		return nil, errors.New("invalid column type")
	}
	typ, err := databaseParseType(engine, spec.Type)
	if err != nil {
		return nil, err
	}
	c := schema.NewColumn(spec.Name).SetType(typ).SetNull(spec.Nullable && !spec.Primary)
	if spec.Default != nil && *spec.Default != "" {
		c.SetDefault(&schema.RawExpr{X: *spec.Default})
	}
	if spec.Auto {
		switch engine {
		case "mysql":
			c.AddAttrs(&mysql.AutoIncrement{})
		case "postgres":
			c.AddAttrs(&postgres.Identity{Generation: "BY DEFAULT"})
		case "sqlite":
			if !spec.Primary || strings.ToLower(spec.Type) != "integer" {
				return nil, errors.New("SQLite autoincrement requires an INTEGER primary key")
			}
			c.AddAttrs(&sqlite.AutoIncrement{})
		}
	}
	return c, nil
}

func buildDatabaseSchemaPlan(ctx context.Context, pool *databasePool, target databaseTarget, req databaseSchemaRequest) (*migrate.Plan, string, error) {
	driver, err := databaseAtlas(pool.db, pool.profile.Engine)
	if err != nil {
		return nil, "", err
	}
	engine := pool.profile.Engine
	var changes []schema.Change
	var baseline any
	if req.Action == "createTable" {
		if req.Name == "" || len(req.Columns) == 0 {
			return nil, "", errors.New("table name and columns are required")
		}
		table := schema.NewTable(req.Name).SetSchema(schema.New(databaseSchemaName(engine, target)))
		primary := []*schema.Column{}
		seen := map[string]bool{}
		for _, spec := range req.Columns {
			if seen[spec.Name] {
				return nil, "", errors.New("duplicate column name")
			}
			seen[spec.Name] = true
			c, e := databaseNewColumn(engine, spec)
			if e != nil {
				return nil, "", e
			}
			table.AddColumns(c)
			if spec.Primary {
				primary = append(primary, c)
			}
		}
		if len(primary) > 0 {
			table.SetPrimaryKey(schema.NewPrimaryKey(primary...))
		}
		changes = []schema.Change{&schema.AddTable{T: table}}
	} else {
		from, e := inspectDatabaseTable(ctx, driver, engine, target)
		if e != nil {
			return nil, "", e
		}
		baseline = describeDatabaseTable(engine, from)
		to, e := inspectDatabaseTable(ctx, driver, engine, target)
		if e != nil {
			return nil, "", e
		}
		switch req.Action {
		case "setComment":
			if engine == "sqlite" {
				return nil, "", errors.New("SQLite does not support native table comments")
			}
			if len(req.Comment) > 8192 || strings.ContainsRune(req.Comment, 0) {
				return nil, "", errors.New("invalid table comment")
			}
			attrs := make([]schema.Attr, 0, len(to.Attrs)+1)
			for _, attr := range to.Attrs {
				if _, ok := attr.(*schema.Comment); !ok {
					attrs = append(attrs, attr)
				}
			}
			if req.Comment != "" {
				attrs = append(attrs, &schema.Comment{Text: req.Comment})
			}
			to.Attrs = attrs
		case "renameTable":
			if req.Name == "" {
				return nil, "", errors.New("new table name is required")
			}
			to.Name = req.Name
			changes = []schema.Change{&schema.RenameTable{From: from, To: to}}
		case "dropTable":
			changes = []schema.Change{&schema.DropTable{T: from}}
		case "addColumn":
			if _, ok := to.Column(req.Column.Name); ok {
				return nil, "", errors.New("column already exists")
			}
			c, e := databaseNewColumn(engine, req.Column)
			if e != nil {
				return nil, "", e
			}
			if req.Column.Primary {
				return nil, "", errors.New("primary key changes must be executed in the SQL editor")
			}
			to.AddColumns(c)
		case "alterColumn":
			c, ok := to.Column(req.OldName)
			if !ok {
				return nil, "", errors.New("column does not exist")
			}
			if req.Column.Name != c.Name {
				return nil, "", errors.New("use rename column to change its name")
			}
			fresh, e := databaseNewColumn(engine, req.Column)
			if e != nil {
				return nil, "", e
			}
			c.Type = fresh.Type
			c.Default = fresh.Default
		case "renameColumn":
			c, ok := to.Column(req.OldName)
			old, _ := from.Column(req.OldName)
			if !ok || req.Name == "" {
				return nil, "", errors.New("column and new name are required")
			}
			if _, exists := to.Column(req.Name); exists {
				return nil, "", errors.New("column name already exists")
			}
			c.Name = req.Name
			changes = []schema.Change{&schema.ModifyTable{T: to, Changes: []schema.Change{&schema.RenameColumn{From: old, To: c}}}}
		case "dropColumn":
			c, ok := to.Column(req.OldName)
			if !ok || len(to.Columns) < 2 {
				return nil, "", errors.New("column does not exist or is the last column")
			}
			indexes := append([]*schema.Index{}, to.Indexes...)
			if to.PrimaryKey != nil {
				indexes = append(indexes, to.PrimaryKey)
			}
			for _, i := range indexes {
				for _, p := range i.Parts {
					if p.C == c {
						return nil, "", errors.New("remove this column's indexes or primary key in SQL before dropping it")
					}
				}
			}
			if len(c.ForeignKeys) > 0 {
				return nil, "", errors.New("remove foreign keys before dropping this column")
			}
			kept := []*schema.Column{}
			for _, col := range to.Columns {
				if col != c {
					kept = append(kept, col)
				}
			}
			to.Columns = kept
		case "createIndex":
			if req.Index.Name == "" || len(req.Index.Columns) == 0 {
				return nil, "", errors.New("index name and columns are required")
			}
			index := schema.NewIndex(req.Index.Name).SetUnique(req.Index.Unique)
			for _, name := range req.Index.Columns {
				col, ok := to.Column(name)
				if !ok {
					return nil, "", fmt.Errorf("column %q does not exist", name)
				}
				index.AddColumns(col)
			}
			to.AddIndexes(index)
		case "dropIndex":
			found := false
			kept := []*schema.Index{}
			for _, index := range to.Indexes {
				if index.Name == req.OldName {
					found = true
				} else {
					kept = append(kept, index)
				}
			}
			if !found {
				return nil, "", errors.New("index does not exist or belongs to the primary key")
			}
			to.Indexes = kept
		default:
			return nil, "", errors.New("unsupported schema action")
		}
		if changes == nil {
			changes, err = driver.TableDiff(from, to)
			if err != nil {
				return nil, "", err
			}
			if len(changes) > 0 {
				changes = []schema.Change{&schema.ModifyTable{T: to, Changes: changes}}
			}
		}
	}
	plan, err := driver.PlanChanges(ctx, "database-designer", changes)
	if err != nil {
		return nil, "", err
	}
	// SQLite rebuilds can drop attached triggers. Keep their definitions in the
	// same migration transaction; invalid definitions make the transaction fail.
	if engine == "sqlite" && req.Action != "dropTable" {
		rebuild := false
		for _, change := range plan.Changes {
			if strings.HasPrefix(change.Cmd, "DROP TABLE") {
				rebuild = true
			}
		}
		if rebuild {
			rows, e := pool.db.QueryContext(ctx, "SELECT sql FROM sqlite_schema WHERE type='trigger' AND tbl_name=? AND sql IS NOT NULL ORDER BY name", target.Table)
			if e != nil {
				return nil, "", e
			}
			for rows.Next() {
				var text string
				if e = rows.Scan(&text); e != nil {
					rows.Close()
					return nil, "", e
				}
				plan.Changes = append(plan.Changes, &migrate.Change{Cmd: text, Comment: "restore table trigger"})
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return nil, "", e
			}
		}
	}
	commands := []string{}
	for _, c := range plan.Changes {
		commands = append(commands, c.Cmd)
	}
	data, _ := json.Marshal([]any{target, baseline, commands})
	hash := sha256.Sum256(data)
	return plan, hex.EncodeToString(hash[:]), nil
}

func handleDatabaseSchema(w http.ResponseWriter, r *http.Request, pool *databasePool, target databaseTarget) error {
	var req databaseSchemaRequest
	if err := decodeDatabaseRequest(w, r, &req); err != nil {
		return err
	}
	plan, hash, err := buildDatabaseSchemaPlan(r.Context(), pool, target, req)
	if err != nil {
		return err
	}
	commands := []string{}
	for _, c := range plan.Changes {
		commands = append(commands, c.Cmd)
	}
	if req.Hash == "" {
		writeJSON(w, 200, map[string]any{"sql": commands, "hash": hash, "transactional": plan.Transactional})
		return nil
	}
	if req.Hash != hash {
		return errors.New("table structure changed after preview; generate a new preview")
	}
	if pool.profile.Engine == "mysql" {
		for i, c := range plan.Changes {
			if _, err = pool.db.ExecContext(r.Context(), c.Cmd, c.Args...); err != nil {
				return fmt.Errorf("DDL failed after %d statement(s) (MySQL DDL may auto-commit): %w", i, err)
			}
		}
	} else {
		var tx interface {
			schema.ExecQuerier
			Commit() error
			Rollback() error
		}
		if pool.profile.Engine == "sqlite" {
			tx, err = sqlite.OpenTx(r.Context(), pool.db, nil)
		} else {
			tx, err = pool.db.BeginTx(r.Context(), nil)
		}
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, c := range plan.Changes {
			if _, err = tx.ExecContext(r.Context(), c.Cmd, c.Args...); err != nil {
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "statements": len(commands)})
	return nil
}
