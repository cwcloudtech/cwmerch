#!/usr/bin/env python3
"""Generates cwmerch-db/postgresql/*.sql from cwmerch-db/mysql/*.sql.

The MySQL migrations are the reference (they reproduce WordPress/WooCommerce's
own schema); this keeps the PostgreSQL ones equivalent:

* identifiers lowercase; text columns are citext (case-insensitive like
  MySQL's *_ci collations), datetime -> timestamp, AUTO_INCREMENT -> identity,
  JSON -> jsonb, BLOB/VARBINARY -> bytea, unsigned ints widened;
* MySQL zero dates '0000-00-00 00:00:00' -> '0001-01-01 00:00:00' (Go's zero
  time, what the API reads zero dates as on MySQL too);
* prefix indexes on long text -> expression indexes on left(col, n);
  index names are made unique per schema;
* ON UPDATE CURRENT_TIMESTAMP -> a trigger;
* INSERT IGNORE -> ON CONFLICT DO NOTHING, MySQL string escapes -> standard.

Usage: python3 ci/tools/mysql2pg.py   (from the repository root)
"""
import hashlib, os, re, sys

ROOT = os.path.join(os.path.dirname(__file__), '..', '..', 'cwmerch-db')
SRC, DST = os.path.join(ROOT, 'mysql'), os.path.join(ROOT, 'postgresql')
P = '${table_prefix}'

def tokens(sql):
    """MySQL tokenizer: (kind, text) with kind in ws, word, qident, str, punct, comment."""
    i, out = 0, []
    while i < len(sql):
        c = sql[i]
        if c in ' \t\r\n':
            j = i
            while j < len(sql) and sql[j] in ' \t\r\n': j += 1
            out.append(('ws', sql[i:j])); i = j
        elif sql.startswith('--', i):
            j = sql.find('\n', i); j = len(sql) if j < 0 else j
            out.append(('comment', sql[i:j])); i = j
        elif sql.startswith('/*', i):
            j = sql.find('*/', i) + 2
            out.append(('comment', sql[i:j])); i = j
        elif c in "'\"":
            j, buf = i + 1, []
            while j < len(sql):
                ch = sql[j]
                if ch == '\\':
                    n = sql[j + 1]
                    buf.append({'n': '\n', 't': '\t', 'r': '\r', '0': ''}.get(n, ('\\' + n) if n in '%_' else n)); j += 2; continue
                if ch == c:
                    if j + 1 < len(sql) and sql[j + 1] == c: buf.append(c); j += 2; continue
                    break
                buf.append(ch); j += 1
            out.append(('str', ''.join(buf))); i = j + 1
        elif c == '`':
            j = sql.index('`', i + 1)
            out.append(('qident', sql[i + 1:j])); i = j + 1
        elif c.isalnum() or c in '_$@{}':
            j = i
            while j < len(sql) and (sql[j].isalnum() or sql[j] in '_$@{}'): j += 1
            out.append(('word', sql[i:j])); i = j
        else:
            out.append(('punct', c)); i += 1
    return out

def statements(sql):
    cur, out = [], []
    for t in tokens(sql):
        if t == ('punct', ';'):
            out.append(cur); cur = []
        else:
            cur.append(t)
    if any(k not in ('ws', 'comment') for k, _ in cur): out.append(cur)
    return out

def name(t):
    return t[1].lower() if t[0] in ('qident', 'word') else t[1]

def q(ident):
    """A column/index identifier: quoted lowercase; table names keep the placeholder unquoted."""
    if ident.startswith(P): return ident.lower().replace(P.lower(), P)
    return '"' + ident.lower() + '"'

def lit(s): return "'" + s.replace("'", "''") + "'"

def render(toks):
    out = []
    for k, v in toks:
        if k == 'qident': out.append(q(v))
        elif k == 'str': out.append(lit(v))
        elif k == 'comment': out.append(v)
        else: out.append(v)
    return ''.join(out)

TEXT = ('varchar', 'char', 'text', 'tinytext', 'mediumtext', 'longtext', 'enum', 'set')
LONG = ('text', 'tinytext', 'mediumtext', 'longtext')

def pg_type(mysql_type):
    t = mysql_type.lower()
    base = re.match(r'[a-z]+', t).group(0)
    uns = 'unsigned' in t
    if base in TEXT: return 'citext'
    if base == 'bigint': return 'bigint'
    if base in ('int', 'integer', 'mediumint'): return 'bigint' if uns else 'integer'
    if base == 'smallint': return 'integer' if uns else 'smallint'
    if base == 'tinyint': return 'smallint'
    if base == 'double': return 'double precision'
    if base == 'float': return 'real'
    if base == 'decimal': return 'numeric' + (re.search(r'\(.*?\)', t).group(0) if '(' in t else '')
    if base == 'datetime' or base == 'timestamp':
        m = re.search(r'\((\d)\)', t)
        return 'timestamp(%s) without time zone' % (m.group(1) if m else '0')
    if base == 'date': return 'date'
    if base == 'json': return 'jsonb'
    if base in ('blob', 'longblob', 'mediumblob', 'varbinary', 'binary'): return 'bytea'
    raise SystemExit('unknown type ' + mysql_type)

