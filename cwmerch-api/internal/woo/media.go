package woo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"cwmerch-api/internal/php"
)

// images loads attachment posts as Images.
func (s *Store) images(ctx context.Context, ids []uint64) (map[uint64]Image, error) {
	out := map[uint64]Image{}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT ID, post_title, guid FROM `+s.T("posts")+
		` WHERE ID IN (`+inList(ids)+`) AND post_type = 'attachment'`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var img Image
		var guid sql.NullString
		if err := rows.Scan(&img.ID, &img.Name, &guid); err != nil {
			rows.Close()
			return nil, err
		}
		img.Src = guid.String
		out[img.ID] = img
	}
	rows.Close()
	metas, err := s.postMetas(ctx, s.DB, ids)
	if err != nil {
		return nil, err
	}
	for id, img := range out {
		m := metas[id]
		if file := m["_wp_attached_file"]; file != "" {
			img.Src = s.uploadURL(file)
		}
		img.Alt = m["_wp_attachment_image_alt"]
		out[id] = img
	}
	return out, nil
}

// uploadURL is the public URL of a file relative to wp-content/uploads.
func (s *Store) uploadURL(rel string) string {
	if strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		return rel
	}
	base := s.UploadsURL
	if base == "" {
		base = s.SiteURL + "/wp-content/uploads"
	}
	return base + "/" + strings.TrimLeft(rel, "/")
}

// MediaList pages through image attachments, newest first.
func (s *Store) MediaList(ctx context.Context, page, perPage int) (Paged[Image], error) {
	page, perPage, offset := Page(page, perPage)
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.T("posts")+` WHERE post_type = 'attachment' AND post_mime_type LIKE 'image/%'`).Scan(&total); err != nil {
		return Paged[Image]{}, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT ID FROM `+s.T("posts")+` WHERE post_type = 'attachment' AND post_mime_type LIKE 'image/%'
		ORDER BY post_date_gmt DESC, ID DESC LIMIT ? OFFSET ?`, perPage, offset)
	if err != nil {
		return Paged[Image]{}, err
	}
	ids := []uint64{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Paged[Image]{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	imgs, err := s.images(ctx, ids)
	if err != nil {
		return Paged[Image]{}, err
	}
	items := []Image{}
	for _, id := range ids {
		items = append(items, imgs[id])
	}
	return NewPaged(items, total, page, perPage), nil
}

var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp", "image/svg+xml": ".svg", "image/avif": ".avif",
}

// UploadMedia stores an uploaded image under uploads/YYYY/MM (the layout
// WordPress uses, so a shared uploads volume serves it to both) and creates
// its attachment post.
func (s *Store) UploadMedia(ctx context.Context, filename, contentType string, r io.Reader, authorID uint64, maxBytes int64) (Image, error) {
	if s.UploadsDir == "" {
		return Image{}, refuse(503, "errors.uploadsNotConfigured", "No uploads folder is configured (CWMERCH_UPLOADS_DIR)")
	}
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); allowedImageTypes[byExt] != "" {
			contentType, ext = byExt, allowedImageTypes[byExt]
		} else {
			return Image{}, refuse(400, "errors.unsupportedImage", "Only JPEG, PNG, GIF, WebP, AVIF and SVG images are accepted")
		}
	}
	t := now()
	sub := fmt.Sprintf("%04d/%02d", t.Year(), int(t.Month()))
	dir := filepath.Join(s.UploadsDir, filepath.FromSlash(sub))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Image{}, err
	}
	base := Slugify(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)))
	if base == "" {
		base = "image"
	}
	name := base + ext
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		suffix := make([]byte, 3)
		_, _ = rand.Read(suffix)
		name = base + "-" + hex.EncodeToString(suffix) + ext
	}
	full := filepath.Join(dir, name)
	f, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return Image{}, err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxBytes+1))
	_ = f.Close()
	if err != nil || n > maxBytes {
		_ = os.Remove(full)
		if err == nil {
			return Image{}, refuse(400, "errors.imageTooLarge", "Image is too large")
		}
		return Image{}, err
	}
	rel := path.Join(sub, name)
	url := s.uploadURL(rel)
	local := s.localTime(ctx, t)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO `+s.T("posts")+` (post_author, post_date, post_date_gmt, post_content, post_title, post_excerpt,
		post_status, comment_status, ping_status, post_name, to_ping, pinged, post_modified, post_modified_gmt, post_content_filtered,
		post_parent, guid, menu_order, post_type, post_mime_type, comment_count)
		VALUES (?, ?, ?, '', ?, '', 'inherit', 'open', 'closed', ?, '', '', ?, ?, '', 0, ?, 0, 'attachment', ?, 0)`,
		authorID, local, t, base, Slugify(base), local, t, url, contentType)
	if err != nil {
		return Image{}, err
	}
	id64, _ := res.LastInsertId()
	id := uint64(id64)
	meta := php.NewArray().Set("file", rel).Set("filesize", n).Set("sizes", php.NewArray()).
		Set("image_meta", php.NewArray())
	if err := s.setPostMetas(ctx, s.DB, id, map[string]string{"_wp_attached_file": rel, "_wp_attachment_metadata": php.Marshal(meta)}); err != nil {
		return Image{}, err
	}
	return Image{ID: id, Src: url, Name: base}, nil
}

// MediaFile resolves a path under the uploads folder (for GET /v1/media/*),
// refusing anything that escapes it.
func (s *Store) MediaFile(rel string) (string, bool) {
	if s.UploadsDir == "" {
		return "", false
	}
	clean := filepath.Clean("/" + rel)
	full := filepath.Join(s.UploadsDir, clean)
	if !strings.HasPrefix(full, filepath.Clean(s.UploadsDir)+string(os.PathSeparator)) {
		return "", false
	}
	if st, err := os.Stat(full); err != nil || st.IsDir() {
		return "", false
	}
	return full, true
}

// MediaBytes reads an attachment's file from the uploads folder.
func (s *Store) MediaBytes(id uint64) ([]byte, bool) {
	m, err := s.postMeta(context.Background(), s.DB, id)
	if err != nil {
		return nil, false
	}
	path, ok := s.MediaFile(m["_wp_attached_file"])
	if !ok || !strings.HasSuffix(strings.ToLower(path), ".png") {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	return raw, err == nil
}
