package woo

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/db"
	"goocommerce-api/internal/php"
)

// Customers are WordPress users other than administrators and shop
// managers, with WooCommerce's billing_* / shipping_* user meta. Their order
// count and total spent are computed from the orders (like
// wc_get_customer_order_count / wc_get_customer_total_spent), and
// wc_customer_lookup is kept in sync when one changes.

// ShopCustomer is a customer account.
type ShopCustomer struct {
	ID          uint64     `json:"id"`
	Email       string     `json:"email"`
	Username    string     `json:"username"`
	DisplayName string     `json:"displayName"`
	FirstName   string     `json:"firstName"`
	LastName    string     `json:"lastName"`
	Role        string     `json:"role"`
	OrdersCount int64      `json:"ordersCount"`
	TotalSpent  float64    `json:"totalSpent"`
	Billing     Address    `json:"billing"`
	Shipping    Address    `json:"shipping"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastOrderAt *time.Time `json:"lastOrderAt"`
}

// CustomerUpdate is an admin edit of a customer; nil fields are kept.
type CustomerUpdate struct {
	FirstName *string  `json:"firstName,omitempty"`
	LastName  *string  `json:"lastName,omitempty"`
	Email     *string  `json:"email,omitempty"`
	Billing   *Address `json:"billing,omitempty"`
	Shipping  *Address `json:"shipping,omitempty"`
}

// CustomerOrderSummary is one of a customer's orders.
type CustomerOrderSummary struct {
	ID        uint64    `json:"id"`
	Number    string    `json:"number"`
	Status    string    `json:"status"`
	Total     float64   `json:"total"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"createdAt"`
}

var customerAddressFields = []string{"first_name", "last_name", "company", "address_1", "address_2", "city", "state", "postcode", "country", "email", "phone"}

func customerMetaKeys() []string {
	keys := []string{"first_name", "last_name"}
	for _, kind := range []string{"billing_", "shipping_"} {
		for _, f := range customerAddressFields {
			keys = append(keys, kind+f)
		}
	}
	return keys
}

func custAddressFromMeta(m map[string]string, kind string) Address {
	a := Address{FirstName: m[kind+"first_name"], LastName: m[kind+"last_name"], Company: m[kind+"company"],
		Address1: m[kind+"address_1"], Address2: m[kind+"address_2"], City: m[kind+"city"], State: m[kind+"state"],
		Postcode: m[kind+"postcode"], Country: m[kind+"country"], Phone: m[kind+"phone"]}
	if kind == "billing_" {
		a.Email = m[kind+"email"]
	}
	return a
}

func custAddressToMeta(a Address, kind string) map[string]string {
	out := map[string]string{
		kind + "first_name": strings.TrimSpace(a.FirstName), kind + "last_name": strings.TrimSpace(a.LastName),
		kind + "company": strings.TrimSpace(a.Company), kind + "address_1": strings.TrimSpace(a.Address1),
		kind + "address_2": strings.TrimSpace(a.Address2), kind + "city": strings.TrimSpace(a.City),
		kind + "state": strings.TrimSpace(a.State), kind + "postcode": strings.TrimSpace(a.Postcode),
		kind + "country": strings.ToUpper(strings.TrimSpace(a.Country)), kind + "phone": strings.TrimSpace(a.Phone),
	}
	if kind == "billing_" {
		out[kind+"email"] = strings.TrimSpace(a.Email)
	}
	return out
}

func (s *Store) customerCapsKey() string { return s.DB.Raw("capabilities") }

// customerWhere excludes administrators and shop managers.
func (s *Store) customerWhere() (string, []any) {
	return `NOT EXISTS (SELECT 1 FROM ` + s.T("usermeta") + ` cm WHERE cm.user_id = u.ID AND cm.meta_key = ?
		AND (cm.meta_value LIKE '%"administrator"%' OR cm.meta_value LIKE '%"shop_manager"%'))`, []any{s.customerCapsKey()}
}

// ListCustomers lists customers, newest first, searching email, login,
// display name and first/last name.
func (s *Store) ListCustomers(ctx context.Context, search string, page, perPage int) (Paged[ShopCustomer], error) {
	page, perPage, offset := Page(page, perPage)
	where, args := s.customerWhere()
	if t := strings.TrimSpace(search); t != "" {
		like := "%" + t + "%"
		where += ` AND (u.user_email LIKE ? OR u.user_login LIKE ? OR u.display_name LIKE ? OR EXISTS (SELECT 1 FROM ` +
			s.T("usermeta") + ` nm WHERE nm.user_id = u.ID AND nm.meta_key IN ('first_name','last_name','billing_email','billing_phone') AND nm.meta_value LIKE ?))`
		args = append(args, like, like, like, like)
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("users")+` u WHERE `+where, args...).Scan(&total); err != nil {
		return Paged[ShopCustomer]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT u.ID, u.user_email, u.user_login, u.display_name, u.user_registered FROM `+s.T("users")+
		` u WHERE `+where+` ORDER BY u.user_registered DESC, u.ID DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return Paged[ShopCustomer]{}, err
	}
	items, err := s.scanCustomers(ctx, rows)
	if err != nil {
		return Paged[ShopCustomer]{}, err
	}
	return NewPaged(items, total, page, perPage), nil
}

// GetCustomer is one customer (ErrNotFound for an unknown user or a staff one).
func (s *Store) GetCustomer(ctx context.Context, id uint64) (ShopCustomer, error) {
	where, args := s.customerWhere()
	rows, err := s.DB.QueryContext(ctx, `SELECT u.ID, u.user_email, u.user_login, u.display_name, u.user_registered FROM `+s.T("users")+
		` u WHERE u.ID = ? AND `+where, append([]any{id}, args...)...)
	if err != nil {
		return ShopCustomer{}, err
	}
	items, err := s.scanCustomers(ctx, rows)
	if err != nil {
		return ShopCustomer{}, err
	}
	if len(items) == 0 {
		return ShopCustomer{}, ErrNotFound
	}
	return items[0], nil
}

func (s *Store) scanCustomers(ctx context.Context, rows *sql.Rows) ([]ShopCustomer, error) {
	out := []ShopCustomer{}
	for rows.Next() {
		var c ShopCustomer
		var registered sql.NullTime
		if err := rows.Scan(&c.ID, &c.Email, &c.Username, &c.DisplayName, &registered); err != nil {
			rows.Close()
			return nil, err
		}
		c.CreatedAt = registered.Time.UTC()
		out = append(out, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]uint64, len(out))
	for i, c := range out {
		ids[i] = c.ID
	}
	keys := append(customerMetaKeys(), s.customerCapsKey())
	mrows, err := s.DB.QueryContext(ctx, `SELECT user_id, meta_key, meta_value FROM `+s.T("usermeta")+` WHERE user_id IN (`+inList(ids)+
		`) AND meta_key IN (`+placeholders(len(keys))+`)`, strArgs(keys)...)
	if err != nil {
		return nil, err
	}
	meta := map[uint64]map[string]string{}
	for mrows.Next() {
		var id uint64
		var k string
		var v sql.NullString
		if err := mrows.Scan(&id, &k, &v); err != nil {
			mrows.Close()
			return nil, err
		}
		if meta[id] == nil {
			meta[id] = map[string]string{}
		}
		if _, seen := meta[id][k]; !seen {
			meta[id][k] = v.String
		}
	}
	mrows.Close()
	if err := mrows.Err(); err != nil {
		return nil, err
	}
	stats, err := s.customerStats(ctx, ids)
	if err != nil {
		return nil, err
	}
	dec := s.Decimals(ctx)
	for i := range out {
		c := &out[i]
		m := meta[c.ID]
		c.FirstName, c.LastName = m["first_name"], m["last_name"]
		c.Billing, c.Shipping = custAddressFromMeta(m, "billing_"), custAddressFromMeta(m, "shipping_")
		c.Role = "customer"
		caps := php.UnmarshalArray(m[s.customerCapsKey()])
		for _, k := range caps.Keys {
			if v := caps.Get(k); v == true || php.ToString(v) == "1" {
				c.Role = php.ToString(k)
				break
			}
		}
		st := stats[c.ID]
		c.OrdersCount, c.TotalSpent = st.orders, round(st.spent, dec)
		if !st.last.IsZero() {
			last := st.last
			c.LastOrderAt = &last
		}
	}
	return out, nil
}

type customerStat struct {
	orders int64
	spent  float64
	last   time.Time
}

// customerOrderExcluded are not counted in a customer's order count.
var customerOrderExcluded = []any{"trash", "auto-draft", "draft", "checkout-draft", "wc-checkout-draft"}

// customerStats is the order count (every order but drafts and trash), the
// total spent (paid orders, refunds deducted) and the last order of users.
func (s *Store) customerStats(ctx context.Context, ids []uint64) (map[uint64]customerStat, error) {
	out := map[uint64]customerStat{}
	if len(ids) == 0 {
		return out, nil
	}
	hpos, _ := s.HPOS(ctx)
	paid := []any{"wc-processing", "wc-completed"}
	var q, rq string
	if hpos {
		o := s.T("wc_orders")
		q = `SELECT customer_id, COUNT(*), COALESCE(SUM(CASE WHEN status IN (?,?) THEN total_amount ELSE 0 END),0), MAX(date_created_gmt)
			FROM ` + o + ` WHERE type = 'shop_order' AND customer_id IN (` + inList(ids) + `) AND status NOT IN (` + placeholders(len(customerOrderExcluded)) + `)
			GROUP BY customer_id`
		rq = `SELECT par.customer_id, COALESCE(SUM(r.total_amount),0) FROM ` + o + ` r JOIN ` + o + ` par ON par.id = r.parent_order_id
			WHERE r.type = 'shop_order_refund' AND par.customer_id IN (` + inList(ids) + `) AND par.status IN (?,?) GROUP BY par.customer_id`
	} else {
		p, pm := s.T("posts"), s.T("postmeta")
		q = `SELECT CAST(cu.meta_value AS UNSIGNED), COUNT(*), COALESCE(SUM(CASE WHEN p.post_status IN (?,?) THEN CAST(tot.meta_value AS DECIMAL(26,8)) ELSE 0 END),0), MAX(p.post_date_gmt)
			FROM ` + p + ` p JOIN ` + pm + ` cu ON cu.post_id = p.ID AND cu.meta_key = '_customer_user'
			LEFT JOIN ` + pm + ` tot ON tot.post_id = p.ID AND tot.meta_key = '_order_total'
			WHERE p.post_type = 'shop_order' AND cu.meta_value IN (` + repQuotedIDs(ids) + `) AND p.post_status NOT IN (` + placeholders(len(customerOrderExcluded)) + `)
			GROUP BY cu.meta_value`
		rq = `SELECT CAST(cu.meta_value AS UNSIGNED), COALESCE(SUM(CAST(tot.meta_value AS DECIMAL(26,8))),0)
			FROM ` + p + ` r JOIN ` + p + ` par ON par.ID = r.post_parent
			JOIN ` + pm + ` cu ON cu.post_id = par.ID AND cu.meta_key = '_customer_user'
			LEFT JOIN ` + pm + ` tot ON tot.post_id = r.ID AND tot.meta_key = '_order_total'
			WHERE r.post_type = 'shop_order_refund' AND cu.meta_value IN (` + repQuotedIDs(ids) + `) AND par.post_status IN (?,?)
			GROUP BY cu.meta_value`
	}
	rows, err := s.DB.QueryContext(ctx, q, append(paid, customerOrderExcluded...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id uint64
		var st customerStat
		var last sql.NullTime
		if err := rows.Scan(&id, &st.orders, &st.spent, &last); err != nil {
			rows.Close()
			return nil, err
		}
		st.last = last.Time.UTC()
		out[id] = st
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rrows, err := s.DB.QueryContext(ctx, rq, paid...)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var id uint64
		var refunded float64
		if err := rrows.Scan(&id, &refunded); err != nil {
			return nil, err
		}
		st := out[id]
		st.spent += refunded // refund totals are negative
		out[id] = st
	}
	return out, rrows.Err()
}

// UpdateCustomer edits a customer's names, account email (unique among
// users) and addresses, then refreshes wc_customer_lookup.
func (s *Store) UpdateCustomer(ctx context.Context, id uint64, u CustomerUpdate) (ShopCustomer, error) {
	if _, err := s.GetCustomer(ctx, id); err != nil {
		return ShopCustomer{}, err
	}
	values := map[string]string{}
	if u.FirstName != nil {
		values["first_name"] = strings.TrimSpace(*u.FirstName)
	}
	if u.LastName != nil {
		values["last_name"] = strings.TrimSpace(*u.LastName)
	}
	if u.Billing != nil {
		if e := strings.TrimSpace(u.Billing.Email); e != "" {
			if _, err := mail.ParseAddress(e); err != nil {
				return ShopCustomer{}, refuse(http.StatusBadRequest, "errors.invalidEmail", "Invalid billing email")
			}
		}
		for k, v := range custAddressToMeta(*u.Billing, "billing_") {
			values[k] = v
		}
	}
	if u.Shipping != nil {
		for k, v := range custAddressToMeta(*u.Shipping, "shipping_") {
			values[k] = v
		}
	}
	email := ""
	if u.Email != nil {
		email = strings.TrimSpace(*u.Email)
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			return ShopCustomer{}, refuse(http.StatusBadRequest, "errors.invalidEmail", "Invalid email")
		}
	}
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if email != "" {
			var other uint64
			err := tx.QueryRowContext(ctx, `SELECT ID FROM `+s.T("users")+` WHERE LOWER(user_email) = LOWER(?) AND ID <> ? LIMIT 1`, email, id).Scan(&other)
			if err == nil {
				return refuse(http.StatusBadRequest, "errors.duplicateEmail", "Email already in use")
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("users")+` SET user_email = ? WHERE ID = ?`, email, id); err != nil {
				return err
			}
		}
		for k, v := range values {
			if err := s.custSetUserMeta(ctx, tx, id, k, v); err != nil {
				return err
			}
		}
		return s.upsertCustomerLookup(ctx, tx, id)
	})
	if err != nil {
		return ShopCustomer{}, err
	}
	return s.GetCustomer(ctx, id)
}

// custSetUserMeta is update_user_meta: one row per key.
func (s *Store) custSetUserMeta(ctx context.Context, q db.Querier, userID uint64, key, value string) error {
	var metaID uint64
	err := q.QueryRowContext(ctx, `SELECT umeta_id FROM `+s.T("usermeta")+` WHERE user_id = ? AND meta_key = ? ORDER BY umeta_id LIMIT 1`,
		userID, key).Scan(&metaID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = q.ExecContext(ctx, `INSERT INTO `+s.T("usermeta")+` (user_id, meta_key, meta_value) VALUES (?, ?, ?)`, userID, key, value)
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE `+s.T("usermeta")+` SET meta_value = ? WHERE umeta_id = ?`, value, metaID)
	return err
}

