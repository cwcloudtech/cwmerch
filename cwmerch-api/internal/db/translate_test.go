package db

import "testing"

type fakeCat struct{}

func (fakeCat) UniqueKeys(table string) ([]string, [][]string) {
	switch table {
	case "wp_options":
		return []string{"option_id"}, [][]string{{"option_name"}}
	case "wp_wc_order_stats":
		return []string{"order_id"}, nil
	case "wp_cwmerch_sessions":
		return []string{"id"}, [][]string{{"ip", "user_id"}}
	}
	return nil, nil
}
func (fakeCat) Identity(table string) string {
	if table == "wp_options" || table == "wp_posts" {
		return map[string]string{"wp_options": "option_id", "wp_posts": "id"}[table]
	}
	return ""
}

func TestTranslate(t *testing.T) {
	cases := []struct{ in, out string }{
		{"SELECT ID FROM `wp_posts` WHERE post_name = ? AND x = 'it\\'s ?'", `SELECT ID FROM "wp_posts" WHERE post_name = $1 AND x = 'it''s ?'`},
		{"SELECT CAST(COALESCE(NULLIF(meta_value, ''), '0') AS SIGNED) + 1, CAST(o.id AS CHAR), CAST(x AS DECIMAL(20,4))", "SELECT cwm_int(COALESCE(NULLIF(meta_value, ''), '0')) + 1, CAST(o.id AS TEXT), cwm_num(x)"},
		{"SELECT IF(a = 1, IF(b, 2, 3), ?) FROM t", "SELECT (CASE WHEN a = 1 THEN (CASE WHEN b THEN 2 ELSE 3 END) ELSE $1 END) FROM t"},
		{"SELECT UTC_TIMESTAMP()", "SELECT (now() AT TIME ZONE 'UTC')"},
		{"SELECT JSON_UNQUOTE(JSON_EXTRACT(c.data, '$.model.buyer')) LIKE ?", "SELECT (CAST(c.data AS jsonb) #>> '{model,buyer}') LIKE $1"},
		{"ORDER BY FIELD(p.ID, 3, 1)", "ORDER BY (CASE p.ID WHEN  3 THEN 1 WHEN  1 THEN 2 ELSE 0 END)"},
		{"INSERT INTO `wp_options` (option_name, option_value, autoload) VALUES (?, ?, 'on') ON DUPLICATE KEY UPDATE option_value = VALUES(option_value)",
			`INSERT INTO "wp_options" (option_name, option_value, autoload) VALUES ($1, $2, 'on') ON CONFLICT (option_name) DO UPDATE SET option_value = EXCLUDED.option_value RETURNING option_id`},
		{"REPLACE INTO `wp_wc_order_stats` (order_id, status) VALUES (?, ?)", `INSERT INTO "wp_wc_order_stats" (order_id, status) VALUES ($1, $2) ON CONFLICT (order_id) DO UPDATE SET status = EXCLUDED.status`},
		{"INSERT IGNORE INTO `wp_term_relationships` (object_id) VALUES (?)", `INSERT INTO "wp_term_relationships" (object_id) VALUES ($1) ON CONFLICT DO NOTHING`},
		{"INSERT INTO wp_cwmerch_sessions (id, ip, user_id, data) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE data = VALUES(data)", "INSERT INTO wp_cwmerch_sessions (id, ip, user_id, data) VALUES ($1, $2, $3, $4) ON CONFLICT (ip, user_id) DO UPDATE SET data = EXCLUDED.data"},
		{"SELECT NULLIF(a, '') FROM x", "SELECT NULLIF(a, '') FROM x"},
	}
	for _, c := range cases {
		if got := Translate(c.in, fakeCat{}).SQL; got != c.out {
			t.Errorf("\n in: %s\ngot: %s\nwant %s", c.in, got, c.out)
		}
	}
}
