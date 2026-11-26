package woo

import "context"

func (s *Store) groupCount(ctx context.Context, query string, args ...any) (map[string]int64, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var n int64
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// CountProductsByStatus feeds the goocommerce_products_total gauge.
func (s *Store) CountProductsByStatus(ctx context.Context) (map[string]int64, error) {
	return s.groupCount(ctx, `SELECT post_status, COUNT(*) FROM `+s.T("posts")+` WHERE post_type = 'product' GROUP BY post_status`)
}

// CountOrdersByStatus feeds the goocommerce_orders_total gauge.
func (s *Store) CountOrdersByStatus(ctx context.Context) (map[string]int64, error) {
	hpos, _ := s.HPOS(ctx)
	if hpos {
		return s.groupCount(ctx, `SELECT REPLACE(status, 'wc-', ''), COUNT(*) FROM `+s.T("wc_orders")+` WHERE type = 'shop_order' GROUP BY status`)
	}
	return s.groupCount(ctx, `SELECT REPLACE(post_status, 'wc-', ''), COUNT(*) FROM `+s.T("posts")+` WHERE post_type = 'shop_order' GROUP BY post_status`)
}

// CountUsersByRole feeds the goocommerce_users_total gauge (WordPress roles).
func (s *Store) CountUsersByRole(ctx context.Context) (map[string]int64, error) {
	raw, err := s.groupCount(ctx, `SELECT meta_value, COUNT(*) FROM `+s.T("usermeta")+` WHERE meta_key = ? GROUP BY meta_value`, s.DB.Raw("capabilities"))
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for caps, n := range raw {
		role := "other"
		for _, r := range []string{"administrator", "shop_manager", "customer", "subscriber", "editor", "author", "contributor"} {
			if contains(caps, `"`+r+`"`) {
				role = r
				break
			}
		}
		out[role] += n
	}
	return out, nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
