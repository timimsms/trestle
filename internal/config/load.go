package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// file mirrors the on-disk shape of `.trestle.yml`. It is decoded loosely —
// `severity` lands in an `any` map so that a bad value produces a message about
// severities rather than a Go type name.
type file struct {
	Version  *int           `yaml:"version"`
	Diagrams []string       `yaml:"diagrams"`
	Discover []string       `yaml:"discover"`
	Shared   []string       `yaml:"shared"`
	Exclude  []string       `yaml:"exclude"`
	Severity map[string]any `yaml:"severity"`
	Render   *renderFile    `yaml:"render"`
}

type renderFile struct {
	Out    string `yaml:"out"`
	Layout string `yaml:"layout"`
	Theme  int    `yaml:"theme"`
}

var topLevelKeys = []string{"version", "diagrams", "discover", "shared", "exclude", "severity", "render"}

var renderKeys = []string{"out", "layout", "theme"}

// Find walks up from startDir looking for `.trestle.yml` and returns its
// absolute path. The directory containing it is the root.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", &Error{Msg: fmt.Sprintf("resolve %s: %v", startDir, err)}
	}
	for {
		candidate := filepath.Join(dir, Filename)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", &Error{
				Msg:  fmt.Sprintf("no %s found in %s or any parent directory", Filename, startDir),
				Hint: "run `trestle init` in the repo root to create one",
			}
		}
		dir = parent
	}
}

// LoadFrom finds `.trestle.yml` by walking up from startDir and loads it. This
// is the entry point the CLI uses.
func LoadFrom(startDir string) (*Config, error) {
	path, err := Find(startDir)
	if err != nil {
		return nil, err
	}
	return Load(path)
}

// Load reads and validates a specific config file. The root is the directory
// containing it. This is the only filesystem access this package performs.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, &Error{Path: path, Msg: fmt.Sprintf("resolve path: %v", err)}
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		return nil, &Error{Path: abs, Msg: fmt.Sprintf("read: %v", err)}
	}
	return Parse(abs, src)
}

