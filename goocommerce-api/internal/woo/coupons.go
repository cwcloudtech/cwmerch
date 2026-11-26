package woo

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/php"
)

// Coupon is a shop_coupon.
type Coupon struct {
	ID                        uint64     `json:"id"`
	Code                      string     `json:"code"`
	Amount                    float64    `json:"amount"`
	DiscountType              string     `json:"discountType"`
	Description               string     `json:"description"`
	DateExpires               *time.Time `json:"dateExpires"`
	UsageCount                int64      `json:"usageCount"`
	IndividualUse             bool       `json:"individualUse"`
	ProductIDs                []uint64   `json:"productIds"`
	ExcludedProductIDs        []uint64   `json:"excludedProductIds"`
	UsageLimit                *int64     `json:"usageLimit"`
	UsageLimitPerUser         *int64     `json:"usageLimitPerUser"`
	LimitUsageToXItems        *int64     `json:"limitUsageToXItems"`
	FreeShipping              bool       `json:"freeShipping"`
	ProductCategories         []uint64   `json:"productCategories"`
	ExcludedProductCategories []uint64   `json:"excludedProductCategories"`
	ExcludeSaleItems          bool       `json:"excludeSaleItems"`
	MinimumAmount             *float64   `json:"minimumAmount"`
	MaximumAmount             *float64   `json:"maximumAmount"`
	EmailRestrictions         []string   `json:"emailRestrictions"`
	UsedBy                    []string   `json:"usedBy"`
	Status                    string     `json:"status"`
	CreatedAt                 time.Time  `json:"createdAt"`
	UpdatedAt                 time.Time  `json:"updatedAt"`
}

