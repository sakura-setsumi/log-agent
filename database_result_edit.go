package main

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Each output label is mapped to its actual table column before enabling edits.
// Expressions, joins and incomplete keys never infer an update destination.
type databaseQueryEdit struct {
	Target  databaseTarget            `json:"target"`
	Columns map[string]databaseColumn `json:"columns"`
}
type databaseSelectToken struct {
	text               string
	identifier, quoted bool
}

func (t databaseSelectToken) word(s string) bool {
	return t.identifier && !t.quoted && strings.EqualFold(t.text, s)
}

func databaseSelectTokens(sql, engine string) ([]databaseSelectToken, bool) {
	var tokens []databaseSelectToken
	for i := 0; i < len(sql); {
		r, n := utf8.DecodeRuneInString(sql[i:])
		if unicode.IsSpace(r) {
			if r > 127 {
				return nil, false
			}
			i += n
			continue
		}
		if (strings.HasPrefix(sql[i:], "--") && (engine != "mysql" || i+2 == len(sql) || sql[i+2] <= 32)) || engine == "mysql" && sql[i] == '#' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if strings.HasPrefix(sql[i:], "/*") {
			// Executable MySQL comments may change the statement's meaning.
			if strings.HasPrefix(sql[i:], "/*!") || len(sql[i:]) >= 4 && strings.EqualFold(sql[i:i+4], "/*M!") {
				return nil, false
			}
			i += 2
			depth := 1
			for i < len(sql) && depth > 0 {
				if strings.HasPrefix(sql[i:], "/*") {
					if engine != "postgres" {
						return nil, false
					}
					depth++
					i += 2
				} else if strings.HasPrefix(sql[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return nil, false
			}
			continue
		}
		if sql[i] == '$' {
			return nil, false
		}
		if sql[i] == '\'' || sql[i] == '"' || sql[i] == '`' || sql[i] == '[' {
			start := sql[i]
			end := start
			if end == '[' {
				end = ']'
			}
			i++
			var value strings.Builder
			closed := false
			for i < len(sql) {
				if sql[i] == end {
					i++
					if i < len(sql) && sql[i] == end {
						value.WriteByte(end)
						i++
						continue
					}
					closed = true
					break
				}
				if sql[i] == '\\' && (engine == "mysql" || start == '\'') {
					return nil, false
				}
				value.WriteByte(sql[i])
				i++
			}
			if !closed {
				return nil, false
			}
			identifier := start != '\'' && !(engine == "mysql" && start == '"')
			tokens = append(tokens, databaseSelectToken{text: value.String(), identifier: identifier, quoted: true})
			continue
		}
		if unicode.IsLetter(r) || r == '_' {
			start := i
			i += n
			for i < len(sql) {
				r, n = utf8.DecodeRuneInString(sql[i:])
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
					break
				}
				i += n
			}
			name := sql[start:i]
			if engine == "postgres" {
				name = strings.ToLower(name)
			}
			tokens = append(tokens, databaseSelectToken{text: name, identifier: true})
			continue
		}
		tokens = append(tokens, databaseSelectToken{text: sql[i : i+1]})
		i++
	}
	return tokens, true
}

type databaseSelectProjection struct {
	name, output string
	star         bool
}

func databaseEditableSelect(sql, engine string) (qualifier, table string, projection []databaseSelectProjection, ok bool) {
	tokens, valid := databaseSelectTokens(sql, engine)
	if !valid || len(tokens) < 4 || !tokens[0].word("SELECT") {
		return
	}
	from := -1
	for i, t := range tokens {
		if t.word("FROM") {
			if from != -1 {
				return
			}
			from = i
		}
		if i > 0 && t.word("SELECT") {
			return
		}
		for _, word := range []string{"DISTINCT", "JOIN", "UNION", "INTERSECT", "EXCEPT", "GROUP", "HAVING", "WINDOW", "INTO", "RETURNING"} {
			if t.word(word) {
				return
			}
		}
		if t.text == ";" && i != len(tokens)-1 {
			return
		}
	}
	if from < 2 || from+1 >= len(tokens) {
		return
	}
	at := from + 1
	if !tokens[at].identifier {
		return
	}
	table = tokens[at].text
	at++
	if at < len(tokens) && tokens[at].text == "." {
		at++
		if at >= len(tokens) || !tokens[at].identifier {
			return
		}
		qualifier, table = table, tokens[at].text
		at++
	}
	alias := ""
	clause := func(t databaseSelectToken) bool {
		return t.word("WHERE") || t.word("ORDER") || t.word("LIMIT") || t.word("OFFSET") || t.word("FETCH") || t.text == ";"
	}
	if at < len(tokens) && tokens[at].word("AS") {
		at++
		if at >= len(tokens) || !tokens[at].identifier {
			return
		}
		alias = tokens[at].text
		at++
	} else if at < len(tokens) && tokens[at].identifier && !clause(tokens[at]) {
		alias = tokens[at].text
		at++
	}
	if at < len(tokens) && !clause(tokens[at]) {
		return
	}
	// A second table after a comma is only legal in FROM, which is rejected above.
	qualifierMatches := func(name string) bool { return name == table || alias != "" && name == alias }
	start := 1
	if tokens[start].word("ALL") {
		start++
	}
	for start < from {
		end := start
		for end < from && tokens[end].text != "," {
			end++
		}
		part := tokens[start:end]
		if len(part) == 0 {
			return
		}
		p := databaseSelectProjection{}
		pos := 0
		if len(part) > 2 && part[0].identifier && part[1].text == "." {
			if !qualifierMatches(part[0].text) {
				return
			}
			pos = 2
		}
		if pos >= len(part) {
			return
		}
		if part[pos].text == "*" && !part[pos].quoted {
			p.star = true
		} else if part[pos].identifier {
			p.name = part[pos].text
			p.output = p.name
		} else {
			return
		}
		pos++
		if pos < len(part) && part[pos].word("AS") {
			pos++
		}
		if pos < len(part) {
			if p.star || !part[pos].identifier {
				return
			}
			p.output = part[pos].text
			pos++
		}
		if pos != len(part) {
			return
		}
		projection = append(projection, p)
		start = end + 1
		if end == from-1 && tokens[end].text == "," {
			return
		}
	}
	ok = len(projection) > 0
	return
}

func describeDatabaseQueryEdit(ctx context.Context, pool *databasePool, target databaseTarget, sql string, result databaseResult) *databaseQueryEdit {
	qualifier, table, projection, ok := databaseEditableSelect(sql, pool.profile.Engine)
	if !ok || len(result.Columns) == 0 {
		return nil
	}
	target.Table = table
	switch pool.profile.Engine {
	case "mysql":
		if qualifier != "" {
			target.Database = qualifier
		}
		target.Schema = target.Database
	case "postgres":
		// Resolve using the same search_path as the query, including user schemas.
		name := databaseQuote("postgres", table)
		if qualifier != "" {
			name = databaseQuote("postgres", qualifier) + "." + name
		}
		if err := pool.db.QueryRowContext(ctx, "SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=to_regclass($1)", name).Scan(&target.Schema); err != nil {
			return nil
		}
	case "sqlite":
		if qualifier != "" && qualifier != "main" {
			return nil
		}
		if qualifier == "" {
			var count int
			if err := pool.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_temp_master WHERE name=? COLLATE NOCASE", table).Scan(&count); err != nil || count != 0 {
				return nil
			}
		}
		target.Schema = "main"
	default:
		return nil
	}
	driver, err := databaseAtlas(pool.db, pool.profile.Engine)
	if err != nil {
		return nil
	}
	inspected, err := inspectDatabaseTable(ctx, driver, pool.profile.Engine, target)
	if err != nil {
		return nil
	}
	meta := describeDatabaseTable(pool.profile.Engine, inspected)
	columns := map[string]databaseColumn{}
	primary := map[string]bool{}
	for _, c := range meta.Columns {
		columns[c.Name] = c
		if c.Primary {
			primary[c.Name] = true
		}
	}
	if len(primary) == 0 {
		return nil
	}
	// Expand stars from inspected metadata, then verify the exact driver output.
	expanded := []databaseSelectProjection{}
	for _, p := range projection {
		if p.star {
			for _, c := range meta.Columns {
				expanded = append(expanded, databaseSelectProjection{name: c.Name, output: c.Name})
			}
		} else {
			expanded = append(expanded, p)
		}
	}
	if len(expanded) != len(result.Columns) {
		return nil
	}
	edit := &databaseQueryEdit{Target: target, Columns: map[string]databaseColumn{}}
	seen := map[string]bool{}
	for i, p := range expanded {
		c, exists := columns[p.name]
		if !exists || seen[c.Name] || result.Columns[i].Name != p.output {
			return nil
		}
		if _, duplicate := edit.Columns[p.output]; duplicate {
			return nil
		}
		edit.Columns[p.output] = c
		seen[c.Name] = true
	}
	for name := range primary {
		if !seen[name] {
			return nil
		}
	}
	return edit
}
