package db

import (
	"strconv"
	"strings"
	"unicode"
)

// The API's queries are written in the MySQL dialect WordPress/WooCommerce
// use (they run unchanged next to a WooCommerce shop). On PostgreSQL they go
// through Translate, at the driver level, so the stores don't need two
// versions of each query. It handles what cwmerch's queries use:
//
//   - `?` placeholders → $1, $2...; `back-quoted` identifiers → "lowercase"
//     (the PostgreSQL schema is created with lowercase names);
//   - MySQL string literals (backslash escapes, "double-quoted" strings);
//   - UTC_TIMESTAMP(), CAST(x AS SIGNED|UNSIGNED|DECIMAL|CHAR), IF(a,b,c),
//     JSON_UNQUOTE(JSON_EXTRACT(x, '$.a.b')), FIELD(x, ...);
//   - INSERT IGNORE, REPLACE INTO and ON DUPLICATE KEY UPDATE (→ ON CONFLICT,
//     the conflict target being the table's unique key the insert covers).
//
// Text columns are citext on PostgreSQL, so comparisons and LIKE stay
// case-insensitive like MySQL's *_ci collations.

type tokKind int

const (
	tWS tokKind = iota
	tWord
	tQIdent
	tString
	tNum
	tParam
	tPunct
)

type token struct {
	kind tokKind
	text string // tWord/tPunct/tNum/tWS: as written; tQIdent/tString: the decoded value
}

func (t token) is(word string) bool { return t.kind == tWord && strings.EqualFold(t.text, word) }

// tokenize splits a MySQL query.
func tokenize(q string) []token {
	var out []token
	i := 0
	for i < len(q) {
		c := q[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			j := i
			for j < len(q) && (q[j] == ' ' || q[j] == '\t' || q[j] == '\n' || q[j] == '\r') {
				j++
			}
			out = append(out, token{tWS, q[i:j]})
			i = j
		case c == '-' && i+1 < len(q) && q[i+1] == '-':
			j := strings.IndexByte(q[i:], '\n')
			if j < 0 {
				j = len(q) - i
			}
			out = append(out, token{tWS, " "})
			i += j
		case c == '/' && i+1 < len(q) && q[i+1] == '*':
			j := strings.Index(q[i+2:], "*/")
			if j < 0 {
				i = len(q)
			} else {
				i += j + 4
			}
			out = append(out, token{tWS, " "})
		case c == '\'' || c == '"':
			var sb strings.Builder
			j := i + 1
			for j < len(q) {
				ch := q[j]
				if ch == '\\' && j+1 < len(q) {
					n := q[j+1]
					switch n {
					case 'n':
						sb.WriteByte('\n')
					case 't':
						sb.WriteByte('\t')
					case 'r':
						sb.WriteByte('\r')
					case '0':
						sb.WriteByte(0)
					case '%', '_': // MySQL keeps the backslash for LIKE
						sb.WriteByte('\\')
						sb.WriteByte(n)
					default:
						sb.WriteByte(n)
					}
					j += 2
					continue
				}
				if ch == c {
					if j+1 < len(q) && q[j+1] == c {
						sb.WriteByte(c)
						j += 2
						continue
					}
					break
				}
				sb.WriteByte(ch)
				j++
			}
			out = append(out, token{tString, sb.String()})
			i = j + 1
		case c == '`':
			j := strings.IndexByte(q[i+1:], '`')
			if j < 0 {
				j = len(q) - i - 1
			}
			out = append(out, token{tQIdent, q[i+1 : i+1+j]})
			i += j + 2
		case c == '?':
			out = append(out, token{tParam, "?"})
			i++
		case c >= '0' && c <= '9':
			j := i
			for j < len(q) && (q[j] >= '0' && q[j] <= '9' || q[j] == '.' || q[j] == 'e' || q[j] == 'E') {
				j++
			}
			out = append(out, token{tNum, q[i:j]})
			i = j
		case c == '_' || c == '$' || c == '@' || unicode.IsLetter(rune(c)) || c >= 0x80:
			j := i
			for j < len(q) && (q[j] == '_' || q[j] == '$' || q[j] == '@' || q[j] == '%' && j > i || q[j] >= '0' && q[j] <= '9' || unicode.IsLetter(rune(q[j])) || q[j] >= 0x80) {
				j++
			}
			out = append(out, token{tWord, q[i:j]})
			i = j
		default:
			if i+1 < len(q) {
				two := q[i : i+2]
				if two == "<=" || two == ">=" || two == "<>" || two == "!=" || two == "::" || two == "||" || two == "->" {
					if two == "<=" && i+2 < len(q) && q[i+2] == '>' {
						out = append(out, token{tPunct, "<=>"})
						i += 3
						continue
					}
					out = append(out, token{tPunct, two})
					i += 2
					continue
				}
			}
			out = append(out, token{tPunct, string(c)})
			i++
		}
	}
	return out
}