// UpsertCustomerLookup refreshes a registered customer's wc_customer_lookup
// row (WooCommerce Analytics' customers), from the user and its billing meta.
func (s *Store) UpsertCustomerLookup(ctx context.Context, userID uint64) error {
	return s.upsertCustomerLookup(ctx, s.DB, userID)
}

func (s *Store) upsertCustomerLookup(ctx context.Context, q db.Querier, userID uint64) error {
	var login, email string
	var registered sql.NullTime
	err := q.QueryRowContext(ctx, `SELECT user_login, user_email, user_registered FROM `+s.T("users")+` WHERE ID = ?`, userID).
		Scan(&login, &email, &registered)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	keys := []string{"first_name", "last_name", "billing_first_name", "billing_last_name", "billing_country",
		"billing_postcode", "billing_city", "billing_state", "wc_last_active"}
	m := map[string]string{}
	rows, err := q.QueryContext(ctx, `SELECT meta_key, meta_value FROM `+s.T("usermeta")+` WHERE user_id = ? AND meta_key IN (`+
		placeholders(len(keys))+`)`, append([]any{userID}, strArgs(keys)...)...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k string
		var v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return err
		}
		m[k] = v.String
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	first, last := m["first_name"], m["last_name"]
	if first == "" {
		first = m["billing_first_name"]
	}
	if last == "" {
		last = m["billing_last_name"]
	}
	var lastActive any
	if n, err := strconv.ParseInt(strings.TrimSpace(m["wc_last_active"]), 10, 64); err == nil && n > 0 {
		lastActive = time.Unix(n, 0).UTC()
	}
	var reg any
	if registered.Valid {
		reg = registered.Time.UTC()
	}
	lookup := s.T("wc_customer_lookup")
	var customerID uint64
	err = q.QueryRowContext(ctx, `SELECT customer_id FROM `+lookup+` WHERE user_id = ? LIMIT 1`, userID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = q.ExecContext(ctx, `INSERT INTO `+lookup+` (user_id, username, first_name, last_name, email, date_last_active,
			date_registered, country, postcode, city, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, login, first, last, email, lastActive, reg, m["billing_country"], m["billing_postcode"], m["billing_city"], m["billing_state"])
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE `+lookup+` SET username = ?, first_name = ?, last_name = ?, email = ?,
		date_last_active = COALESCE(?, date_last_active), date_registered = COALESCE(?, date_registered),
		country = ?, postcode = ?, city = ?, state = ? WHERE customer_id = ?`,
		login, first, last, email, lastActive, reg, m["billing_country"], m["billing_postcode"], m["billing_city"], m["billing_state"], customerID)
	return err
}

// CustomerOrders is a customer's orders, newest first (every status but
// drafts and trash).
func (s *Store) CustomerOrders(ctx context.Context, userID uint64, page, perPage int) (Paged[CustomerOrderSummary], error) {
	page, perPage, offset := Page(page, perPage)
	hpos, _ := s.HPOS(ctx)
	var count, list string
	args := append([]any{}, customerOrderExcluded...)
	if hpos {
		from := ` FROM ` + s.T("wc_orders") + ` o WHERE o.type = 'shop_order' AND o.customer_id = ? AND o.status NOT IN (` +
			placeholders(len(customerOrderExcluded)) + `)`
		count = `SELECT COUNT(*)` + from
		list = `SELECT o.id, COALESCE(o.status,''), COALESCE(o.total_amount,0), COALESCE(o.currency,''), o.date_created_gmt` + from +
			` ORDER BY o.date_created_gmt DESC, o.id DESC LIMIT ? OFFSET ?`
	} else {
		pm := s.T("postmeta")
		from := ` FROM ` + s.T("posts") + ` p JOIN ` + pm + ` cu ON cu.post_id = p.ID AND cu.meta_key = '_customer_user'
			WHERE p.post_type = 'shop_order' AND cu.meta_value = ? AND p.post_status NOT IN (` + placeholders(len(customerOrderExcluded)) + `)`
		count = `SELECT COUNT(*)` + from
		list = `SELECT p.ID, p.post_status,
			COALESCE((SELECT meta_value FROM ` + pm + ` WHERE post_id = p.ID AND meta_key = '_order_total' LIMIT 1),'0'),
			COALESCE((SELECT meta_value FROM ` + pm + ` WHERE post_id = p.ID AND meta_key = '_order_currency' LIMIT 1),''),
			p.post_date_gmt` + from + ` ORDER BY p.post_date_gmt DESC, p.ID DESC LIMIT ? OFFSET ?`
	}
	var uid any = userID
	if !hpos {
		uid = strconv.FormatUint(userID, 10)
	}
	full := append([]any{uid}, args...)
	var total int64
	if err := s.DB.QueryRowContext(ctx, count, full...).Scan(&total); err != nil {
		return Paged[CustomerOrderSummary]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, list, append(full, perPage, offset)...)
	if err != nil {
		return Paged[CustomerOrderSummary]{}, err
	}
	defer rows.Close()
	items := []CustomerOrderSummary{}
	for rows.Next() {
		var o CustomerOrderSummary
		var total string
		var created sql.NullTime
		if err := rows.Scan(&o.ID, &o.Status, &total, &o.Currency, &created); err != nil {
			return Paged[CustomerOrderSummary]{}, err
		}
		o.Status = repStatus(o.Status)
		o.Total = parseFloat(total)
		o.Number = strconv.FormatUint(o.ID, 10)
		o.CreatedAt = created.Time.UTC()
		items = append(items, o)
	}
	if err := rows.Err(); err != nil {
		return Paged[CustomerOrderSummary]{}, err
	}
	return NewPaged(items, total, page, perPage), nil
}
