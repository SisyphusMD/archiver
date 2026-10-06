package envelope

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrUnprintable means some text on the page has characters the PDF's fonts cannot show (or
// control characters such as a tab): printing it would put a credential on paper that cannot
// be typed back, so no PDF is written.
var ErrUnprintable = errors.New("text the PDF's fonts cannot show")

// cp1252High maps the characters Windows-1252 places at 0x80-0x9F.
var cp1252High = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87, 'ˆ': 0x88, '‰': 0x89,
	'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
	'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// toCP1252 converts UTF-8 text to Windows-1252, the standard fonts' encoding.
func toCP1252(s string) (string, error) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '�':
			return "", ErrUnprintable // invalid UTF-8
		case r < 0x80 || (r >= 0xA0 && r <= 0xFF):
			b.WriteByte(byte(r))
		default:
			c, ok := cp1252High[r]
			if !ok {
				return "", ErrUnprintable
			}
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

// qrModules is a QR code of data as rows of '1' (dark) and '0', two modules of margin.
func qrModules(data string) (string, error) {
	cmd := exec.Command("qrencode", "-t", "ASCII", "-m", "2", "-o", "-")
	cmd.Stdin = strings.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// qrencode's ASCII output draws each module as two characters, '#' for dark.
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		rows = append(rows, strings.NewReplacer("##", "1", "  ", "0").Replace(line))
	}
	return strings.Join(rows, "|"), nil
}

// PDF draws the page as a PDF 1.4 file on US Letter in the standard fonts PDF readers carry
// built in (Helvetica, Courier), with QR codes as filled squares: no renderer needed.
func (p *Page) PDF() ([]byte, error) {
	for _, e := range p.Elements {
		if e.Kind != "qr" && strings.IndexFunc(e.Text, isControl) >= 0 {
			return nil, ErrUnprintable
		}
	}
	r := &renderer{}
	r.newPage()
	for _, e := range p.Elements {
		text := e.Text
		if e.Kind == "qr" {
			m, err := qrModules(text)
			if err != nil {
				return nil, err
			}
			text = m
		}
		t, err := toCP1252(text)
		if err != nil {
			return nil, err
		}
		r.element(e.Kind, t)
	}
	return r.document(), nil
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7F || (r >= 0x80 && r < 0xA0) }

const (
	left, rightEdge, top, bottom = 42.0, 570.0, 766.0, 34.0
)

// renderer lays out elements top to bottom. A qr floats at the right of its block: the lines
// after it wrap beside it until they pass its bottom, and the next block starts below it.
type renderer struct {
	pages   []*bytes.Buffer
	y       float64
	fbottom float64 // the float's bottom; top+1 when there is none
	fleft   float64
}

func (r *renderer) out() *bytes.Buffer { return r.pages[len(r.pages)-1] }

func (r *renderer) newPage() {
	r.pages = append(r.pages, &bytes.Buffer{})
	r.y = top
	r.fbottom = top + 1
}

// need starts a new page unless h points fit above the bottom margin.
func (r *renderer) need(h float64) {
	if r.y-h < bottom {
		r.newPage()
	}
}

// right is the right edge for a line at the current y, short of a float beside it.
func (r *renderer) right() float64 {
	if r.y > r.fbottom {
		return r.fleft - 10
	}
	return rightEdge
}

// clear moves below any float, ending a block.
func (r *renderer) clear() {
	if r.y > r.fbottom {
		r.y = r.fbottom
	}
	r.fbottom = top + 1
}

// esc is s as a PDF string literal's content: control bytes become '?', and \ ( ) are escaped.
func esc(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c < 0x20 || c == 0x7F:
			b.WriteByte('?')
		case c == '\\' || c == '(' || c == ')':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func (r *renderer) text(font string, size, x float64, s string) {
	fmt.Fprintf(r.out(), "BT /%s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", font, size, x, r.y, esc(s))
}

// words splits at runs of spaces, as awk's split(s, a, " ") does.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' })
}

// lines writes s wrapped word by word to the width at hand (narrower beside a float), at
// charw points per character, one line per lead points. A word longer than a line is cut.
func (r *renderer) lines(font string, size, lead, charw float64, s string) {
	ws := words(s)
	i := 0
	for {
		r.need(lead)
		r.y -= lead
		width := int((r.right() - left) / charw)
		line := ""
		for i < len(ws) {
			w := ws[i]
			if line == "" {
				if len(w) > width {
					line, ws[i] = w[:width], w[width:]
					break
				}
				line = w
				i++
			} else if len(line)+1+len(w) <= width {
				line += " " + w
				i++
			} else {
				break
			}
		}
		r.text(font, size, left, line)
		if i >= len(ws) {
			return
		}
	}
}

