// Package mdpdf renders the Markdown subset invoice templates use into a PDF:
// headings (#, ##, ###), lines and paragraphs with **bold**, bullet lists,
// horizontal rules (---), and pipe tables whose cells wrap (a `---:`
// separator right-aligns a column, for amounts). Every line of a paragraph is
// kept as written, so templates lay out addresses line by line without
// Markdown's trailing-double-space convention.
package mdpdf

import (
	"bytes"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Options tune a document.
type Options struct {
	LogoPNG    []byte
	LogoHeight float64 // mm, default 14
	Footer     string
	Title      string
	Accent     [3]int // headings and table header colour
}

const margin = 15.0

type doc struct {
	pdf *fpdf.Fpdf
	tr  func(string) string
	opt Options
}

// Render converts Markdown to PDF bytes (A4 portrait).
func Render(markdown string, opt Options) ([]byte, error) {
	if opt.LogoHeight == 0 {
		opt.LogoHeight = 14
	}
	if opt.Accent == [3]int{} {
		opt.Accent = [3]int{37, 99, 175}
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(margin, margin, margin)
	pdf.SetAutoPageBreak(true, 18)
	pdf.SetTitle(opt.Title, true)
	pdf.SetCreator("cwmerch", true)
	d := &doc{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), opt: opt}
	pdf.SetFooterFunc(func() {
		pdf.SetY(-13)
		pdf.SetFont("Helvetica", "I", 7.5)
		pdf.SetTextColor(140, 140, 140)
		w, _ := pdf.GetPageSize()
		text := opt.Footer
		pdf.SetX(margin)
		pdf.MultiCell(w-2*margin-20, 3.5, d.tr(text), "", "L", false)
		pdf.SetXY(w-margin-20, -13)
		pdf.CellFormat(20, 3.5, d.tr(pageLabel(pdf.PageNo())), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()
	if len(opt.LogoPNG) > 0 {
		info := pdf.RegisterImageOptionsReader("logo", fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(opt.LogoPNG))
		if info != nil && info.Height() > 0 {
			lw := info.Width() * opt.LogoHeight / info.Height()
			pageW, _ := pdf.GetPageSize()
			pdf.ImageOptions("logo", pageW-margin-lw, margin-4, lw, opt.LogoHeight, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
		}
	}
	pdf.SetTextColor(30, 30, 30)
	d.blocks(strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n"))
	if pdf.Err() {
		return nil, pdf.Error()
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pageLabel(n int) string { return "Page " + itoa(n) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func (d *doc) width() float64 {
	w, _ := d.pdf.GetPageSize()
	return w - 2*margin
}

func (d *doc) blocks(lines []string) {
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			d.pdf.Ln(2.5)
		case strings.HasPrefix(trimmed, "### "):
			d.heading(strings.TrimPrefix(trimmed, "### "), 11)
		case strings.HasPrefix(trimmed, "## "):
			d.heading(strings.TrimPrefix(trimmed, "## "), 13)
		case strings.HasPrefix(trimmed, "# "):
			d.heading(strings.TrimPrefix(trimmed, "# "), 18)
		case trimmed == "---" || trimmed == "***" || trimmed == "___":
			y := d.pdf.GetY() + 1.5
			d.pdf.SetDrawColor(200, 200, 200)
			d.pdf.Line(margin, y, margin+d.width(), y)
			d.pdf.Ln(3.5)
		case strings.HasPrefix(trimmed, "|"):
			j := i
			for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
				j++
			}
			d.table(lines[i:j])
			i = j - 1
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			d.richLine("•  "+strings.TrimSpace(trimmed[2:]), 9.5, 5)
		default:
			d.richLine(trimmed, 9.5, 0)
		}
	}
}

func (d *doc) heading(text string, size float64) {
	d.pdf.Ln(1)
	d.pdf.SetFont("Helvetica", "B", size)
	d.pdf.SetTextColor(d.opt.Accent[0], d.opt.Accent[1], d.opt.Accent[2])
	d.pdf.MultiCell(d.width()*0.7, size*0.45, d.tr(stripBold(text)), "", "L", false)
	d.pdf.SetTextColor(30, 30, 30)
	d.pdf.Ln(1)
}

func stripBold(s string) string { return strings.ReplaceAll(s, "**", "") }

// richLine writes one line with **bold** runs, wrapping on the page width.
func (d *doc) richLine(text string, size, indent float64) {
	d.pdf.SetX(margin + indent)
	parts := strings.Split(text, "**")
	lineH := size * 0.48
	for i, p := range parts {
		if p == "" {
			continue
		}
		style := ""
		if i%2 == 1 {
			style = "B"
		}
		d.pdf.SetFont("Helvetica", style, size)
		d.pdf.Write(lineH, d.tr(p))
	}
	d.pdf.Ln(lineH)
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		c = strings.Trim(c, ": ")
		if c == "" || strings.Trim(c, "-") != "" {
			return false
		}
	}
	return len(cells) > 0
}

// table draws a pipe table: widths from content, wrapped cells, zebra rows.
func (d *doc) table(lines []string) {
	rows := [][]string{}
	align := []string{}
	header := -1
	for _, l := range lines {
		cells := splitRow(l)
		if isSeparator(cells) {
			header = len(rows) - 1
			align = make([]string, len(cells))
			for i, c := range cells {
				align[i] = "L"
				if strings.HasSuffix(c, ":") && strings.HasPrefix(c, ":") {
					align[i] = "C"
				} else if strings.HasSuffix(c, ":") {
					align[i] = "R"
				}
			}
			continue
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	for len(align) < cols {
		align = append(align, "L")
	}
	// natural widths, capped, then scaled to the page
	d.pdf.SetFont("Helvetica", "", 9)
	natural := make([]float64, cols)
	for _, r := range rows {
		for i := 0; i < cols && i < len(r); i++ {
			w := d.pdf.GetStringWidth(d.tr(stripBold(r[i]))) + 4
			if w > 90 {
				w = 90
			}
			if w > natural[i] {
				natural[i] = w
			}
		}
	}
	total := 0.0
	for _, w := range natural {
		total += w
	}
	widths := make([]float64, cols)
	for i := range natural {
		widths[i] = natural[i] * d.width() / total
	}
	d.pdf.Ln(1)
	for ri, r := range rows {
		isHeader := ri <= header
		if isHeader && strings.TrimSpace(strings.Join(r, "")) == "" {
			continue // an empty header row ("| | |") only exists to start a table
		}
		style := ""
		if isHeader {
			style = "B"
		}
		d.pdf.SetFont("Helvetica", style, 9)
		// height = tallest wrapped cell
		lineH := 4.6
		maxLines := 1
		cellLines := make([][]string, cols)
		for i := 0; i < cols; i++ {
			text := ""
			if i < len(r) {
				text = r[i]
			}
			bold := strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") && len(text) > 4
			if bold {
				d.pdf.SetFont("Helvetica", "B", 9)
			}
			cellLines[i] = []string{}
			cellLines[i] = d.wrap(stripBold(text), widths[i]-3)
			if len(cellLines[i]) == 0 {
				cellLines[i] = []string{""}
			}
			if len(cellLines[i]) > maxLines {
				maxLines = len(cellLines[i])
			}
			d.pdf.SetFont("Helvetica", style, 9)
		}
		h := float64(maxLines)*lineH + 2
		_, pageH := d.pdf.GetPageSize()
		if d.pdf.GetY()+h > pageH-20 {
			d.pdf.AddPage()
		}
		y := d.pdf.GetY()
		x := margin
		for i := 0; i < cols; i++ {
			fill := false
			if isHeader {
				d.pdf.SetFillColor(d.opt.Accent[0], d.opt.Accent[1], d.opt.Accent[2])
				d.pdf.SetTextColor(255, 255, 255)
				fill = true
			} else if (ri-header)%2 == 0 {
				d.pdf.SetFillColor(245, 247, 250)
				fill = true
			}
			if fill {
				d.pdf.Rect(x, y, widths[i], h, "F")
			}
			text := ""
			if i < len(r) {
				text = r[i]
			}
			if strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") && len(text) > 4 && !isHeader {
				d.pdf.SetFont("Helvetica", "B", 9)
			}
			for li, l := range cellLines[i] {
				d.pdf.SetXY(x+1.5, y+1+float64(li)*lineH)
				d.pdf.CellFormat(widths[i]-3, lineH, d.tr(l), "", 0, align[i], false, 0, "")
			}
			d.pdf.SetFont("Helvetica", style, 9)
			d.pdf.SetTextColor(30, 30, 30)
			x += widths[i]
		}
		d.pdf.SetDrawColor(225, 228, 233)
		d.pdf.Line(margin, y+h, margin+d.width(), y+h)
		d.pdf.SetXY(margin, y+h)
	}
	d.pdf.Ln(2)
}

// wrap splits text into lines fitting width (measured in the current font),
// breaking long words when needed. Lines are returned untranslated.
func (d *doc) wrap(text string, width float64) []string {
	fits := func(s string) bool { return d.pdf.GetStringWidth(d.tr(s)) <= width }
	out := []string{}
	cur := ""
	for _, word := range strings.Fields(text) {
		candidate := word
		if cur != "" {
			candidate = cur + " " + word
		}
		if fits(candidate) {
			cur = candidate
			continue
		}
		if cur != "" {
			out = append(out, cur)
		}
		cur = ""
		for !fits(word) && len([]rune(word)) > 1 {
			r := []rune(word)
			n := len(r) - 1
			for n > 1 && !fits(string(r[:n])) {
				n--
			}
			out = append(out, string(r[:n]))
			word = string(r[n:])
		}
		cur = word
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
