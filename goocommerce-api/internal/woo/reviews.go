package woo

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"goocommerce-api/internal/db"
	"goocommerce-api/internal/php"
)

// Product reviews are WordPress comments on the product with
// comment_type 'review' (replies are plain 'comment's with a parent), the
// rating and "verified owner" flag in commentmeta. After every change the
// product's rating caches are recomputed the way WooCommerce does
// (WC_Comments::clear_transients): _wc_average_rating, _wc_rating_count,
// _wc_review_count, posts.comment_count, the rated-N product_visibility term
// and wc_product_meta_lookup.

// ProductReview is a review (or a reply to one).
type ProductReview struct {
	ID            uint64    `json:"id"`
	ParentID      uint64    `json:"parentId"`
	ProductID     uint64    `json:"productId"`
	ProductName   string    `json:"productName"`
	Reviewer      string    `json:"reviewer"`
	ReviewerEmail string    `json:"reviewerEmail"`
	Review        string    `json:"review"`
	Rating        int       `json:"rating"`
	Verified      bool      `json:"verified"`
	Status        string    `json:"status"` // approved | hold | spam | trash
	Type          string    `json:"type"`   // review | reply
	UserID        uint64    `json:"userId"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ReviewFilter filters the admin list.
type ReviewFilter struct {
	Status    string // approved | hold | spam | trash | "" (all but trash and spam)
	ProductID uint64
	Search    string
	Page      int
	PerPage   int
}

// ReviewInput is a review written on the shop.
type ReviewInput struct {
	Rating int    `json:"rating"`
	Review string `json:"review"`
	Author string `json:"author,omitempty"`
	Email  string `json:"email,omitempty"`
}

// ReviewUpdate is an admin edit; nil fields are kept.
type ReviewUpdate struct {
	Status *string `json:"status,omitempty"`
	Review *string `json:"review,omitempty"`
	Rating *int    `json:"rating,omitempty"`
}

var reviewStatusToApproved = map[string]string{"approved": "1", "hold": "0", "spam": "spam", "trash": "trash"}

func reviewStatus(approved string) string {
	switch approved {
	case "1":
		return "approved"
	case "spam":
		return "spam"
	case "trash", "post-trashed":
		return "trash"
	}
	return "hold"
}

const reviewColumns = `c.comment_ID, c.comment_parent, c.comment_post_ID, COALESCE(p.post_title,''), c.comment_author,
	c.comment_author_email, c.comment_content, c.comment_approved, c.comment_type, c.user_id, c.comment_date_gmt`

func (s *Store) reviewFrom() string {
	return ` FROM ` + s.T("comments") + ` c JOIN ` + s.T("posts") + ` p ON p.ID = c.comment_post_ID AND p.post_type = 'product'`
}

// reviewTypes are the comment types shown as reviews (older WooCommerce
// stored reviews as plain comments).
const reviewTypeWhere = `c.comment_type IN ('review','comment','')`

func (s *Store) scanReviews(ctx context.Context, rows *sql.Rows) ([]ProductReview, error) {
	out := []ProductReview{}
	for rows.Next() {
		var r ProductReview
		var approved, typ string
		var created sql.NullTime
		if err := rows.Scan(&r.ID, &r.ParentID, &r.ProductID, &r.ProductName, &r.Reviewer, &r.ReviewerEmail,
			&r.Review, &approved, &typ, &r.UserID, &created); err != nil {
			rows.Close()
			return nil, err
		}
		r.Status = reviewStatus(approved)
		r.Type = "review"
		if r.ParentID != 0 {
			r.Type = "reply"
		}
		r.CreatedAt = created.Time.UTC()
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]uint64, len(out))
	for i, r := range out {
		ids[i] = r.ID
	}
	mrows, err := s.DB.QueryContext(ctx, `SELECT comment_id, meta_key, meta_value FROM `+s.T("commentmeta")+
		` WHERE comment_id IN (`+inList(ids)+`) AND meta_key IN ('rating','verified')`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	meta := map[uint64]map[string]string{}
	for mrows.Next() {
		var id uint64
		var k string
		var v sql.NullString
		if err := mrows.Scan(&id, &k, &v); err != nil {
			return nil, err
		}
		if meta[id] == nil {
			meta[id] = map[string]string{}
		}
		meta[id][k] = v.String
	}
	for i := range out {
		m := meta[out[i].ID]
		out[i].Rating = int(parseFloat(m["rating"]))
		out[i].Verified = m["verified"] == "1"
	}
	return out, mrows.Err()
}

// ListReviews is the admin list of reviews and replies, newest first.
func (s *Store) ListReviews(ctx context.Context, f ReviewFilter) (Paged[ProductReview], error) {
	page, perPage, offset := Page(f.Page, f.PerPage)
	where := reviewTypeWhere
	args := []any{}
	if f.Status != "" {
		v, ok := reviewStatusToApproved[f.Status]
		if !ok {
			return Paged[ProductReview]{}, refuse(http.StatusBadRequest, "errors.invalidReviewStatus", "Invalid review status")
		}
		if v == "trash" {
			where += " AND c.comment_approved IN ('trash','post-trashed')"
		} else {
			where += " AND c.comment_approved = ?"
			args = append(args, v)
		}
	} else {
		where += " AND c.comment_approved IN ('0','1')"
	}
	if f.ProductID != 0 {
		where += " AND c.comment_post_ID = ?"
		args = append(args, f.ProductID)
	}
	if t := strings.TrimSpace(f.Search); t != "" {
		like := "%" + t + "%"
		where += " AND (c.comment_content LIKE ? OR c.comment_author LIKE ? OR c.comment_author_email LIKE ?)"
		args = append(args, like, like, like)
	}
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*)`+s.reviewFrom()+` WHERE `+where, args...).Scan(&total); err != nil {
		return Paged[ProductReview]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reviewColumns+s.reviewFrom()+` WHERE `+where+
		` ORDER BY c.comment_date_gmt DESC, c.comment_ID DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		return Paged[ProductReview]{}, err
	}
	items, err := s.scanReviews(ctx, rows)
	if err != nil {
		return Paged[ProductReview]{}, err
	}
	return NewPaged(items, total, page, perPage), nil
}

// ProductReviews is a product's approved reviews and replies (shop), oldest
// first like WooCommerce's product page.
func (s *Store) ProductReviews(ctx context.Context, productID uint64) ([]ProductReview, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reviewColumns+s.reviewFrom()+` WHERE `+reviewTypeWhere+
		` AND c.comment_approved = '1' AND c.comment_post_ID = ? AND p.post_status = 'publish'
		ORDER BY c.comment_date_gmt, c.comment_ID`, productID)
	if err != nil {
		return nil, err
	}
	return s.scanReviews(ctx, rows)
}

// GetReview is one review or reply.
func (s *Store) GetReview(ctx context.Context, id uint64) (ProductReview, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reviewColumns+s.reviewFrom()+` WHERE c.comment_ID = ? AND `+reviewTypeWhere, id)
	if err != nil {
		return ProductReview{}, err
	}
	items, err := s.scanReviews(ctx, rows)
	if err != nil {
		return ProductReview{}, err
	}
	if len(items) == 0 {
		return ProductReview{}, ErrNotFound
	}
	return items[0], nil
}

// reviewUser is the WordPress user writing a review or reply.
func (s *Store) reviewUser(ctx context.Context, userID uint64) (name, email string, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT display_name, user_email FROM `+s.T("users")+` WHERE ID = ?`, userID).Scan(&name, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return name, email, err
}