// Parse validates config bytes that have already been read. path is used for
// error messages and to derive the root; nothing is read from disk.
func Parse(path string, src []byte) (*Config, error) {
	abs := path
	if a, err := filepath.Abs(path); err == nil {
		abs = a
	}

	astFile, perr := parser.ParseBytes(src, 0)
	if perr != nil {
		return nil, yamlError(abs, perr, "invalid YAML")
	}
	if len(astFile.Docs) > 1 {
		return nil, &Error{
			Path: abs,
			Line: docLine(astFile.Docs[1]),
			Msg:  "expected a single YAML document",
			Hint: "remove the `---` separator; Trestle reads one config per file",
		}
	}
	loc := newLocator(astFile)

	var f file
	if err := yaml.Unmarshal(src, &f); err != nil {
		return nil, yamlError(abs, err, "invalid config")
	}

	var errs ErrorList

	// Unknown keys are checked against the AST rather than via a strict decode
	// so that every unknown key is reported, not just the first.
	for _, kv := range loc.entries() {
		if !contains(topLevelKeys, kv.key) {
			errs = append(errs, &Error{
				Path: abs, Line: kv.line, Key: kv.key,
				Msg:  "unknown top-level key",
				Hint: "valid keys: " + strings.Join(topLevelKeys, ", "),
			})
		}
	}
	for _, kv := range loc.entriesUnder("render") {
		if !contains(renderKeys, kv.key) {
			errs = append(errs, &Error{
				Path: abs, Line: kv.line, Key: "render." + kv.key,
				Msg:  "unknown key",
				Hint: "valid render keys: " + strings.Join(renderKeys, ", "),
			})
		}
	}

	cfg := &Config{
		Version:  Version,
		Diagrams: f.Diagrams,
		Discover: f.Discover,
		Shared:   f.Shared,
		Exclude:  f.Exclude,
		Severity: DefaultSeverity(),
		Root:     filepath.Dir(abs),
		Path:     abs,
	}
	if f.Render != nil {
		cfg.Render = Render{Out: f.Render.Out, Layout: f.Render.Layout, Theme: f.Render.Theme}
	}
	if f.Exclude == nil {
		cfg.Exclude = DefaultExclude()
	}

	errs = append(errs, validateVersion(abs, loc, f)...)
	errs = append(errs, validateDiagrams(abs, loc, f)...)
	errs = append(errs, validateSeverity(abs, loc, f, cfg)...)
	errs = append(errs, validateShared(abs, loc, f)...)
	sortErrors(errs)

	if err := errs.err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateVersion(path string, loc *locator, f file) ErrorList {
	if f.Version == nil {
		return ErrorList{{
			Path: path, Key: "version",
			Msg:  "missing; every config must declare `version: 1`",
			Hint: "add `version: 1` as the first line",
		}}
	}
	if *f.Version != Version {
		return ErrorList{{
			Path: path, Line: loc.line("version"), Key: "version",
			Msg:  fmt.Sprintf("unsupported version %d; this build understands version %d", *f.Version, Version),
			Hint: fmt.Sprintf("set `version: %d`", Version),
		}}
	}
	return nil
}

// validateDiagrams rejects a config with nothing to check. This is beyond the
// four validation classes the phase file names, and it is deliberate: a config
// with no `diagrams:` makes `trestle check` a silent no-op that exits 0, which
// is the failure mode this tool exists to prevent.
func validateDiagrams(path string, loc *locator, f file) ErrorList {
	if len(f.Diagrams) > 0 {
		return nil
	}
	e := &Error{
		Path: path, Key: "diagrams",
		Msg:  "no diagrams configured; `trestle check` would have nothing to check",
		Hint: "add e.g. `diagrams: [docs/architecture/*.d2]`",
	}
	if f.Diagrams != nil { // present but empty
		e.Line = loc.line("diagrams")
	}
	return ErrorList{e}
}

func validateSeverity(path string, loc *locator, f file, cfg *Config) ErrorList {
	var errs ErrorList
	for code, raw := range f.Severity {
		line := loc.line("severity", code)
		if !contains(Codes, code) {
			errs = append(errs, &Error{
				Path: path, Line: line, Key: "severity." + code,
				Msg:  "unknown violation code",
				Hint: "valid codes: " + strings.Join(Codes, ", "),
			})
			continue
		}
		s, ok := raw.(string)
		if !ok || !Severity(s).Valid() {
			errs = append(errs, &Error{
				Path: path, Line: line, Key: "severity." + code,
				Msg:  fmt.Sprintf("invalid severity %v", raw),
				Hint: "valid severities: " + joinSeverities(),
			})
			continue
		}
		cfg.Severity[code] = Severity(s)
	}
	// Deterministic messages: map iteration order is not.
	sortErrors(errs)
	return errs
}

// validateShared enforces L11. A blanket entry is the one thing that can make
// `shared:` unsafe: `lib/**` would swallow a future `lib/dispatch_engine/` —
// real architectural weight, silently exempted.
func validateShared(path string, loc *locator, f file) ErrorList {
	var errs ErrorList
	for i, entry := range f.Shared {
		prefix, ok := blanketPrefix(entry)
		if !ok {
			continue
		}
		scope := "the repo root"
		example := "lib/http_client/**, lib/logging/**"
		if prefix != "" {
			scope = prefix + "/"
			example = prefix + "/http_client/**, " + prefix + "/logging/**"
		}
		errs = append(errs, &Error{
			Path: path, Line: loc.seqLine("shared", i), Key: fmt.Sprintf("shared[%d]", i),
			Msg:  fmt.Sprintf("blanket entry %q exempts everything under %s, including subsystems that do not exist yet", entry, scope),
			Hint: fmt.Sprintf("enumerate the shared subsystems instead: %s (see L11; `exclude:` is the blanket-capable list, and it is a blindspot by design)", example),
		})
	}
	return errs
}

// blanketPrefix reports whether a shared entry is a bare directory wildcard —
// at most one literal path segment followed by nothing but `*`/`**` — and
// returns that literal segment, if any.
func blanketPrefix(pattern string) (prefix string, blanket bool) {
	p := strings.TrimSuffix(strings.TrimSpace(pattern), "/")
	if p == "" || p == "." {
		return "", true
	}
	segs := strings.Split(p, "/")
	literal := 0
	for _, s := range segs {
		if hasGlobMeta(s) {
			break
		}
		literal++
	}
	switch {
	case literal >= 2:
		// `lib/http_client/**` — names a real subsystem.
		return "", false
	case literal == len(segs):
		// No wildcard at all — enumerated by construction.
		return "", false
	}
	for _, s := range segs[literal:] {
		if s != "*" && s != "**" {
			// `app/*/middleware/**` still names a specific leaf.
			return "", false
		}
	}
	if literal == 1 {
		return segs[0], true
	}
	return "", true
}

func hasGlobMeta(s string) bool { return strings.ContainsAny(s, "*?[{") }

// yamlError converts a goccy error into a config Error, keeping the line number
// goccy went to the trouble of tracking.
func yamlError(path string, err error, context string) *Error {
	var yerr yaml.Error
	if errors.As(err, &yerr) {
		line := 0
		if tk := yerr.GetToken(); tk != nil && tk.Position != nil {
			line = tk.Position.Line
		}
		return &Error{Path: path, Line: line, Msg: context + ": " + yerr.GetMessage()}
	}
	return &Error{Path: path, Msg: context + ": " + err.Error()}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func joinSeverities() string {
	out := make([]string, 0, len(Severities))
	for _, s := range Severities {
		out = append(out, string(s))
	}
	return strings.Join(out, ", ")
}

// sortErrors orders by line then key, so a config with several problems reports
// them in file order and the message is stable across runs — map iteration
// order is not.
func sortErrors(errs ErrorList) {
	sort.SliceStable(errs, func(i, j int) bool {
		if errs[i].Line != errs[j].Line {
			return errs[i].Line < errs[j].Line
		}
		return errs[i].Key < errs[j].Key
	})
}

// --- AST position lookup ------------------------------------------------
//
// goccy keeps token positions, which is the reason it was chosen over the
// alternatives: a config error that cannot say which line it is about makes the
// user diff their file against the docs by eye.

type locator struct{ body ast.Node }

type keyLine struct {
	key  string
	line int
}

func newLocator(f *ast.File) *locator {
	if len(f.Docs) == 0 || f.Docs[0].Body == nil {
		return &locator{}
	}
	return &locator{body: f.Docs[0].Body}
}

func mapEntries(n ast.Node) []*ast.MappingValueNode {
	switch v := n.(type) {
	case *ast.MappingNode:
		return v.Values
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{v}
	default:
		return nil
	}
}

func nodeLine(n ast.Node) int {
	if n == nil {
		return 0
	}
	if tk := n.GetToken(); tk != nil && tk.Position != nil {
		return tk.Position.Line
	}
	return 0
}

func docLine(d *ast.DocumentNode) int {
	if d == nil {
		return 0
	}
	return nodeLine(d.Body)
}

func keyOf(mv *ast.MappingValueNode) string {
	if mv == nil || mv.Key == nil {
		return ""
	}
	if tk := mv.Key.GetToken(); tk != nil {
		return tk.Value
	}
	return ""
}

// entries lists the top-level keys with their line numbers, in file order.
func (l *locator) entries() []keyLine {
	var out []keyLine
	for _, mv := range mapEntries(l.body) {
		out = append(out, keyLine{key: keyOf(mv), line: nodeLine(mv.Key)})
	}
	return out
}

// entriesUnder lists the keys of a nested mapping.
func (l *locator) entriesUnder(key string) []keyLine {
	node := l.node(key)
	if node == nil {
		return nil
	}
	var out []keyLine
	for _, mv := range mapEntries(node) {
		out = append(out, keyLine{key: keyOf(mv), line: nodeLine(mv.Key)})
	}
	return out
}

// node returns the value node at a dotted key path.
func (l *locator) node(keys ...string) ast.Node {
	cur := l.body
	for _, k := range keys {
		found := ast.Node(nil)
		for _, mv := range mapEntries(cur) {
			if keyOf(mv) == k {
				found = mv.Value
				break
			}
		}
		if found == nil {
			return nil
		}
		cur = found
	}
	return cur
}

// line returns the line of the key at a dotted path, or 0 if it is absent.
func (l *locator) line(keys ...string) int {
	cur := l.body
	for i, k := range keys {
		var match *ast.MappingValueNode
		for _, mv := range mapEntries(cur) {
			if keyOf(mv) == k {
				match = mv
				break
			}
		}
		if match == nil {
			return 0
		}
		if i == len(keys)-1 {
			return nodeLine(match.Key)
		}
		cur = match.Value
	}
	return 0
}

// seqLine returns the line of the i-th element of a sequence-valued key.
func (l *locator) seqLine(key string, idx int) int {
	seq, ok := l.node(key).(*ast.SequenceNode)
	if !ok || idx < 0 || idx >= len(seq.Values) {
		return l.line(key)
	}
	return nodeLine(seq.Values[idx])
}
