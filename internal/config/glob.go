package config

import (
	"os"
	"sort"
	"strings"
)

// ExpandServiceDirectories expands each SERVICE_DIRECTORIES pattern the way bash's pathname
// expansion did: matches sorted byte-wise, a wildcard never matching a leading dot, and a
// pattern that matches nothing tried as a literal path (so a directory named with glob
// characters still works). Only directories count; a pattern that yields none is returned
// in unmatched, since everything under it is silently not backed up.
func ExpandServiceDirectories(patterns []string) (dirs, unmatched []string) {
	for _, pattern := range patterns {
		var found []string
		for _, m := range glob(pattern) {
			if isDir(m) {
				found = append(found, strings.TrimSuffix(m, "/"))
			}
		}
		if len(found) == 0 {
			unmatched = append(unmatched, pattern)
			continue
		}
		dirs = append(dirs, found...)
	}
	return dirs, unmatched
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// glob returns the pattern's matches, or the pattern itself when it has no wildcard or
// matches nothing, as an unquoted bash word does with nullglob off.
func glob(pattern string) []string {
	if !hasMeta(pattern) {
		return []string{pattern}
	}
	trailing := strings.HasSuffix(pattern, "/")
	parts := strings.Split(strings.TrimRight(pattern, "/"), "/")
	paths := []string{""}
	if parts[0] == "" { // absolute
		paths = []string{"/"}
		parts = parts[1:]
	}
	for _, part := range parts {
		var next []string
		for _, base := range paths {
			if !hasMeta(part) {
				next = append(next, join(base, unescape(part)))
				continue
			}
			dir := base
			if dir == "" {
				dir = "."
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if name := e.Name(); match(part, name) {
					next = append(next, join(base, name))
				}
			}
		}
		paths = next
	}
	var out []string
	for _, p := range paths {
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		if trailing {
			if !isDir(p) {
				continue
			}
			p += "/"
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{pattern}
	}
	sort.Strings(out)
	return out
}

func join(base, name string) string {
	switch base {
	case "":
		return name
	case "/":
		return "/" + name
	}
	return base + "/" + name
}

func hasMeta(s string) bool { return strings.ContainsAny(s, "*?[") }

// unescape removes the backslashes bash's quote removal drops from a literal part.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// match reports whether name matches one bash pattern component, byte by byte as in the C
// locale. A leading dot in name must be matched by a literal dot, escaped or not.
func match(pattern, name string) bool {
	if strings.HasPrefix(name, ".") && !strings.HasPrefix(pattern, ".") && !strings.HasPrefix(pattern, `\.`) {
		return false
	}
	return matchAt(pattern, name)
}

func matchAt(p, s string) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if p == "" {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if matchAt(p, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if s == "" {
				return false
			}
			p, s = p[1:], s[1:]
		case '[':
			if end, ok := classMatch(p, s); end > 0 {
				if !ok {
					return false
				}
				p, s = p[end:], s[1:]
				continue
			}
			// An unclosed [ is a literal.
			if s == "" || s[0] != '[' {
				return false
			}
			p, s = p[1:], s[1:]
		case '\\':
			if len(p) > 1 {
				p = p[1:]
			}
			fallthrough
		default:
			if s == "" || s[0] != p[0] {
				return false
			}
			p, s = p[1:], s[1:]
		}
	}
	return s == ""
}

// classMatch matches s's first byte against the bracket expression at the start of p, by
// bash's rules: ! or ^ negates, a ] first is literal, - first or last is literal,
// [:name:] is a C-locale class, and [=c=] or [.c.] is the character c. It returns the
// length of the expression, or 0 when the [ has no closing ] (and so is a literal).
func classMatch(p, s string) (int, bool) {
	i := 1
	negate := false
	if i < len(p) && (p[i] == '!' || p[i] == '^') {
		negate = true
		i++
	}
	matched := false
	var c byte
	if s != "" {
		c = s[0]
	}
	for first := true; i < len(p); first = false {
		if p[i] == ']' && !first {
			return i + 1, s != "" && matched != negate
		}
		if p[i] == '[' && i+1 < len(p) && p[i+1] == ':' {
			// The closing :] comes after the opening [: (in "[:]" they overlap: no class).
			if e := strings.Index(p[i:], ":]"); e >= 2 {
				if in, ok := classes[p[i+2:i+e]]; ok {
					matched = matched || (s != "" && in(c))
					i += e + 2
					continue
				}
			}
		}
		// [=c=] and [.c.] name one character in the C locale.
		if p[i] == '[' && i+2 < len(p) && (p[i+1] == '=' || p[i+1] == '.') {
			if e := strings.Index(p[i+2:], string(p[i+1])+"]"); e == 1 {
				matched = matched || (s != "" && c == p[i+2])
				i += 5
				continue
			}
		}
		lo := p[i]
		if lo == '\\' && i+1 < len(p) {
			i++
			lo = p[i]
		}
		i++
		hi := lo
		if i+1 < len(p) && p[i] == '-' && p[i+1] != ']' {
			hi = p[i+1]
			if hi == '\\' && i+2 < len(p) {
				hi = p[i+2]
				i++
			}
			i += 2
		}
		if s != "" && lo <= c && c <= hi {
			matched = true
		}
	}
	return 0, false
}

// classes are the POSIX character classes, as the C locale defines them.
var classes = map[string]func(byte) bool{
	"alnum":  func(c byte) bool { return isAlpha(c) || isDigit(c) },
	"alpha":  isAlpha,
	"blank":  func(c byte) bool { return c == ' ' || c == '\t' },
	"cntrl":  func(c byte) bool { return c < 0x20 || c == 0x7f },
	"digit":  isDigit,
	"graph":  func(c byte) bool { return c > 0x20 && c < 0x7f },
	"lower":  func(c byte) bool { return c >= 'a' && c <= 'z' },
	"print":  func(c byte) bool { return c >= 0x20 && c < 0x7f },
	"punct":  func(c byte) bool { return c > 0x20 && c < 0x7f && !isAlpha(c) && !isDigit(c) },
	"space":  func(c byte) bool { return c == ' ' || c >= '\t' && c <= '\r' },
	"upper":  func(c byte) bool { return c >= 'A' && c <= 'Z' },
	"xdigit": func(c byte) bool { return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' },
	"word":   func(c byte) bool { return isAlpha(c) || isDigit(c) || c == '_' },
}

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// MatchName reports whether a service name matches one pattern component as
// ExpandServiceDirectories matches it (bash rules: a leading dot is never matched by a
// wildcard, [!...] negates, POSIX classes), for placing services that do not exist yet.
func MatchName(pattern, name string) bool { return match(pattern, name) }

// HasMeta reports whether a path or component holds glob characters.
func HasMeta(s string) bool { return hasMeta(s) }

// Unescape removes a pattern's backslash escapes, as expansion does to a path it uses
// literally (tenant\-one is the directory tenant-one).
func Unescape(s string) string { return unescape(s) }