// Catalog answers what ON CONFLICT and RETURNING need about a table.
type Catalog interface {
	// UniqueKeys lists the unique indexes' columns, the primary key first.
	UniqueKeys(table string) (primary []string, others [][]string)
	// Identity is the auto-increment column ("" if none).
	Identity(table string) string
}

// Translated is a query rewritten for PostgreSQL.
type Translated struct {
	SQL string
	// ReturningID: an INSERT into a table with an identity column; the
	// statement ends with RETURNING <col> so LastInsertId can be answered.
	ReturningID bool
}

// Translate rewrites a MySQL query for PostgreSQL.
func Translate(q string, cat Catalog) Translated {
	toks := tokenize(q)
	n := 0
	for i := range toks {
		if toks[i].kind == tParam {
			n++
			toks[i] = token{tPunct, "$" + strconv.Itoa(n)}
		}
	}
	toks = rewrite(toks)
	return statement(toks, cat)
}

// render prints tokens as PostgreSQL.
func render(toks []token) string {
	var sb strings.Builder
	for _, t := range toks {
		switch t.kind {
		case tQIdent:
			sb.WriteString(`"` + strings.ToLower(strings.ReplaceAll(t.text, `"`, `""`)) + `"`)
		case tString:
			sb.WriteString("'" + strings.ReplaceAll(strings.ReplaceAll(t.text, "'", "''"), "\x00", "") + "'")
		default:
			sb.WriteString(t.text)
		}
	}
	return sb.String()
}

func raw(s string) token { return token{tPunct, s} }

