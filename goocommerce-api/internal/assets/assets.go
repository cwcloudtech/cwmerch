// Package assets embeds goocommerce's static binary assets so they can be
// shared across packages (report PDFs, transactional emails, ...) without
// each one needing its own copy or a go:embed reaching outside its
// directory tree.
package assets

import _ "embed"

//go:embed goocommerce-logo.png
var LogoPNG []byte