// exact writes s cut into lines at exact character counts, keeping every space, for
// credentials that must be typed back exactly.
func (r *renderer) exact(font string, size, lead, charw float64, s string) {
	for {
		r.need(lead)
		r.y -= lead
		width := int((r.right() - left) / charw)
		n := min(width, len(s))
		r.text(font, size, left, s[:n])
		if s = s[n:]; s == "" {
			return
		}
	}
}

// wrap splits s at spaces into lines of at most width characters (longer words cut).
func wrap(s string, width int) []string {
	var out []string
	line := ""
	for _, w := range words(s) {
		for len(w) > width {
			if line != "" {
				out = append(out, line)
				line = ""
			}
			out = append(out, w[:width])
			w = w[width:]
		}
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= width:
			line += " " + w
		default:
			out = append(out, line)
			line = w
		}
	}
	if line != "" || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

func (r *renderer) element(kind, s string) {
	switch kind {
	case "title":
		r.lines("F1", 16, 20, 9.2, s)
		r.y -= 2
	case "sub":
		r.lines("F3", 8.5, 11, 4.5, s)
		r.y -= 4
	case "h":
		r.clear()
		r.need(30)
		r.y -= 6
		r.lines("F1", 11, 14, 6.4, s)
	case "p":
		r.lines("F3", 9.5, 12, 4.95, s)
		r.y -= 2
	case "m", "cmd":
		r.exact("F2", 8.5, 10.5, 5.1, s)
	case "warn":
		edge := r.right()
		ls := wrap(s, int((edge-left-8)/5.6))
		r.need(float64(len(ls))*12 + 8)
		r.y -= 4
		boxTop := r.y
		for _, l := range ls {
			r.y -= 12
			r.text("F1", 9.5, left+4, l)
		}
		r.y -= 4
		fmt.Fprintf(r.out(), "1.2 w %.1f %.1f %.1f %.1f re S\n", left, r.y, edge-left, boxTop-r.y)
		r.y -= 2
	case "qr":
		rows := strings.Split(s, "|")
		cols := len(rows[0])
		mod := 2.2
		if float64(len(rows))*mod > 150 {
			mod = 150 / float64(len(rows))
		}
		size := float64(len(rows)) * mod
		r.need(size + 4)
		r.fleft = rightEdge - float64(cols)*mod
		var b strings.Builder
		b.WriteString("0 g\n")
		for ri, row := range rows {
			for c := 0; c < cols; {
				if c >= len(row) || row[c] != '1' {
					c++
					continue
				}
				start := c
				for c < cols && c < len(row) && row[c] == '1' {
					c++
				}
				fmt.Fprintf(&b, "%.2f %.2f %.2f %.2f re ", r.fleft+float64(start)*mod, r.y-float64(ri+1)*mod, float64(c-start)*mod, mod)
			}
			b.WriteString("\n")
		}
		r.out().WriteString(b.String() + "f\n")
		r.fbottom = r.y - size - 4
	case "notes":
		r.clear()
		label, count, _ := strings.Cut(s, "|")
		r.lines("F1", 9.5, 12, 5.6, label)
		n := 0
		fmt.Sscanf(count, "%d", &n)
		for range n {
			r.need(18)
			r.y -= 18
			fmt.Fprintf(r.out(), "0.6 w %.1f %.1f m %.1f %.1f l S\n", left, r.y, rightEdge, r.y)
		}
	case "rule":
		r.clear()
		r.need(14)
		r.y -= 8
		fmt.Fprintf(r.out(), "0.8 w %.1f %.1f m %.1f %.1f l S\n", left, r.y, rightEdge, r.y)
		r.y -= 2
	}
}

func (r *renderer) document() []byte {
	var doc bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, doc.Len())
		fmt.Fprintf(&doc, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	doc.WriteString("%PDF-1.4\n")
	// 1 catalog, 2 pages, 3-5 fonts, then a page and its content per page.
	var kids strings.Builder
	for i := range r.pages {
		fmt.Fprintf(&kids, "%d 0 R ", 6+i*2)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [ %s] /Count %d >>", kids.String(), len(r.pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	for i, page := range r.pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >> >>", 7+i*2))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", page.Len(), page.String()))
	}
	xref := doc.Len()
	fmt.Fprintf(&doc, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&doc, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&doc, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return doc.Bytes()
}