// CustomerBoughtProduct is wc_customer_bought_product: a processing or
// completed order of the user (or billing email) with the product or one of
// its variations.
func (s *Store) CustomerBoughtProduct(ctx context.Context, userID uint64, email string, productID uint64) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if userID == 0 && email == "" {
		return false, nil
	}
	items, itemmeta := s.T("woocommerce_order_items"), s.T("woocommerce_order_itemmeta")
	pid := strconv.FormatUint(productID, 10)
	hpos, _ := s.HPOS(ctx)
	var q string
	var args []any
	if hpos {
		q = `SELECT 1 FROM ` + s.T("wc_orders") + ` o
			JOIN ` + items + ` i ON i.order_id = o.id AND i.order_item_type = 'line_item'
			JOIN ` + itemmeta + ` m ON m.order_item_id = i.order_item_id AND m.meta_key IN ('_product_id','_variation_id') AND m.meta_value = ?
			WHERE o.type = 'shop_order' AND o.status IN ('wc-processing','wc-completed')
			AND ((? <> 0 AND o.customer_id = ?) OR (? <> '' AND LOWER(o.billing_email) = ?)) LIMIT 1`
		args = []any{pid, userID, userID, email, email}
	} else {
		pm := s.T("postmeta")
		q = `SELECT 1 FROM ` + s.T("posts") + ` p
			JOIN ` + items + ` i ON i.order_id = p.ID AND i.order_item_type = 'line_item'
			JOIN ` + itemmeta + ` m ON m.order_item_id = i.order_item_id AND m.meta_key IN ('_product_id','_variation_id') AND m.meta_value = ?
			LEFT JOIN ` + pm + ` cu ON cu.post_id = p.ID AND cu.meta_key = '_customer_user'
			LEFT JOIN ` + pm + ` be ON be.post_id = p.ID AND be.meta_key = '_billing_email'
			WHERE p.post_type = 'shop_order' AND p.post_status IN ('wc-processing','wc-completed')
			AND ((? <> 0 AND cu.meta_value = ?) OR (? <> '' AND LOWER(be.meta_value) = ?)) LIMIT 1`
		args = []any{pid, userID, strconv.FormatUint(userID, 10), email, email}
	}
	var one int
	err := s.DB.QueryRowContext(ctx, q, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CreateReview posts a review from the shop. userID is the signed-in
// customer (0 for a guest, who must then give a name and email unless
// WordPress' require_name_email is off). It honours woocommerce_enable_reviews,
// the product's comment_status, woocommerce_enable_review_rating /
// woocommerce_review_rating_required and
// woocommerce_review_rating_verification_required ("verified owners only"),
// and WordPress' comment_moderation (then the review waits as "hold").
func (s *Store) CreateReview(ctx context.Context, productID uint64, in ReviewInput, userID uint64, ip, agent string) (ProductReview, error) {
	if !s.OptionBool(ctx, "woocommerce_enable_reviews", true) {
		return ProductReview{}, refuse(http.StatusForbidden, "errors.reviewsDisabled", "Reviews are disabled")
	}
	var status, commentStatus string
	err := s.DB.QueryRowContext(ctx, `SELECT post_status, comment_status FROM `+s.T("posts")+` WHERE ID = ? AND post_type = 'product'`,
		productID).Scan(&status, &commentStatus)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "publish") {
		return ProductReview{}, ErrNotFound
	}
	if err != nil {
		return ProductReview{}, err
	}
	if commentStatus != "open" {
		return ProductReview{}, refuse(http.StatusForbidden, "errors.reviewsClosed", "Reviews are closed for this product")
	}
	text := strings.TrimSpace(in.Review)
	if text == "" {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.reviewRequired", "Please write a review")
	}
	ratingOn := s.OptionBool(ctx, "woocommerce_enable_review_rating", true)
	if in.Rating < 0 || in.Rating > 5 {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.invalidRating", "The rating must be between 1 and 5")
	}
	if !ratingOn {
		in.Rating = 0
	} else if in.Rating == 0 && s.OptionBool(ctx, "woocommerce_review_rating_required", true) {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.ratingRequired", "Please select a rating")
	}
	author, email := strings.TrimSpace(in.Author), strings.TrimSpace(in.Email)
	if userID != 0 {
		name, mail, err := s.reviewUser(ctx, userID)
		if err != nil {
			return ProductReview{}, err
		}
		author, email = name, mail
	} else if s.OptionOr(ctx, "require_name_email", "1") == "1" && (author == "" || !strings.Contains(email, "@")) {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.reviewNameEmail", "Please give your name and email")
	}
	verified, err := s.CustomerBoughtProduct(ctx, userID, email, productID)
	if err != nil {
		return ProductReview{}, err
	}
	if !verified && s.OptionBool(ctx, "woocommerce_review_rating_verification_required", false) {
		return ProductReview{}, refuse(http.StatusForbidden, "errors.verifiedOwnersOnly", "Only verified owners can leave a review")
	}
	approved := "1"
	if s.OptionOr(ctx, "comment_moderation", "0") == "1" {
		approved = "0"
	}
	if len(agent) > 254 {
		agent = agent[:254]
	}
	var id uint64
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = s.revInsertComment(ctx, tx, productID, 0, author, email, ip, agent, text, approved, "review", userID)
		if err != nil {
			return err
		}
		if in.Rating > 0 {
			if err := s.revSetCommentMeta(ctx, tx, id, "rating", strconv.Itoa(in.Rating)); err != nil {
				return err
			}
		}
		if err := s.revSetCommentMeta(ctx, tx, id, "verified", map[bool]string{true: "1", false: "0"}[verified]); err != nil {
			return err
		}
		return s.syncProductRating(ctx, tx, productID)
	})
	if err != nil {
		return ProductReview{}, err
	}
	return s.GetReview(ctx, id)
}

