package main

import (
	"context"
	"errors"
	"strings"
)

// Only classify result transport here. The database validates the SQL itself.
// Ignore quoted values/identifiers, comments and nested expressions so a
// literal named RETURNING or a CTE's SELECT cannot change the execution path.
func databaseSQLWords(text, engine string) []string {
	var words []string
	depth := 0
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "--") || text[i] == '#' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(text[i:], "/*") {
			i += 2
			comments := 1
			for i < len(text) && comments > 0 {
				if strings.HasPrefix(text[i:], "/*") {
					comments++
					i += 2
				} else if strings.HasPrefix(text[i:], "*/") {
					comments--
					i += 2
				} else {
					i++
				}
			}
			continue
		}
		if text[i] == '\'' || text[i] == '"' || text[i] == '`' || text[i] == '[' {
			backslashEscapes := engine == "mysql" || engine == "postgres" && text[i] == '\'' && i > 0 && (text[i-1] == 'E' || text[i-1] == 'e')
			end := text[i]
			if end == '[' {
				end = ']'
			}
			i++
			for i < len(text) {
				if backslashEscapes && text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == end {
					i++
					if i < len(text) && text[i] == end {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}
		if text[i] == '$' {
			j := i + 1
			for j < len(text) && (text[j] == '_' || text[j] >= 'a' && text[j] <= 'z' || text[j] >= 'A' && text[j] <= 'Z' || text[j] >= '0' && text[j] <= '9') {
				j++
			}
			if j < len(text) && text[j] == '$' {
				delimiter := text[i : j+1]
				if end := strings.Index(text[j+1:], delimiter); end >= 0 {
					i = j + 1 + end + len(delimiter)
					continue
				}
			}
		}
		if text[i] == '(' {
			depth++
			i++
			continue
		}
		if text[i] == ')' {
			depth--
			i++
			continue
		}
		if text[i] >= 'a' && text[i] <= 'z' || text[i] >= 'A' && text[i] <= 'Z' || text[i] == '_' {
			start := i
			i++
			for i < len(text) && (text[i] >= 'a' && text[i] <= 'z' || text[i] >= 'A' && text[i] <= 'Z' || text[i] >= '0' && text[i] <= '9' || text[i] == '_') {
				i++
			}
			if depth == 0 {
				words = append(words, strings.ToUpper(text[start:i]))
			}
			continue
		}
		i++
	}
	return words
}

func databaseStatementKind(text string, engines ...string) (command string, rows bool) {
	engine := ""
	if len(engines) > 0 {
		engine = engines[0]
	}
	words := databaseSQLWords(text, engine)
	if len(words) == 0 {
		return "", false
	}
	command = words[0]
	if command == "WITH" {
		for _, word := range words[1:] {
			if word == "SELECT" || word == "VALUES" || word == "TABLE" || word == "INSERT" || word == "UPDATE" || word == "DELETE" || word == "MERGE" {
				command = word
				break
			}
		}
	}
	switch command {
	case "SELECT", "VALUES", "TABLE", "EXPLAIN", "SHOW", "DESCRIBE", "DESC", "PRAGMA", "CALL", "EXEC", "EXECUTE":
		rows = true
	}
	for _, word := range words {
		if word == "RETURNING" {
			rows = true
		}
	}
	return
}

func databaseRunStatement(ctx context.Context, pool *databasePool, text string, limit int) (databaseResult, *int64, error) {
	command, returnsRows := databaseStatementKind(text, pool.profile.Engine)
	// Requests do not hold a dedicated connection across separate executions.
	// Do not leave a transaction open on a pooled connection.
	words := databaseSQLWords(text, pool.profile.Engine)
	transactionControl := command == "START" && len(words) > 1 && words[1] == "TRANSACTION"
	switch command {
	case "BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE", "END":
		transactionControl = true
	}
	if transactionControl {
		return databaseResult{}, nil, errors.New("transaction-control statements require a persistent session; each SQL execution here uses database autocommit")
	}
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if returnsRows {
		result, err := queryDatabase(ctx, pool.db, text, limit)
		if err != nil {
			return result, nil, err
		}
		if pool.profile.Engine == "sqlite" && (command == "INSERT" || command == "UPDATE" || command == "DELETE" || command == "REPLACE") {
			var affected int64
			if err = pool.db.QueryRowContext(ctx, "SELECT changes()").Scan(&affected); err == nil {
				return result, &affected, nil
			}
		}
		return result, nil, nil
	}
	result, err := pool.db.ExecContext(ctx, text)
	if err != nil {
		return databaseResult{}, nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return databaseResult{}, nil, nil
	}
	if pool.profile.Engine == "sqlite" && command != "INSERT" && command != "UPDATE" && command != "DELETE" && command != "REPLACE" {
		affected = 0
	}
	return databaseResult{}, &affected, nil
}