func csvIDs(v string) []uint64 {
	out := []uint64{}
	for _, p := range strings.Split(v, ",") {
		if id := ParseID(p); id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func joinIDs(ids []uint64) string {
	parts := []string{}
	for _, id := range ids {
		parts = append(parts, strconv.FormatUint(id, 10))
	}
	return strings.Join(parts, ",")
}

func positivePtr(p *int64) *int64 {
	if p == nil || *p <= 0 {
		return nil
	}
	return p
}

func (s *Store) hydrateCoupons(ctx context.Context, rows []postRow) ([]Coupon, error) {
	out := []Coupon{}
	ids := []uint64{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	metas, err := s.postMetas(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}
	usedBy := map[uint64][]string{}
	if len(ids) > 0 {
		urows, err := s.DB.QueryContext(ctx, `SELECT post_id, meta_value FROM `+s.T("postmeta")+` WHERE meta_key = '_used_by' AND post_id IN (`+inList(ids)+`)`)
		if err != nil {
			return nil, err
		}
		for urows.Next() {
			var id uint64
			var v sql.NullString
			if err := urows.Scan(&id, &v); err != nil {
				urows.Close()
				return nil, err
			}
			usedBy[id] = append(usedBy[id], v.String)
		}
		urows.Close()
	}
	for _, r := range rows {
		m := metas[r.ID]
		c := Coupon{
			ID: r.ID, Code: r.Title, Amount: m.Float("coupon_amount"), DiscountType: orDefault(m["discount_type"], "fixed_cart"),
			Description: r.Excerpt, DateExpires: unixPtr(m["date_expires"]), UsageCount: m.Int("usage_count"),
			IndividualUse: m.Bool("individual_use"), ProductIDs: csvIDs(m["product_ids"]), ExcludedProductIDs: csvIDs(m["exclude_product_ids"]),
			UsageLimit: positivePtr(m.IntPtr("usage_limit")), UsageLimitPerUser: positivePtr(m.IntPtr("usage_limit_per_user")),
			LimitUsageToXItems: positivePtr(m.IntPtr("limit_usage_to_x_items")), FreeShipping: m.Bool("free_shipping"),
			ProductCategories: idsFromSerialized(m["product_categories"]), ExcludedProductCategories: idsFromSerialized(m["exclude_product_categories"]),
			ExcludeSaleItems: m.Bool("exclude_sale_items"), MinimumAmount: m.FloatPtr("minimum_amount"), MaximumAmount: m.FloatPtr("maximum_amount"),
			EmailRestrictions: php.UnmarshalArray(m["customer_email"]).StringList(), UsedBy: usedBy[r.ID],
			Status: r.Status, CreatedAt: r.Created, UpdatedAt: r.Modified,
		}
		if c.UsedBy == nil {
			c.UsedBy = []string{}
		}
		if c.MinimumAmount != nil && *c.MinimumAmount == 0 {
			c.MinimumAmount = nil
		}
		if c.MaximumAmount != nil && *c.MaximumAmount == 0 {
			c.MaximumAmount = nil
		}
		out = append(out, c)
	}
	return out, nil
}

// Coupons lists coupons (admin).
func (s *Store) Coupons(ctx context.Context, search string, page, perPage int) (Paged[Coupon], error) {
	page, perPage, offset := Page(page, perPage)
	where := "p.post_type = 'shop_coupon' AND p.post_status IN ('publish', 'draft', 'pending', 'private', 'future')"
	args := []any{}
	if search != "" {
		where += " AND p.post_title LIKE ?"
		args = append(args, "%"+search+"%")
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("posts")+` p WHERE `+where, args...).Scan(&total); err != nil {
		return Paged[Coupon]{}, err
	}
	rows, err := s.posts(ctx, s.DB, where+` ORDER BY p.post_date_gmt DESC, p.ID DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return Paged[Coupon]{}, err
	}
	items, err := s.hydrateCoupons(ctx, rows)
	if err != nil {
		return Paged[Coupon]{}, err
	}
	return NewPaged(items, total, page, perPage), nil
}

func (s *Store) Coupon(ctx context.Context, id uint64) (Coupon, error) {
	rows, err := s.posts(ctx, s.DB, `p.post_type = 'shop_coupon' AND p.ID = ?`, id)
	if err != nil {
		return Coupon{}, err
	}
	cs, err := s.hydrateCoupons(ctx, rows)
	if err != nil {
		return Coupon{}, err
	}
	if len(cs) == 0 {
		return Coupon{}, ErrNotFound
	}
	return cs[0], nil
}

// CouponByCode finds a published coupon (codes are case-insensitive, like WooCommerce).
func (s *Store) CouponByCode(ctx context.Context, code string) (Coupon, error) {
	rows, err := s.posts(ctx, s.DB, `p.post_type = 'shop_coupon' AND p.post_status = 'publish' AND LOWER(p.post_title) = ? ORDER BY p.ID DESC LIMIT 1`,
		strings.ToLower(strings.TrimSpace(code)))
	if err != nil {
		return Coupon{}, err
	}
	cs, err := s.hydrateCoupons(ctx, rows)
	if err != nil {
		return Coupon{}, err
	}
	if len(cs) == 0 {
		return Coupon{}, ErrNotFound
	}
	return cs[0], nil
}

// SaveCoupon creates (id 0) or replaces a coupon.
func (s *Store) SaveCoupon(ctx context.Context, id uint64, c Coupon, authorID uint64) (Coupon, error) {
	code := strings.ToLower(strings.TrimSpace(c.Code))
	if code == "" {
		return Coupon{}, refuse(400, "errors.couponCodeRequired", "A coupon code is required")
	}
	switch c.DiscountType {
	case "percent", "fixed_cart", "fixed_product":
	case "":
		c.DiscountType = "fixed_cart"
	default:
		return Coupon{}, refuse(400, "errors.invalidDiscountType", "Discount type must be percent, fixed_cart or fixed_product")
	}
	var dup int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("posts")+` WHERE post_type = 'shop_coupon' AND post_status <> 'trash' AND LOWER(post_title) = ? AND ID <> ?`,
		code, id).Scan(&dup); err != nil {
		return Coupon{}, err
	}
	if dup > 0 {
		return Coupon{}, refuse(400, "errors.couponCodeTaken", "A coupon with this code already exists")
	}
	status := orDefault(c.Status, "publish")
	err := s.DB.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if id == 0 {
			if id, err = s.insertPost(ctx, tx, "shop_coupon", status, code, Slugify(code), "", c.Description, 0, authorID, 0, "closed"); err != nil {
				return err
			}
		} else {
			t := now()
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_title = ?, post_name = ?, post_excerpt = ?, post_status = ?, post_modified = ?, post_modified_gmt = ?
				WHERE ID = ? AND post_type = 'shop_coupon'`, code, Slugify(code), c.Description, status, s.localTime(ctx, t), t, id); err != nil {
				return err
			}
		}
		i64 := func(p *int64) string {
			if p == nil || *p <= 0 {
				return ""
			}
			return strconv.FormatInt(*p, 10)
		}
		f64 := func(p *float64) string {
			if p == nil {
				return ""
			}
			return fmtFloat(*p)
		}
		expires := ""
		if c.DateExpires != nil {
			expires = strconv.FormatInt(c.DateExpires.Unix(), 10)
		}
		emails := []string{}
		for _, e := range c.EmailRestrictions {
			if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
				emails = append(emails, e)
			}
		}
		return s.setPostMetas(ctx, tx, id, map[string]string{
			"discount_type": c.DiscountType, "coupon_amount": fmtFloat(c.Amount), "individual_use": yesNo(c.IndividualUse),
			"product_ids": joinIDs(c.ProductIDs), "exclude_product_ids": joinIDs(c.ExcludedProductIDs),
			"usage_limit": i64(c.UsageLimit), "usage_limit_per_user": i64(c.UsageLimitPerUser), "limit_usage_to_x_items": i64(c.LimitUsageToXItems),
			"date_expires": expires, "free_shipping": yesNo(c.FreeShipping), "product_categories": serializeIDs(c.ProductCategories),
			"exclude_product_categories": serializeIDs(c.ExcludedProductCategories), "exclude_sale_items": yesNo(c.ExcludeSaleItems),
			"minimum_amount": f64(c.MinimumAmount), "maximum_amount": f64(c.MaximumAmount), "customer_email": serializeStrings(emails),
		})
	})
	if err != nil {
		return Coupon{}, err
	}
	if c.UsageCount == 0 {
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = 'usage_count'`, id).Scan(&n)
		if n == 0 {
			_ = s.setPostMeta(ctx, s.DB, id, "usage_count", "0")
		}
	}
	return s.Coupon(ctx, id)
}

// DeleteCoupon trashes (or force deletes) a coupon.
func (s *Store) DeleteCoupon(ctx context.Context, id uint64, force bool) error {
	if _, err := s.Coupon(ctx, id); err != nil {
		return err
	}
	if force {
		return s.DB.InTx(ctx, func(tx *sql.Tx) error { return s.deletePostHard(ctx, tx, id) })
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET post_status = 'trash' WHERE ID = ?`, id)
	return err
}