func (s *Store) revInsertComment(ctx context.Context, q db.Querier, postID, parent uint64, author, email, ip, agent, content, approved, typ string, userID uint64) (uint64, error) {
	t := now()
	res, err := q.ExecContext(ctx, `INSERT INTO `+s.T("comments")+` (comment_post_ID, comment_author, comment_author_email,
		comment_author_url, comment_author_IP, comment_date, comment_date_gmt, comment_content, comment_karma,
		comment_approved, comment_agent, comment_type, comment_parent, user_id)
		VALUES (?, ?, ?, '', ?, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		postID, author, email, ip, s.localTime(ctx, t), t, content, approved, agent, typ, parent, userID)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

func (s *Store) revSetCommentMeta(ctx context.Context, q db.Querier, commentID uint64, key, value string) error {
	var metaID uint64
	err := q.QueryRowContext(ctx, `SELECT meta_id FROM `+s.T("commentmeta")+` WHERE comment_id = ? AND meta_key = ? ORDER BY meta_id LIMIT 1`,
		commentID, key).Scan(&metaID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = q.ExecContext(ctx, `INSERT INTO `+s.T("commentmeta")+` (comment_id, meta_key, meta_value) VALUES (?, ?, ?)`, commentID, key, value)
		return err
	}
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `UPDATE `+s.T("commentmeta")+` SET meta_value = ? WHERE meta_id = ?`, value, metaID)
	return err
}

// UpdateReview changes a review's status, text or rating (admin).
func (s *Store) UpdateReview(ctx context.Context, id uint64, u ReviewUpdate) (ProductReview, error) {
	cur, err := s.GetReview(ctx, id)
	if err != nil {
		return ProductReview{}, err
	}
	var approved string
	if u.Status != nil {
		v, ok := reviewStatusToApproved[*u.Status]
		if !ok {
			return ProductReview{}, refuse(http.StatusBadRequest, "errors.invalidReviewStatus", "Invalid review status")
		}
		approved = v
	}
	if u.Rating != nil && (*u.Rating < 0 || *u.Rating > 5) {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.invalidRating", "The rating must be between 1 and 5")
	}
	if u.Review != nil && strings.TrimSpace(*u.Review) == "" {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.reviewRequired", "Please write a review")
	}
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if approved != "" && approved != reviewStatusToApproved[cur.Status] {
			if approved == "trash" {
				// wp_trash_comment remembers the status to restore.
				var prev string
				if err := tx.QueryRowContext(ctx, `SELECT comment_approved FROM `+s.T("comments")+` WHERE comment_ID = ?`, id).Scan(&prev); err != nil {
					return err
				}
				if err := s.revSetCommentMeta(ctx, tx, id, "_wp_trash_meta_status", prev); err != nil {
					return err
				}
				if err := s.revSetCommentMeta(ctx, tx, id, "_wp_trash_meta_time", strconv.FormatInt(time.Now().Unix(), 10)); err != nil {
					return err
				}
			} else if cur.Status == "trash" {
				if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("commentmeta")+` WHERE comment_id = ? AND meta_key IN ('_wp_trash_meta_status','_wp_trash_meta_time')`, id); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("comments")+` SET comment_approved = ? WHERE comment_ID = ?`, approved, id); err != nil {
				return err
			}
		}
		if u.Review != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("comments")+` SET comment_content = ? WHERE comment_ID = ?`, strings.TrimSpace(*u.Review), id); err != nil {
				return err
			}
		}
		if u.Rating != nil && cur.ParentID == 0 {
			if *u.Rating == 0 {
				if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("commentmeta")+` WHERE comment_id = ? AND meta_key = 'rating'`, id); err != nil {
					return err
				}
			} else if err := s.revSetCommentMeta(ctx, tx, id, "rating", strconv.Itoa(*u.Rating)); err != nil {
				return err
			}
		}
		return s.syncProductRating(ctx, tx, cur.ProductID)
	})
	if err != nil {
		return ProductReview{}, err
	}
	return s.GetReview(ctx, id)
}

// DeleteReview deletes a review for good (wp_delete_comment: its replies
// move up to its parent).
func (s *Store) DeleteReview(ctx context.Context, id uint64) error {
	cur, err := s.GetReview(ctx, id)
	if err != nil {
		return err
	}
	return s.DB.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE `+s.T("comments")+` SET comment_parent = ? WHERE comment_parent = ?`, cur.ParentID, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("commentmeta")+` WHERE comment_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.T("comments")+` WHERE comment_ID = ?`, id); err != nil {
			return err
		}
		return s.syncProductRating(ctx, tx, cur.ProductID)
	})
}

// ReplyToReview posts the shop's answer to a review as adminUserID (approved).
func (s *Store) ReplyToReview(ctx context.Context, id uint64, text string, adminUserID uint64, ip, agent string) (ProductReview, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ProductReview{}, refuse(http.StatusBadRequest, "errors.reviewRequired", "Please write a reply")
	}
	parent, err := s.GetReview(ctx, id)
	if err != nil {
		return ProductReview{}, err
	}
	name, email, err := s.reviewUser(ctx, adminUserID)
	if err != nil {
		return ProductReview{}, err
	}
	if len(agent) > 254 {
		agent = agent[:254]
	}
	var newID uint64
	err = s.DB.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		newID, err = s.revInsertComment(ctx, tx, parent.ProductID, parent.ID, name, email, ip, agent, text, "1", "comment", adminUserID)
		if err != nil {
			return err
		}
		return s.syncProductRating(ctx, tx, parent.ProductID)
	})
	if err != nil {
		return ProductReview{}, err
	}
	return s.GetReview(ctx, newID)
}

// ------------------------------------------------------------------ rating caches

// ratingStats is a product's approved ratings.
type ratingStats struct {
	counts  map[int]int64
	reviews int64
}

func (r ratingStats) count() int64 {
	var n int64
	for _, c := range r.counts {
		n += c
	}
	return n
}

// average is WC_Comments::get_average_rating_for_product: Σ ratings / count, 2 decimals.
func (r ratingStats) average() float64 {
	n := r.count()
	if n == 0 {
		return 0
	}
	var sum int64
	for rating, c := range r.counts {
		sum += int64(rating) * c
	}
	return math.Round(float64(sum)/float64(n)*100) / 100
}

// serializedCounts is the _wc_rating_count meta: a:N:{i:rating;i:count;...}.
func (r ratingStats) serializedCounts() string {
	keys := make([]int, 0, len(r.counts))
	for k := range r.counts {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	a := php.NewArray()
	for _, k := range keys {
		a.Set(int64(k), r.counts[k])
	}
	return php.Marshal(a)
}

// syncProductRating recomputes a product's rating caches.
func (s *Store) syncProductRating(ctx context.Context, q db.Querier, productID uint64) error {
	st := ratingStats{counts: map[int]int64{}}
	rows, err := q.QueryContext(ctx, `SELECT m.meta_value, COUNT(*) FROM `+s.T("commentmeta")+` m
		JOIN `+s.T("comments")+` c ON c.comment_ID = m.comment_id
		WHERE m.meta_key = 'rating' AND c.comment_post_ID = ? AND c.comment_approved = '1' AND m.meta_value > 0
		GROUP BY m.meta_value`, productID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		var n int64
		if err := rows.Scan(&v, &n); err != nil {
			rows.Close()
			return err
		}
		if r := int(parseFloat(v)); r > 0 {
			st.counts[r] += n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("comments")+` WHERE comment_parent = 0 AND comment_post_ID = ?
		AND comment_approved = '1' AND comment_type IN ('review','comment','')`, productID).Scan(&st.reviews); err != nil {
		return err
	}
	avg := st.average()
	if err := s.setPostMetas(ctx, q, productID, map[string]string{
		"_wc_average_rating": strconv.FormatFloat(avg, 'f', -1, 64),
		"_wc_rating_count":   st.serializedCounts(),
		"_wc_review_count":   strconv.FormatInt(st.reviews, 10),
	}); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `UPDATE `+s.T("posts")+` SET comment_count = (SELECT COUNT(*) FROM `+s.T("comments")+
		` WHERE comment_post_ID = ? AND comment_approved = '1') WHERE ID = ?`, productID, productID); err != nil {
		return err
	}
	// The lookup table may be missing on an old install: not fatal.
	_, _ = q.ExecContext(ctx, `UPDATE `+s.T("wc_product_meta_lookup")+` SET average_rating = ?, rating_count = ? WHERE product_id = ?`,
		avg, st.count(), productID)
	return s.syncRatedTerm(ctx, q, productID, avg)
}

// syncRatedTerm sets the product's rated-N product_visibility term
// (N = round(average), none below 1), like update_visibility does.
func (s *Store) syncRatedTerm(ctx context.Context, q db.Querier, productID uint64, avg float64) error {
	rated := int(math.Min(5, math.Round(avg)))
	tt, tr, terms := s.T("term_taxonomy"), s.T("term_relationships"), s.T("terms")
	rows, err := q.QueryContext(ctx, `SELECT tt.term_taxonomy_id, t.slug FROM `+tr+` r
		JOIN `+tt+` tt ON tt.term_taxonomy_id = r.term_taxonomy_id AND tt.taxonomy = 'product_visibility'
		JOIN `+terms+` t ON t.term_id = tt.term_id
		WHERE r.object_id = ? AND t.slug LIKE 'rated-%'`, productID)
	if err != nil {
		return err
	}
	type cur struct {
		ttID uint64
		slug string
	}
	current := []cur{}
	for rows.Next() {
		var c cur
		if err := rows.Scan(&c.ttID, &c.slug); err != nil {
			rows.Close()
			return err
		}
		current = append(current, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	want := ""
	if rated > 0 {
		want = "rated-" + strconv.Itoa(rated)
	}
	has := false
	touched := []uint64{}
	for _, c := range current {
		if c.slug == want {
			has = true
			continue
		}
		if _, err := q.ExecContext(ctx, `DELETE FROM `+tr+` WHERE object_id = ? AND term_taxonomy_id = ?`, productID, c.ttID); err != nil {
			return err
		}
		touched = append(touched, c.ttID)
	}
	if want != "" && !has {
		ttID, err := s.revVisibilityTerm(ctx, q, want)
		if err != nil {
			return err
		}
		if _, err := q.ExecContext(ctx, `INSERT IGNORE INTO `+tr+` (object_id, term_taxonomy_id, term_order) VALUES (?, ?, 0)`, productID, ttID); err != nil {
			return err
		}
		touched = append(touched, ttID)
	}
	for _, id := range touched {
		if _, err := q.ExecContext(ctx, `UPDATE `+tt+` SET count = (SELECT COUNT(*) FROM `+tr+` WHERE term_taxonomy_id = ?) WHERE term_taxonomy_id = ?`, id, id); err != nil {
			return err
		}
	}
	return nil
}

// revVisibilityTerm is the term_taxonomy_id of a product_visibility term,
// created when missing (WooCommerce creates them on install).
func (s *Store) revVisibilityTerm(ctx context.Context, q db.Querier, slug string) (uint64, error) {
	var id uint64
	err := q.QueryRowContext(ctx, `SELECT tt.term_taxonomy_id FROM `+s.T("term_taxonomy")+` tt JOIN `+s.T("terms")+
		` t ON t.term_id = tt.term_id WHERE tt.taxonomy = 'product_visibility' AND t.slug = ? LIMIT 1`, slug).Scan(&id)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return id, err
	}
	res, err := q.ExecContext(ctx, `INSERT INTO `+s.T("terms")+` (name, slug, term_group) VALUES (?, ?, 0)`, slug, slug)
	if err != nil {
		return 0, err
	}
	termID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	res, err = q.ExecContext(ctx, `INSERT INTO `+s.T("term_taxonomy")+` (term_id, taxonomy, description, parent, count) VALUES (?, 'product_visibility', '', 0, 0)`, termID)
	if err != nil {
		return 0, err
	}
	ttID, err := res.LastInsertId()
	return uint64(ttID), err
}
