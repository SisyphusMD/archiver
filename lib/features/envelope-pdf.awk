# Lays out the envelope's records (kind TAB text, one per line, from envelope_write_pdf,
# already in Windows-1252) on US Letter pages and writes a PDF 1.4 file to stdout. Run under
# LC_ALL=C, so length() counts bytes, which the cross-reference table's offsets need.
#
# Kinds: title, sub, h, p (wrapped text), m (monospace, wrapped), warn (boxed bold),
# qr (rows of 0/1 modules separated by '|'), notes ("label|lines"), rule. A qr floats at the
# right of its block: the lines after it wrap beside it until they pass its bottom, and the
# next block starts below it.

# esc(s): s as a PDF string literal's content. "&" in a replacement is the matched text, which
# sidesteps awk's own backslash rules: each \ ( ) gets a backslash before it.
function esc(s) {
  gsub(/[^\040-\176\200-\377]/, "?", s)
  gsub(/\\/, "&&", s)
  gsub(/[()]/, "\\\\&", s)
  return s
}

# wrap(s, width): s split at spaces into lines of at most width characters (longer words
# are cut), into the global array W; returns the line count.
function wrap(s, width,    n, line, words, nw, i, w) {
  n = 0; line = ""
  nw = split(s, words, " ")
  for (i = 1; i <= nw; i++) {
    w = words[i]
    while (length(w) > width) {
      if (line != "") { W[++n] = line; line = "" }
      W[++n] = substr(w, 1, width); w = substr(w, width + 1)
    }
    if (line == "") line = w
    else if (length(line) + 1 + length(w) <= width) line = line " " w
    else { W[++n] = line; line = w }
  }
  if (line != "" || n == 0) W[++n] = line
  return n
}

function newpage() {
  npages++
  y = TOP
  fbottom = TOP + 1   # no float
}

# need(h): start a new page unless h points fit above the bottom margin.
function need(h) { if (y - h < BOTTOM) newpage() }

# right(): the right edge for a line at the current y, short of a float beside it.
function right() { return (y > fbottom) ? fleft - 10 : RIGHT }

# clear(): move below any float, ending a block.
function clear() { if (y > fbottom) y = fbottom; fbottom = TOP + 1 }

function text(font, size, x, s) {
  page[npages] = page[npages] sprintf("BT /%s %.1f Tf %.1f %.1f Td (%s) Tj ET\n", font, size, x, y, esc(s))
}

# lines(font, size, lead, charw, s): s wrapped word by word to the width at hand (narrower
# beside a float), at charw points per character, one line per lead points. A word longer
# than a line is cut.
function lines(font, size, lead, charw, s,    words, nw, i, width, line, w) {
  nw = split(s, words, " ")
  i = 1
  do {
    need(lead); y -= lead
    width = int((right() - LEFT) / charw)
    line = ""
    while (i <= nw) {
      w = words[i]
      if (line == "") {
        if (length(w) > width) { line = substr(w, 1, width); words[i] = substr(w, width + 1); break }
        line = w; i++
      } else if (length(line) + 1 + length(w) <= width) { line = line " " w; i++ }
      else break
    }
    text(font, size, LEFT, line)
  } while (i <= nw)
}

# exact(font, size, lead, charw, s): s cut into lines at exact character counts, keeping
# every space, for credentials that must be typed back exactly.
function exact(font, size, lead, charw, s,    width) {
  do {
    need(lead); y -= lead
    width = int((right() - LEFT) / charw)
    text(font, size, LEFT, substr(s, 1, width))
    s = substr(s, width + 1)
  } while (s != "")
}

BEGIN {
  LEFT = 42; RIGHT = 570; TOP = 766; BOTTOM = 34
  npages = 0; newpage()
}

{
  kind = $1; s = substr($0, length($1) + 2)
  if (kind == "title")      { lines("F1", 16, 20, 9.2, s); y -= 2 }
  else if (kind == "sub")   { lines("F3", 8.5, 11, 4.5, s); y -= 4 }
  else if (kind == "h")     { clear(); need(30); y -= 6; lines("F1", 11, 14, 6.4, s) }
  else if (kind == "p")     { lines("F3", 9.5, 12, 4.95, s); y -= 2 }
  else if (kind == "m")     { exact("F2", 8.5, 10.5, 5.1, s) }
  else if (kind == "warn") {
    edge = right()
    n = wrap(s, int((edge - LEFT - 8) / 5.6))
    need(n * 12 + 8); y -= 4
    top = y
    for (i = 1; i <= n; i++) { y -= 12; text("F1", 9.5, LEFT + 4, W[i]) }
    y -= 4
    page[npages] = page[npages] sprintf("1.2 w %.1f %.1f %.1f %.1f re S\n", LEFT, y, edge - LEFT, top - y)
    y -= 2
  }
  else if (kind == "qr") {
    rows = split(s, R, "|"); cols = length(R[1])
    mod = 2.2; if (rows * mod > 150) mod = 150 / rows
    size = rows * mod
    need(size + 4)
    fleft = RIGHT - cols * mod
    out = "0 g\n"
    for (r = 1; r <= rows; r++) {
      row = R[r]; c = 1
      while (c <= cols) {
        if (substr(row, c, 1) != "1") { c++; continue }
        start = c
        while (c <= cols && substr(row, c, 1) == "1") c++
        out = out sprintf("%.2f %.2f %.2f %.2f re ", fleft + (start - 1) * mod, y - r * mod, (c - start) * mod, mod)
      }
      out = out "\n"
    }
    page[npages] = page[npages] out "f\n"
    fbottom = y - size - 4
  }
  else if (kind == "notes") {
    clear()
    split(s, N, "|")
    lines("F1", 9.5, 12, 5.6, N[1])
    for (i = 1; i <= N[2] + 0; i++) {
      need(18); y -= 18
      page[npages] = page[npages] sprintf("0.6 w %.1f %.1f m %.1f %.1f l S\n", LEFT, y, RIGHT, y)
    }
  }
  else if (kind == "rule") {
    clear(); need(14); y -= 8
    page[npages] = page[npages] sprintf("0.8 w %.1f %.1f m %.1f %.1f l S\n", LEFT, y, RIGHT, y)
    y -= 2
  }
}

# obj(body): append object number nobj with body, recording its byte offset.
function obj(body) {
  nobj++
  off[nobj] = pos
  chunk = nobj " 0 obj\n" body "\nendobj\n"
  printf "%s", chunk
  pos += length(chunk)
}

END {
  head = "%PDF-1.4\n"
  printf "%s", head; pos = length(head)
  # 1 catalog, 2 pages, 3-5 fonts, then a page and its content per page.
  kids = ""
  for (p = 1; p <= npages; p++) kids = kids (6 + (p - 1) * 2) " 0 R "
  obj("<< /Type /Catalog /Pages 2 0 R >>")
  obj("<< /Type /Pages /Kids [ " kids "] /Count " npages " >>")
  obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
  obj("<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>")
  obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
  for (p = 1; p <= npages; p++) {
    obj("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents " (7 + (p - 1) * 2) " 0 R /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >> >>")
    content = page[p]
    obj("<< /Length " length(content) " >>\nstream\n" content "endstream")
  }
  xref = pos
  printf "xref\n0 %d\n0000000000 65535 f \n", nobj + 1
  for (i = 1; i <= nobj; i++) printf "%010d 00000 n \n", off[i]
  printf "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", nobj + 1, xref
}