// matching returns the index of the parenthesis closing toks[open].
func matching(toks []token, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		if toks[i].kind != tPunct {
			continue
		}
		switch toks[i].text {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitArgs splits a call's argument tokens on top-level commas.
func splitArgs(toks []token) [][]token {
	var out [][]token
	depth, start := 0, 0
	for i, t := range toks {
		if t.kind != tPunct {
			continue
		}
		switch t.text {
		case "(":
			depth++
		case ")":
			depth--
		case ",":
			if depth == 0 {
				out = append(out, toks[start:i])
				start = i + 1
			}
		}
	}
	return append(out, toks[start:])
}

func nextNonWS(toks []token, i int) int {
	for i < len(toks) && toks[i].kind == tWS {
		i++
	}
	return i
}

func prevNonWS(toks []token, i int) int {
	for i >= 0 && toks[i].kind == tWS {
		i--
	}
	return i
}

func trimWS(toks []token) []token {
	for len(toks) > 0 && toks[0].kind == tWS {
		toks = toks[1:]
	}
	for len(toks) > 0 && toks[len(toks)-1].kind == tWS {
		toks = toks[:len(toks)-1]
	}
	return toks
}

func join(parts ...[]token) []token {
	var out []token
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// rewrite handles function-level differences, inside out.
func rewrite(toks []token) []token {
	var out []token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != tWord {
			out = append(out, t)
			continue
		}
		p := nextNonWS(toks, i+1)
		isCall := p < len(toks) && toks[p].kind == tPunct && toks[p].text == "("
		prev := prevNonWS(toks, i-1)
		qualified := prev >= 0 && toks[prev].kind == tPunct && toks[prev].text == "."
		if !isCall || qualified {
			out = append(out, t)
			continue
		}
		end := matching(toks, p)
		if end < 0 {
			out = append(out, t)
			continue
		}
		inner := rewrite(toks[p+1 : end])
		var repl []token
		switch strings.ToUpper(t.text) {
		case "UTC_TIMESTAMP":
			repl = []token{raw("(now() AT TIME ZONE 'UTC')")}
		case "CAST":
			repl = castExpr(inner)
		case "IF":
			if args := splitArgs(inner); len(args) == 3 {
				repl = join([]token{raw("(CASE WHEN ")}, trimWS(args[0]), []token{raw(" THEN ")}, trimWS(args[1]), []token{raw(" ELSE ")}, trimWS(args[2]), []token{raw(" END)")})
			}
		case "JSON_UNQUOTE":
			if j := trimWS(inner); len(j) > 0 && j[0].kind == tPunct && strings.HasPrefix(j[0].text, "(CAST(") && strings.Contains(j[len(j)-1].text, "#>") {
				// JSON_UNQUOTE(JSON_EXTRACT(...)): already rewritten, make it text
				repl = join(j[:len(j)-1], []token{raw(strings.Replace(j[len(j)-1].text, "#>", "#>>", 1))})
			} else {
				repl = join([]token{raw("(")}, inner, []token{raw(")")})
			}
		case "JSON_EXTRACT":
			if args := splitArgs(inner); len(args) == 2 {
				if path := trimWS(args[1]); len(path) == 1 && path[0].kind == tString {
					segs := strings.Split(strings.TrimPrefix(strings.TrimPrefix(path[0].text, "$"), "."), ".")
					repl = join([]token{raw("(CAST(")}, args[0], []token{raw(" AS jsonb) #> '{" + strings.Join(segs, ",") + "}')")})
				}
			}
		case "FIELD":
			if args := splitArgs(inner); len(args) > 1 {
				repl = []token{raw("(CASE ")}
				repl = append(repl, args[0]...)
				for k, a := range args[1:] {
					repl = append(repl, raw(" WHEN "))
					repl = append(repl, a...)
					repl = append(repl, raw(" THEN "+strconv.Itoa(k+1)))
				}
				repl = append(repl, raw(" ELSE 0 END)"))
			}
		}
		if repl == nil {
			repl = join([]token{t}, toks[i+1:p+1], inner, []token{toks[end]})
		}
		out = append(out, repl...)
		i = end
	}
	return out
}

// castExpr maps MySQL's CAST types. Text to number casts go through
// cwmerch's lenient cwm_int/cwm_num functions (V1): MySQL reads ” or
// '12abc' as 0 / 12 where PostgreSQL raises an error.
func castExpr(inner []token) []token {
	asAt := -1
	depth := 0
	for k, t := range inner {
		if t.kind == tPunct && t.text == "(" {
			depth++
		} else if t.kind == tPunct && t.text == ")" {
			depth--
		} else if depth == 0 && t.is("AS") {
			asAt = k
		}
	}
	if asAt < 0 {
		return join([]token{raw("CAST(")}, inner, []token{raw(")")})
	}
	expr := trimWS(inner[:asAt])
	typ := strings.ToUpper(strings.TrimSpace(render(inner[asAt+1:])))
	switch {
	case strings.HasPrefix(typ, "UNSIGNED"), strings.HasPrefix(typ, "SIGNED"):
		return join([]token{raw("cwm_int(")}, expr, []token{raw(")")})
	case strings.HasPrefix(typ, "DECIMAL"):
		return join([]token{raw("cwm_num(")}, expr, []token{raw(")")})
	case strings.HasPrefix(typ, "CHAR"):
		return join([]token{raw("CAST(")}, expr, []token{raw(" AS TEXT)")})
	}
	return join([]token{raw("CAST(")}, inner, []token{raw(")")})
}

// ident is a table/column name token's PostgreSQL name.
func ident(t token) string {
	if t.kind == tQIdent {
		return strings.ToLower(t.text)
	}
	return strings.ToLower(t.text)
}

// statement handles INSERT IGNORE / REPLACE / ON DUPLICATE KEY UPDATE and
// RETURNING for the identity column.
func statement(toks []token, cat Catalog) Translated {
	first := nextNonWS(toks, 0)
	if first >= len(toks) {
		return Translated{SQL: render(toks)}
	}
	verb := strings.ToUpper(toks[first].text)
	if toks[first].kind != tWord || (verb != "INSERT" && verb != "REPLACE") {
		return Translated{SQL: render(toks)}
	}
	ignore := false
	replace := verb == "REPLACE"
	i := nextNonWS(toks, first+1)
	if i < len(toks) && toks[i].is("IGNORE") {
		ignore = true
		toks = append(toks[:i:i], toks[nextNonWS(toks, i+1):]...)
		i = nextNonWS(toks, first+1)
	}
	if replace {
		toks[first] = token{tWord, "INSERT"}
	}
	if i >= len(toks) || !toks[i].is("INTO") {
		return Translated{SQL: render(toks)}
	}
	ti := nextNonWS(toks, i+1)
	if ti >= len(toks) {
		return Translated{SQL: render(toks)}
	}
	table := ident(toks[ti])
	// the insert's column list
	var cols []string
	if ci := nextNonWS(toks, ti+1); ci < len(toks) && toks[ci].kind == tPunct && toks[ci].text == "(" {
		if end := matching(toks, ci); end > 0 {
			for _, a := range splitArgs(toks[ci+1 : end]) {
				if a = trimWS(a); len(a) == 1 {
					cols = append(cols, ident(a[0]))
				}
			}
		}
	}
	// ON DUPLICATE KEY UPDATE
	var tail []token
	for k := 0; k < len(toks); k++ {
		if toks[k].is("ON") {
			a, b, c := nextNonWS(toks, k+1), 0, 0
			if a < len(toks) && toks[a].is("DUPLICATE") {
				b = nextNonWS(toks, a+1)
				c = nextNonWS(toks, b+1)
				if b < len(toks) && toks[b].is("KEY") && c < len(toks) && toks[c].is("UPDATE") {
					tail = toks[c+1:]
					toks = toks[:k]
					break
				}
			}
		}
	}
	target := conflictTarget(cat, table, cols)
	var conflict string
	switch {
	case tail != nil && len(target) > 0:
		// VALUES(col) → EXCLUDED.col
		var set []token
		for k := 0; k < len(tail); k++ {
			if tail[k].is("VALUES") {
				if p := nextNonWS(tail, k+1); p < len(tail) && tail[p].text == "(" {
					if end := matching(tail, p); end > 0 {
						set = append(set, raw("EXCLUDED."+ident(trimWS(tail[p+1 : end])[0])))
						k = end
						continue
					}
				}
			}
			set = append(set, tail[k])
		}
		conflict = " ON CONFLICT (" + strings.Join(target, ", ") + ") DO UPDATE SET" + render(set)
	case replace && len(target) > 0:
		var sets []string
		for _, c := range cols {
			if !contains(target, c) {
				sets = append(sets, c+" = EXCLUDED."+c)
			}
		}
		if len(sets) == 0 {
			conflict = " ON CONFLICT DO NOTHING"
		} else {
			conflict = " ON CONFLICT (" + strings.Join(target, ", ") + ") DO UPDATE SET " + strings.Join(sets, ", ")
		}
	case ignore || tail != nil || replace:
		conflict = " ON CONFLICT DO NOTHING"
	}
	sql := strings.TrimRight(strings.TrimSpace(render(toks)), ";") + conflict
	out := Translated{SQL: sql}
	hasReturning := false
	for _, t := range toks {
		if t.is("RETURNING") {
			hasReturning = true
		}
	}
	if !hasReturning {
		if id := cat.Identity(table); id != "" {
			out.SQL += " RETURNING " + id
			out.ReturningID = true
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// conflictTarget is the unique key an insert's columns cover, like the key
// MySQL would hit: another unique key first (the primary key is often a
// fresh id), else the primary key.
func conflictTarget(cat Catalog, table string, cols []string) []string {
	if cat == nil || len(cols) == 0 {
		return nil
	}
	primary, others := cat.UniqueKeys(table)
	covered := func(key []string) bool {
		if len(key) == 0 {
			return false
		}
		for _, k := range key {
			if !contains(cols, k) {
				return false
			}
		}
		return true
	}
	for _, k := range others {
		if covered(k) {
			return k
		}
	}
	if covered(primary) {
		return primary
	}
	return nil
}
