package directive

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Parse scans src for directives. file is used only to label positions; nothing
// is read from disk. src is treated as opaque text — no D2 syntax is parsed.
//
// Parse never fails: malformed lines become [SyntaxError] entries in the
// result. A malformed binding must not be able to stop a render.
func Parse(file string, src []byte) Result {
	var res Result
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	for i, raw := range strings.Split(text, "\n") {
		pos := Position{File: file, Line: i + 1}
		d, serr := parseLine(pos, raw)
		switch {
		case serr != nil:
			res.Syntax = append(res.Syntax, *serr)
		case d != nil:
			res.Directives = append(res.Directives, *d)
		}
	}
	return res
}

// ParseReader reads r to completion and parses it. The read error, if any, is
// returned unwrapped; the caller decides whether that is a tool error.
func ParseReader(file string, r io.Reader) (Result, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", file, err)
	}
	return Parse(file, src), nil
}

// ParseFile reads and parses a single `.d2` file. This is the only filesystem
// access this package performs, and it is limited to its own named input.
func ParseFile(path string) (Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(path, src), nil
}

// ParseFiles parses several `.d2` files and merges the results in the order
// given. It stops at the first unreadable file.
func ParseFiles(paths ...string) (Result, error) {
	var out Result
	for _, p := range paths {
		res, err := ParseFile(p)
		if err != nil {
			return Result{}, err
		}
		out.Merge(res)
	}
	return out, nil
}

// parseLine classifies one source line. It returns (nil, nil) for anything that
// is not a directive candidate — including plain comments, which are not errors.
func parseLine(pos Position, raw string) (*Directive, *SyntaxError) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "#") {
		// Not a comment line. Trestle does not read directives out of trailing
		// comments; a directive owns its line.
		return nil, nil
	}
	// Strip the leading run of '#' so that `## @bind ...` is still seen. A
	// binding that is silently ignored is worse than one reported as SYNTAX.
	body := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	if !strings.HasPrefix(body, "@") {
		// A plain comment. `# just a note` is not a directive and not an error.
		return nil, nil
	}

	fields := strings.Fields(body)
	kind := Kind(fields[0])

	bad := func(detail string) (*Directive, *SyntaxError) {
		return nil, &SyntaxError{Source: pos, Raw: trimmed, Detail: detail, Want: Form(kind)}
	}

	switch kind {
	case KindBind:
		switch {
		case len(fields) == 1:
			return bad("@bind requires a node ID and a glob")
		case len(fields) == 2:
			return bad("@bind requires a glob")
		case len(fields) > 3:
			return bad("unexpected trailing tokens after the glob; one glob per line, repeat @bind to add another")
		}
		return &Directive{
			Kind:   KindBind,
			Node:   fields[1],
			Glob:   fields[2],
			Source: pos,
			Raw:    trimmed,
		}, nil

	case KindExternal, KindInfra:
		switch {
		case len(fields) == 1:
			return bad(fmt.Sprintf("%s requires a node ID", kind))
		case len(fields) > 2:
			return bad("unexpected trailing tokens after the node ID")
		}
		return &Directive{Kind: kind, Node: fields[1], Source: pos, Raw: trimmed}, nil

	case KindIgnore:
		if len(fields) == 1 {
			return bad(`@ignore requires a node ID and a quoted reason`)
		}
		node := fields[1]
		// Everything after the node token, verbatim: the reason is a quoted
		// string and may contain spaces, so strings.Fields cannot carve it.
		rest := strings.TrimSpace(body[len(fields[0]):])
		rest = strings.TrimSpace(rest[len(node):])
		reason, detail := parseReason(rest)
		if detail != "" {
			return bad(detail)
		}
		return &Directive{Kind: KindIgnore, Node: node, Reason: reason, Source: pos, Raw: trimmed}, nil

	default:
		return nil, &SyntaxError{
			Source: pos,
			Raw:    trimmed,
			Detail: fmt.Sprintf("unknown directive %q", fields[0]),
		}
	}
}

// parseReason validates the quoted reason string of an @ignore directive. It
// returns either the reason or a detail describing what is wrong with it.
func parseReason(rest string) (reason, detail string) {
	switch {
	case rest == "":
		return "", "@ignore requires a quoted reason; an unexplained suppression is how a check dies quietly"
	case !strings.HasPrefix(rest, `"`):
		return "", "@ignore reason must be a quoted string"
	}
	end := strings.LastIndex(rest, `"`)
	if end == 0 {
		return "", "@ignore reason string is unterminated"
	}
	if tail := strings.TrimSpace(rest[end+1:]); tail != "" {
		return "", "unexpected trailing tokens after the reason string"
	}
	reason = rest[1:end]
	if strings.TrimSpace(reason) == "" {
		return "", "@ignore reason must not be empty"
	}
	return reason, ""
}