// couponUsesBy counts a customer's uses (by user id or email) from _used_by.
func couponUsesBy(c Coupon, userID uint64, email string) int64 {
	n := int64(0)
	uid := strconv.FormatUint(userID, 10)
	for _, u := range c.UsedBy {
		if (userID != 0 && u == uid) || (email != "" && strings.EqualFold(u, email)) {
			n++
		}
	}
	return n
}

func emailAllowed(patterns []string, email string) bool {
	if len(patterns) == 0 {
		return true
	}
	email = strings.ToLower(email)
	for _, p := range patterns {
		p = strings.ToLower(p)
		if strings.Contains(p, "*") {
			parts := strings.SplitN(p, "*", 2)
			if strings.HasPrefix(email, parts[0]) && strings.HasSuffix(email, parts[1]) {
				return true
			}
		} else if p == email {
			return true
		}
	}
	return false
}

// RecordCouponUse increments usage_count and appends _used_by (on order placement).
func (s *Store) RecordCouponUse(ctx context.Context, code string, userID uint64, email string) {
	c, err := s.CouponByCode(ctx, code)
	if err != nil {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE `+s.T("postmeta")+` SET meta_value = CAST(COALESCE(NULLIF(meta_value, ''), '0') AS SIGNED) + 1 WHERE post_id = ? AND meta_key = 'usage_count'`, c.ID)
	who := email
	if userID != 0 {
		who = strconv.FormatUint(userID, 10)
	}
	if who != "" {
		_, _ = s.DB.ExecContext(ctx, `INSERT INTO `+s.T("postmeta")+` (post_id, meta_key, meta_value) VALUES (?, '_used_by', ?)`, c.ID, who)
	}
}

// ReleaseCouponUse undoes RecordCouponUse (cancelled order).
func (s *Store) ReleaseCouponUse(ctx context.Context, code string, userID uint64, email string) {
	c, err := s.CouponByCode(ctx, code)
	if err != nil {
		return
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE `+s.T("postmeta")+` SET meta_value = GREATEST(CAST(COALESCE(NULLIF(meta_value, ''), '0') AS SIGNED) - 1, 0) WHERE post_id = ? AND meta_key = 'usage_count'`, c.ID)
	who := email
	if userID != 0 {
		who = strconv.FormatUint(userID, 10)
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM `+s.T("postmeta")+` WHERE post_id = ? AND meta_key = '_used_by' AND meta_value = ? LIMIT 1`, c.ID, who)
}
