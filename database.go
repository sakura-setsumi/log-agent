package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ariga.io/atlas/sql/migrate"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type databaseProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port,omitempty"`
	User        string `json:"user,omitempty"`
	Password    string `json:"password,omitempty"`
	HasPassword bool   `json:"hasPassword,omitempty"`
	Database    string `json:"database,omitempty"`
	File        string `json:"file,omitempty"`
	TLS         string `json:"tls,omitempty"`
}

type databaseTarget struct {
	ID       string `json:"id"`
	Database string `json:"database"`
	Schema   string `json:"schema,omitempty"`
	Table    string `json:"table,omitempty"`
}

type databasePool struct {
	db      *sql.DB
	profile databaseProfile
	mu      sync.Mutex
}
type databaseManager struct {
	mu    sync.Mutex
	pools map[string]*databasePool
}

var databases = &databaseManager{pools: make(map[string]*databasePool)}

func databaseProfilesPath() string { return filepath.Join(configDir(), "databases.json") }
func readDatabaseProfiles() ([]databaseProfile, error) {
	profiles := []databaseProfile{}
	err := readJSONFile(databaseProfilesPath(), &profiles)
	if errors.Is(err, errNoConfigFile) {
		err = nil
	}
	if profiles == nil {
		profiles = []databaseProfile{}
	}
	return profiles, err
}
func (m *databaseManager) closePools(id string) {
	for key, pool := range m.pools {
		if id == "" || pool.profile.ID == id {
			_ = pool.db.Close()
			delete(m.pools, key)
		}
	}
}
func closeDatabasePools() { databases.mu.Lock(); defer databases.mu.Unlock(); databases.closePools("") }

func databaseConfig(p databaseProfile) (databaseProfile, error) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return p, errors.New("connection name is required")
	}
	switch p.Engine {
	case "mysql", "postgres":
		if strings.TrimSpace(p.Host) == "" || p.User == "" {
			return p, errors.New("host and user are required")
		}
		if p.Port == 0 {
			if p.Engine == "mysql" {
				p.Port = 3306
			} else {
				p.Port = 5432
			}
		}
		if p.Port < 1 || p.Port > 65535 {
			return p, errors.New("invalid port")
		}
		if p.TLS == "" {
			p.TLS = "disable"
		}
		if p.TLS != "disable" && p.TLS != "require" && p.TLS != "verify-full" {
			return p, errors.New("invalid TLS mode")
		}
	case "sqlite":
		if !filepath.IsAbs(p.File) {
			return p, errors.New("SQLite requires an absolute file path")
		}
		p.File = filepath.Clean(p.File)
	default:
		return p, errors.New("unsupported database engine")
	}
	return p, nil
}

func openDatabase(p databaseProfile, database string) (*sql.DB, error) {
	if database == "" {
		database = p.Database
	}
	var db *sql.DB
	var err error
	switch p.Engine {
	case "mysql":
		c := mysqldriver.NewConfig()
		c.User, c.Passwd, c.Net, c.Addr, c.DBName = p.User, p.Password, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), database
		c.Timeout, c.ReadTimeout, c.WriteTimeout = 10*time.Second, 90*time.Second, 90*time.Second
		// Keep MySQL date/time cells in their native text representation so
		// optimistic comparisons and edits preserve wall-clock and zero dates.
		c.ClientFoundRows = true
		if p.TLS == "require" {
			c.TLSConfig = "skip-verify"
		} else if p.TLS == "verify-full" {
			c.TLSConfig = "true"
		}
		db, err = sql.Open("mysql", c.FormatDSN())
	case "postgres":
		if database == "" {
			database = "postgres"
		}
		u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), User: url.UserPassword(p.User, p.Password), Path: "/" + database}
		q := u.Query()
		q.Set("sslmode", p.TLS)
		q.Set("connect_timeout", "10")
		u.RawQuery = q.Encode()
		var c *pgx.ConnConfig
		c, err = pgx.ParseConfig(u.String())
		if err == nil {
			db = stdlib.OpenDB(*c)
		}
	case "sqlite":
		filePath := filepath.ToSlash(p.File)
		if !strings.HasPrefix(filePath, "/") {
			filePath = "/" + filePath
		}
		u := url.URL{Scheme: "file", Path: filePath}
		q := u.Query()
		q.Add("_pragma", "foreign_keys(1)")
		q.Add("_pragma", "busy_timeout(5000)")
		u.RawQuery = q.Encode()
		db, err = sql.Open("sqlite", u.String())
	default:
		err = errors.New("unsupported database engine")
	}
	if err != nil {
		return nil, err
	}
	// A pool belongs to one database. One physical connection also keeps SQLite
	// session pragmas and transaction state isolated from other operations.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}

func (m *databaseManager) pool(ctx context.Context, target databaseTarget) (*databasePool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	profiles, err := readDatabaseProfiles()
	if err != nil {
		return nil, err
	}
	var p databaseProfile
	for _, value := range profiles {
		if value.ID == target.ID {
			p = value
			break
		}
	}
	if p.ID == "" {
		return nil, errors.New("connection does not exist")
	}
	if target.Database == "" {
		target.Database = p.Database
	}
	if p.Engine == "sqlite" {
		target.Database = "main"
	}
	key := p.ID + "\x00" + target.Database
	if pool := m.pools[key]; pool != nil {
		return pool, nil
	}
	db, err := openDatabase(p, target.Database)
	if err != nil {
		return nil, err
	}
	// Opening the pool is lazy; network I/O happens outside the manager lock.
	pool := &databasePool{db: db, profile: p}
	m.pools[key] = pool
	return pool, nil
}

func databaseError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": "database_error", "detail": err.Error()})
}
func decodeDatabaseRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	d := json.NewDecoder(r.Body)
	d.UseNumber()
	return d.Decode(dst)
}
func targetFromQuery(r *http.Request) databaseTarget {
	q := r.URL.Query()
	return databaseTarget{ID: q.Get("id"), Database: q.Get("database"), Schema: q.Get("schema"), Table: q.Get("table")}
}

func (s *server) handleDatabase(w http.ResponseWriter, r *http.Request) {
	if !requireFileBackend(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	action := strings.TrimPrefix(r.URL.Path, "/api/database/")
	if action == "progress" {
		handleDatabaseProgress(w, r)
		return
	}
	if id := r.URL.Query().Get("request_id"); id != "" && (action == "query" || action == "table") {
		progress, err := beginDatabaseProgress(id)
		if err != nil {
			databaseError(w, 400, err)
			return
		}
		defer finishDatabaseProgress(progress)
		ctx = context.WithValue(ctx, databaseProgressKey{}, progress)
		r = r.WithContext(ctx)
	}

	if action == "password" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		databases.mu.Lock()
		defer databases.mu.Unlock()
		profiles, err := readDatabaseProfiles()
		if err != nil {
			databaseError(w, 500, err)
			return
		}
		for _, p := range profiles {
			if p.ID == r.URL.Query().Get("id") {
				writeJSON(w, 200, map[string]string{"password": p.Password})
				return
			}
		}
		databaseError(w, 404, fmt.Errorf("connection not found"))
		return
	}
	if action == "profiles" {
		handleDatabaseProfiles(w, r)
		return
	}
	if action == "test" {
		handleDatabaseTest(w, r)
		return
	}
	if action == "query" {
		handleDatabaseQuery(w, r)
		return
	}
	target := targetFromQuery(r)
	pool, err := databases.pool(ctx, target)
	if err != nil {
		databaseError(w, 400, err)
		return
	}
	if target.Database == "" {
		target.Database = pool.profile.Database
	}
	if pool.profile.Engine == "sqlite" {
		target.Database = "main"
	}
	if pool.profile.Engine == "postgres" && target.Database == "" {
		target.Database = "postgres"
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if err = pool.db.PingContext(ctx); err != nil {
		databaseError(w, 502, err)
		return
	}
	switch action {
	case "catalog":
		err = handleDatabaseCatalog(w, r, pool, target)
	case "objects":
		err = handleDatabaseObjects(w, r, pool, target)
	case "table":
		err = handleDatabaseTable(w, r, pool, target)
	case "structure":
		err = handleDatabaseStructure(w, r, pool, target)
	case "changes":
		if r.Method != "POST" {
			err = errors.New("POST required")
		} else {
			err = handleDatabaseChanges(w, r, pool, target)
		}
	case "schema":
		if r.Method != "POST" {
			err = errors.New("POST required")
		} else {
			err = handleDatabaseSchema(w, r, pool, target)
		}
	default:
		databaseError(w, 404, errors.New("unknown database action"))
		return
	}
	if err != nil {
		databaseError(w, 400, err)
	}
}

func handleDatabaseProfiles(w http.ResponseWriter, r *http.Request) {
	databases.mu.Lock()
	defer databases.mu.Unlock()
	profiles, err := readDatabaseProfiles()
	if err != nil {
		databaseError(w, 500, err)
		return
	}
	switch r.Method {
	case "GET":
		for i := range profiles {
			profiles[i].HasPassword = profiles[i].Password != ""
			profiles[i].Password = ""
		}
		writeJSON(w, 200, profiles)
		return
	case "POST":
		var req struct {
			databaseProfile
			Password *string `json:"password"`
		}
		if err = decodeDatabaseRequest(w, r, &req); err != nil {
			databaseError(w, 400, err)
			return
		}
		p := req.databaseProfile
		index := -1
		for i, old := range profiles {
			if old.ID == p.ID {
				index = i
				p.Password = old.Password
				break
			}
		}
		if req.Password != nil {
			p.Password = *req.Password
		}
		p, err = databaseConfig(p)
		if err != nil {
			databaseError(w, 400, err)
			return
		}
		if p.ID == "" {
			p.ID = newTermID()
		}
		p.HasPassword = false
		if index < 0 {
			profiles = append(profiles, p)
		} else {
			profiles[index] = p
		}
		if err = os.MkdirAll(configDir(), 0o700); err == nil {
			err = writeJSONFile(databaseProfilesPath(), profiles)
		}
		if err != nil {
			databaseError(w, 500, err)
			return
		}
		databases.closePools(p.ID)
		p.HasPassword = p.Password != ""
		p.Password = ""
		writeJSON(w, 200, p)
	case "PUT":
		// Persist the order chosen by dragging connections in the tree.
		var req struct {
			Order []string `json:"order"`
		}
		if err = decodeDatabaseRequest(w, r, &req); err != nil {
			databaseError(w, 400, err)
			return
		}
		byID := make(map[string]databaseProfile, len(profiles))
		for _, p := range profiles {
			byID[p.ID] = p
		}
		ordered := make([]databaseProfile, 0, len(profiles))
		for _, id := range req.Order {
			if p, ok := byID[id]; ok {
				ordered = append(ordered, p)
				delete(byID, id)
			}
		}
		// Connections missing from the request keep their relative order at the end.
		for _, p := range profiles {
			if _, ok := byID[p.ID]; ok {
				ordered = append(ordered, p)
			}
		}
		if err = writeJSONFile(databaseProfilesPath(), ordered); err != nil {
			databaseError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "DELETE":
		id := r.URL.Query().Get("id")
		found := false
		kept := profiles[:0]
		for _, p := range profiles {
			if p.ID == id {
				found = true
			} else {
				kept = append(kept, p)
			}
		}
		if !found {
			databaseError(w, 404, errors.New("connection does not exist"))
			return
		}
		if err = writeJSONFile(databaseProfilesPath(), kept); err != nil {
			databaseError(w, 500, err)
			return
		}
		databases.closePools(id)
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		databaseError(w, 405, errors.New("unsupported method"))
	}
}

func handleDatabaseTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		databaseError(w, 405, errors.New("POST required"))
		return
	}
	var req struct {
		databaseProfile
		Password *string `json:"password"`
	}
	if err := decodeDatabaseRequest(w, r, &req); err != nil {
		databaseError(w, 400, err)
		return
	}
	p, err := databaseConfig(req.databaseProfile)
	if err != nil {
		databaseError(w, 400, err)
		return
	}
	if req.Password != nil {
		p.Password = *req.Password
	} else if p.ID != "" {
		databases.mu.Lock()
		profiles, e := readDatabaseProfiles()
		databases.mu.Unlock()
		if e != nil {
			databaseError(w, 500, e)
			return
		}
		for _, old := range profiles {
			if old.ID == p.ID {
				p.Password = old.Password
			}
		}
	}
	db, err := openDatabase(p, p.Database)
	if err == nil {
		defer db.Close()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		err = db.PingContext(ctx)
	}
	if err != nil {
		databaseError(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type databaseColumn struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Nullable  bool    `json:"nullable"`
	Primary   bool    `json:"primary"`
	Default   *string `json:"default,omitempty"`
	Generated bool    `json:"generated,omitempty"`
}
type databaseResult struct {
	Columns   []databaseColumn `json:"columns"`
	Rows      [][]any          `json:"rows"`
	Truncated bool             `json:"truncated"`
}

func readDatabaseRows(rows *sql.Rows, limit int, progress ...*databaseLoadProgress) (databaseResult, error) {
	defer rows.Close()
	result := databaseResult{Columns: []databaseColumn{}, Rows: [][]any{}}
	types, err := rows.ColumnTypes()
	if err != nil {
		return result, err
	}
	if len(types) > 512 {
		return result, errors.New("result exceeds 512 columns")
	}
	for _, c := range types {
		nullable, _ := c.Nullable()
		result.Columns = append(result.Columns, databaseColumn{Name: c.Name(), Type: c.DatabaseTypeName(), Nullable: nullable})
	}
	size := 0
	for rows.Next() {
		if len(result.Rows) >= limit {
			result.Truncated = true
			break
		}
		values := make([]any, len(types))
		ptrs := make([]any, len(types))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err = rows.Scan(ptrs...); err != nil {
			return result, err
		}
		for i, v := range values {
			switch value := v.(type) {
			case []byte:
				t := strings.ToUpper(types[i].DatabaseTypeName())
				if strings.Contains(t, "BLOB") || strings.Contains(t, "BINARY") || t == "BYTEA" {
					values[i] = map[string]string{"blob": base64.StdEncoding.EncodeToString(value)}
				} else {
					values[i] = string(value)
				}
			case time.Time:
				values[i] = value.Format(time.RFC3339Nano)
			case int64, float64:
				values[i] = fmt.Sprint(value)
			}
		}
		encoded, _ := json.Marshal(values)
		size += len(encoded)
		if size > 8<<20 {
			result.Truncated = true
			break
		}
		result.Rows = append(result.Rows, values)
		if len(progress) > 0 && progress[0] != nil {
			progress[0].rows.Add(1)
		}
	}
	return result, rows.Err()
}
func queryDatabase(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, sqlText string, limit int, args ...any) (databaseResult, error) {
	rows, err := q.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return databaseResult{}, err
	}
	return readDatabaseRows(rows, limit, databaseProgressFromContext(ctx))
}

type databaseSourceResult struct {
	Edit         *databaseQueryEdit `json:"edit,omitempty"`
	Target       databaseTarget     `json:"target"`
	Name         string             `json:"name"`
	Engine       string             `json:"engine"`
	Result       databaseResult     `json:"result"`
	Error        string             `json:"error,omitempty"`
	Milliseconds int64              `json:"milliseconds"`
	RowCount     int                `json:"rowCount"`
	Affected     *int64             `json:"affected,omitempty"`
	Executed     bool               `json:"executed"`
}
type databaseMergedRow struct {
	Source int   `json:"source"`
	Values []any `json:"values"`
}
type databaseQueryResponse struct {
	Columns  []databaseColumn       `json:"columns"`
	Rows     []databaseMergedRow    `json:"rows"`
	Sources  []databaseSourceResult `json:"sources"`
	Affected int64                  `json:"affected"`
	Mode     string                 `json:"mode"`
}

func databaseReadQuery(ctx context.Context, pool *databasePool, text string, limit int) (databaseResult, error) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.profile.Engine == "sqlite" {
		conn, err := pool.db.Conn(ctx)
		if err != nil {
			return databaseResult{}, err
		}
		defer conn.Close()
		if _, err = conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
			return databaseResult{}, err
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = conn.ExecContext(cleanup, "PRAGMA query_only=OFF")
		}()
		return queryDatabase(ctx, conn, text, limit)
	}
	tx, err := pool.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return databaseResult{}, err
	}
	defer tx.Rollback()
	return queryDatabase(ctx, tx, text, limit)
}

func mergeDatabaseResults(sources []databaseSourceResult) databaseQueryResponse {
	response := databaseQueryResponse{Columns: []databaseColumn{}, Rows: []databaseMergedRow{}, Sources: sources}
	positions := map[string]int{}
	for _, source := range sources {
		if source.Error != "" {
			continue
		}
		occurrences := map[string]int{}
		for _, c := range source.Result.Columns {
			key := fmt.Sprintf("%s\x00%d", c.Name, occurrences[c.Name])
			occurrences[c.Name]++
			if pos, ok := positions[key]; ok {
				if response.Columns[pos].Type != c.Type {
					response.Columns[pos].Type = "mixed"
				}
			} else {
				positions[key] = len(response.Columns)
				response.Columns = append(response.Columns, c)
			}
		}
	}
	for n, source := range sources {
		if source.Error != "" {
			continue
		}
		mapping := make([]int, len(source.Result.Columns))
		occurrences := map[string]int{}
		for i, c := range source.Result.Columns {
			key := fmt.Sprintf("%s\x00%d", c.Name, occurrences[c.Name])
			occurrences[c.Name]++
			mapping[i] = positions[key]
		}
		for _, row := range source.Result.Rows {
			values := make([]any, len(response.Columns))
			for i, v := range row {
				values[mapping[i]] = v
			}
			response.Rows = append(response.Rows, databaseMergedRow{Source: n, Values: values})
		}
	}
	for i := range response.Sources {
		if response.Sources[i].Error == "" && response.Sources[i].Affected != nil {
			response.Affected += *response.Sources[i].Affected
		}
		response.Sources[i].RowCount = len(response.Sources[i].Result.Rows)
		response.Sources[i].Result.Rows = nil
	}
	return response
}

func handleDatabaseQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		databaseError(w, 405, errors.New("POST required"))
		return
	}
	var req struct {
		Targets []databaseTarget `json:"targets"`
		SQL     string           `json:"sql"`
		Mode    string           `json:"mode"`
		Limit   int              `json:"limit"`
	}
	if err := decodeDatabaseRequest(w, r, &req); err != nil {
		databaseError(w, 400, err)
		return
	}
	stmts, err := migrate.Stmts(req.SQL)
	if err != nil || len(stmts) != 1 {
		databaseError(w, 400, errors.New("select exactly one SQL statement to execute"))
		return
	}
	if len(req.Targets) < 1 || len(req.Targets) > 16 {
		databaseError(w, 400, errors.New("choose between 1 and 16 database targets"))
		return
	}
	if req.Limit < 1 {
		req.Limit = 500
	}
	if req.Limit > 2500 {
		req.Limit = 2500
	}
	text := stmts[0].Text
	if req.Mode != "" && req.Mode != "query" && req.Mode != "execute" && req.Mode != "run" {
		databaseError(w, 400, errors.New("unknown SQL execution mode"))
		return
	}
	writable := req.Mode == "execute" || req.Mode == "run"
	command, _ := databaseStatementKind(text)
	statementExecution := writable
	switch command {
	case "SELECT", "VALUES", "TABLE", "EXPLAIN", "SHOW", "DESCRIBE", "DESC", "PRAGMA":
		statementExecution = false
	}
	// Query mode never executes writes, including data-changing CTEs. The
	// database enforces read-only mode; this allowlist also excludes session and
	// PRAGMA commands that could disable the SQLite guard.
	fields := strings.Fields(text)
	if len(fields) == 0 {
		databaseError(w, 400, errors.New("SQL is empty"))
		return
	}
	keyword := strings.ToUpper(fields[0])
	if !writable && keyword != "SELECT" && keyword != "WITH" && keyword != "EXPLAIN" && keyword != "SHOW" && keyword != "DESCRIBE" && keyword != "DESC" && keyword != "VALUES" && keyword != "TABLE" {
		databaseError(w, 400, errors.New("query mode accepts result queries; use statement execution for writes"))
		return
	}
	if writable {
		// Validate duplicate destinations before making any changes, including
		// equivalent default-database/SQLite-main aliases.
		profiles, e := readDatabaseProfiles()
		if e != nil {
			databaseError(w, 400, e)
			return
		}
		byID := make(map[string]databaseProfile, len(profiles))
		for _, profile := range profiles {
			byID[profile.ID] = profile
		}
		seen := map[string]bool{}
		for _, target := range req.Targets {
			profile := byID[target.ID]
			database := target.Database
			if database == "" {
				database = profile.Database
			}
			if profile.Engine == "sqlite" {
				database = "main"
			}
			identity := target.ID + "\x00" + database
			if seen[identity] {
				databaseError(w, 400, errors.New("duplicate database target; no statements were executed"))
				return
			}
			seen[identity] = true
		}
	}
	sources := make([]databaseSourceResult, len(req.Targets))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, target := range req.Targets {
		wg.Add(1)
		go func(i int, target databaseTarget) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-r.Context().Done():
				sources[i] = databaseSourceResult{Target: target, Error: r.Context().Err().Error()}
				return
			}
			start := time.Now()
			source := databaseSourceResult{Target: target}
			pool, e := databases.pool(r.Context(), target)
			if e == nil {
				source.Name, source.Engine = pool.profile.Name, pool.profile.Engine
				if source.Target.Database == "" {
					source.Target.Database = pool.profile.Database
				}
				if source.Engine == "sqlite" {
					source.Target.Database = "main"
				}
				ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
				if writable {
					source.Result, source.Affected, e = databaseRunStatement(ctx, pool, text, req.Limit)
					source.Executed = statementExecution
				} else {
					source.Result, e = databaseReadQuery(ctx, pool, text, req.Limit)
				}
				if e == nil && !source.Executed {
					source.Edit = describeDatabaseQueryEdit(ctx, pool, source.Target, text, source.Result)
				}
				cancel()
			}
			if e != nil {
				source.Error = e.Error()
			}
			source.Milliseconds = time.Since(start).Milliseconds()
			sources[i] = source
		}(i, target)
	}
	wg.Wait()
	response := mergeDatabaseResults(sources)
	if statementExecution {
		response.Mode = "execute"
	} else {
		response.Mode = "query"
	}
	writeJSON(w, 200, response)
}

func databaseQuote(engine, name string) string {
	quote := "\""
	if engine == "mysql" {
		quote = "`"
	}
	return quote + strings.ReplaceAll(name, quote, quote+quote) + quote
}
func databaseTableName(engine string, target databaseTarget) string {
	schemaName := target.Schema
	if engine == "mysql" {
		schemaName = target.Database
	}
	if engine == "sqlite" {
		schemaName = "main"
	}
	if schemaName == "" {
		schemaName = "public"
	}
	return databaseQuote(engine, schemaName) + "." + databaseQuote(engine, target.Table)
}
func databasePlaceholder(engine string, n int) string {
	if engine == "postgres" {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

func handleDatabaseCatalog(w http.ResponseWriter, r *http.Request, pool *databasePool, target databaseTarget) error {
	text := "SHOW DATABASES"
	if pool.profile.Engine == "postgres" {
		text = "SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate ORDER BY datname"
	}
	if pool.profile.Engine == "sqlite" {
		writeJSON(w, 200, []string{"main"})
		return nil
	}
	rows, err := pool.db.QueryContext(r.Context(), text)
	if err != nil {
		return err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	sort.Strings(names)
	writeJSON(w, 200, names)
	return nil
}

type databaseObject struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

func handleDatabaseObjects(w http.ResponseWriter, r *http.Request, pool *databasePool, target databaseTarget) error {
	text := ""
	args := []any{}
	switch pool.profile.Engine {
	case "sqlite":
		text = "SELECT 'main', name, type FROM sqlite_schema WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name"
	case "mysql":
		text = "SELECT table_schema,table_name,CASE WHEN table_type='VIEW' THEN 'view' ELSE 'table' END FROM information_schema.tables WHERE table_schema=? ORDER BY table_name"
		db := target.Database
		if db == "" {
			db = pool.profile.Database
		}
		args = append(args, db)
	case "postgres":
		text = "SELECT table_schema,table_name,CASE WHEN table_type='VIEW' THEN 'view' ELSE 'table' END FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') ORDER BY table_schema,table_name"
	}
	rows, err := pool.db.QueryContext(r.Context(), text, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	objects := []databaseObject{}
	for rows.Next() {
		var o databaseObject
		if err = rows.Scan(&o.Schema, &o.Name, &o.Kind); err != nil {
			return err
		}
		objects = append(objects, o)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	writeJSON(w, 200, objects)
	return nil
}
