package config

import (
	"strconv"
	"strings"
)

// Error is a single user-facing config problem.
//
// It carries the file, the line and the offending key so the message can point
// at the thing that is wrong. A config error is a tool error — exit code 2 —
// and is never a violation.
type Error struct {
	Path string // config file path
	Line int    // 1-based; 0 when the problem has no line (e.g. an absent key)
	Key  string // offending key, e.g. "severity.UNBOUND" or "shared[0]"
	Msg  string
	Hint string // a runnable or copy-pasteable next step, where one exists
}

// Error implements error.
func (e *Error) Error() string {
	var b strings.Builder
	if e.Path != "" {
		b.WriteString(e.Path)
		if e.Line > 0 {
			b.WriteString(":")
			b.WriteString(strconv.Itoa(e.Line))
		}
		b.WriteString(": ")
	}
	if e.Key != "" {
		b.WriteString(e.Key)
		b.WriteString(": ")
	}
	b.WriteString(e.Msg)
	if e.Hint != "" {
		b.WriteString("\n  hint: ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

// ErrorList is every problem found in one config file. Validation does not stop
// at the first error: fixing a config one round-trip at a time is how people
// end up deleting it.
type ErrorList []*Error

// Error implements error, one problem per block.
func (l ErrorList) Error() string {
	msgs := make([]string, 0, len(l))
	for _, e := range l {
		msgs = append(msgs, e.Error())
	}
	return strings.Join(msgs, "\n")
}

// err returns the list as an error, or nil when it is empty.
func (l ErrorList) err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}