def idx_name(table, key):
    base = table.replace(P, '') + '_' + key.lower()
    if len(base) > 50: base = base[:40] + '_' + hashlib.md5(base.encode()).hexdigest()[:8]
    return P + base

def split_top(toks):
    """Splits a token list on top-level commas."""
    parts, depth, cur = [], 0, []
    for t in toks:
        if t == ('punct', '('): depth += 1
        if t == ('punct', ')'): depth -= 1
        if t == ('punct', ',') and depth == 0: parts.append(cur); cur = []; continue
        cur.append(t)
    parts.append(cur)
    return parts

def strip(toks):
    toks = [t for t in toks if t[0] != 'comment']
    while toks and toks[0][0] == 'ws': toks = toks[1:]
    while toks and toks[-1][0] == 'ws': toks = toks[:-1]
    return toks

def words(toks): return [t for t in toks if t[0] not in ('ws', 'comment')]

def create_table(toks):
    toks = strip(toks)
    w = words(toks)
    # CREATE TABLE IF NOT EXISTS <name> ( ... ) options
    ti = 5 if w[2][1].upper() == 'IF' else 2
    table = name(w[ti])
    open_i = toks.index(('punct', '('))
    depth, close_i = 0, None
    for i in range(open_i, len(toks)):
        if toks[i] == ('punct', '('): depth += 1
        if toks[i] == ('punct', ')'):
            depth -= 1
            if depth == 0: close_i = i; break
    cols, keys, after, touched = [], [], [], []
    types = {}
    for part in split_top(toks[open_i + 1:close_i]):
        part = strip(part)
        pw = words(part)
        if not pw: continue
        head = pw[0][1].upper() if pw[0][0] == 'word' else ''
        if head in ('PRIMARY', 'UNIQUE', 'KEY', 'INDEX', 'FULLTEXT', 'SPATIAL', 'CONSTRAINT'):
            keys.append(part); continue
        col = name(pw[0])
        # type tokens up to the first keyword
        j, typ = 1, []
        while j < len(pw) and not (pw[j][0] == 'word' and pw[j][1].upper() in ('NOT', 'NULL', 'DEFAULT', 'AUTO_INCREMENT', 'ON', 'COMMENT', 'CHARACTER', 'COLLATE', 'GENERATED')):
            typ.append(pw[j][1]); j += 1
        mtype = ' '.join(typ).replace(' (', '(').replace('( ', '(').replace(' )', ')').replace(' , ', ',')
        types[col] = mtype.lower()
        rest = pw[j:]
        pg = pg_type(mtype)
        defs = []
        k = 0
        auto = False
        while k < len(rest):
            v = rest[k][1].upper() if rest[k][0] == 'word' else rest[k][1]
            if v == 'NOT' and rest[k + 1][1].upper() == 'NULL': defs.append('NOT NULL'); k += 2; continue
            if v == 'NULL': defs.append('NULL'); k += 1; continue
            if v == 'AUTO_INCREMENT': auto = True; k += 1; continue
            if v == 'DEFAULT':
                d = rest[k + 1]
                k += 2
                if d[0] == 'str':
                    val = d[1]
                    if val.startswith('0000-00-00'): val = '0001-01-01 00:00:00'
                    defs.append('DEFAULT ' + lit(val))
                elif d[1].upper().startswith('CURRENT_TIMESTAMP'):
                    if k < len(rest) and rest[k] == ('punct', '('): k += 3
                    defs.append("DEFAULT (now() AT TIME ZONE 'UTC')")
                elif d[1].upper() == 'NULL':
                    defs.append('DEFAULT NULL')
                elif d == ('punct', '-'):
                    defs.append('DEFAULT -' + rest[k][1]); k += 1
                else:
                    defs.append('DEFAULT ' + d[1])
                continue
            if v == 'ON' and rest[k + 1][1].upper() == 'UPDATE':
                touched.append(col); k += 3
                if k < len(rest) and rest[k] == ('punct', '('): k += 3
                continue
            if v in ('CHARACTER', 'COLLATE', 'COMMENT'):
                k += 3 if v == 'CHARACTER' else 2; continue
            raise SystemExit('unhandled %r in %s.%s' % (v, table, col))
        if auto: pg += ' GENERATED BY DEFAULT AS IDENTITY'
        cols.append('  %s %s%s' % (q(col), pg, (' ' + ' '.join(defs)) if defs else ''))
    body, out_after = list(cols), []
    for part in keys:
        pw = words(part)
        kind = pw[0][1].upper()
        if kind == 'CONSTRAINT':
            body.append('  ' + render(part).replace('ON DELETE CASCADE', 'ON DELETE CASCADE')); continue
        # column list
        o = part.index(('punct', '('))
        depth, c = 0, None
        for i in range(o, len(part)):
            if part[i] == ('punct', '('): depth += 1
            if part[i] == ('punct', ')'):
                depth -= 1
                if depth == 0: c = i; break
        items = []
        for it in split_top(part[o + 1:c]):
            iw = words(it)
            cn = name(iw[0])
            if len(iw) > 1 and iw[1] == ('punct', '('):
                n = iw[2][1]
                if types.get(cn, '').split('(')[0] in LONG:
                    items.append('left(%s, %s)' % (q(cn), n))
                    continue
            items.append(q(cn))
        if kind == 'PRIMARY':
            body.append('  PRIMARY KEY (%s)' % ', '.join(items)); continue
        if kind == 'FULLTEXT': continue
        kname = name(pw[2] if kind == 'UNIQUE' and pw[1][1].upper() in ('KEY', 'INDEX') else pw[1])
        uniq = 'UNIQUE ' if kind == 'UNIQUE' else ''
        out_after.append('CREATE %sINDEX IF NOT EXISTS %s ON %s (%s);' % (uniq, idx_name(table, kname), table, ', '.join(items)))
    sql = 'CREATE TABLE IF NOT EXISTS %s (\n%s\n);\n' % (table, ',\n'.join(body))
    sql += '\n'.join(out_after) + ('\n' if out_after else '')
    for col in touched:
        sql += ('DROP TRIGGER IF EXISTS %s ON %s;\nCREATE TRIGGER %s BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION cwm_touch_%s();\n'
                % (idx_name(table, 'touch_' + col), table, idx_name(table, 'touch_' + col), table, col))
    return sql, touched

def insert(toks):
    toks = strip(toks)
    ignore = False
    out = []
    i = 0
    while i < len(toks):
        k, v = toks[i]
        if k == 'word' and v.upper() == 'IGNORE' and not ignore and words(out) and words(out)[-1][1].upper() == 'INSERT':
            ignore = True
            # drop the following whitespace too
            i += 1
            if i < len(toks) and toks[i][0] == 'ws': i += 1
            continue
        if k == 'word' and v.upper() == 'FROM' and i + 2 < len(toks) and toks[i + 2][1].upper() == 'DUAL':
            i += 3
            continue
        out.append(toks[i]); i += 1
    sql = render(out)
    if ignore: sql += '\nON CONFLICT DO NOTHING'
    return sql + ';\n'

HEADER = """-- ============================================================================
-- GENERATED by ci/tools/mysql2pg.py from ../mysql/%s - do not edit by hand.
-- PostgreSQL equivalent of the MySQL migration (same tables, columns, keys).
-- ============================================================================

"""

V1_PRELUDE = """-- Text columns are citext: case-insensitive comparisons and LIKE, like
-- WordPress' utf8mb4_unicode_520_ci collation on MySQL.
CREATE EXTENSION IF NOT EXISTS citext;

-- cwm_int / cwm_num: MySQL's lenient text-to-number casts ('' -> 0,
-- '12abc' -> 12), used by the API's CAST(meta_value AS SIGNED/DECIMAL).
CREATE OR REPLACE FUNCTION cwm_int(v anyelement) RETURNS bigint LANGUAGE sql IMMUTABLE AS $$
  SELECT COALESCE(CAST(NULLIF(substring(CAST(v AS text) FROM '^\\s*([-+]?[0-9]+)'), '') AS bigint), 0)
$$;
CREATE OR REPLACE FUNCTION cwm_num(v anyelement) RETURNS numeric LANGUAGE sql IMMUTABLE AS $$
  SELECT COALESCE(CAST(NULLIF(substring(CAST(v AS text) FROM '^\\s*([-+]?([0-9]+\\.?[0-9]*|\\.[0-9]+)([eE][-+]?[0-9]+)?)'), '') AS numeric), 0)
$$;

-- ON UPDATE CURRENT_TIMESTAMP
CREATE OR REPLACE FUNCTION cwm_touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at := now() AT TIME ZONE 'UTC'; RETURN NEW; END
$$;

"""

def convert(fname):
    src = open(os.path.join(SRC, fname)).read()
    out = [HEADER % fname]
    if fname.startswith('V1__'): out.append(V1_PRELUDE)
    for st in statements(src):
        w = words(st)
        if not w: continue
        head = w[0][1].upper()
        lead = ''.join(v for k, v in st if k == 'comment' and v.startswith('--') and False)
        if head == 'SET': continue
        if head == 'CREATE' and w[1][1].upper() == 'TABLE':
            sql, touched = create_table(st)
            for c in touched:
                if c != 'updated_at': raise SystemExit('touch trigger only for updated_at')
            out.append(sql + '\n')
        elif head == 'INSERT':
            out.append(insert(st) + '\n')
        else:
            out.append(render(strip(st)) + ';\n\n')
    return ''.join(out)

if __name__ == '__main__':
    os.makedirs(DST, exist_ok=True)
    for f in sorted(os.listdir(SRC)):
        if re.match(r'V\d+__.*\.sql$', f):
            open(os.path.join(DST, f), 'w').write(convert(f))
            print('wrote postgresql/' + f)
