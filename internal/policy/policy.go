package policy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tailscale/hujson"
)

// Supported output formats. FormatAuto picks hjson or json by inspecting the
// input template (see ResolveFormat).
const (
	FormatHJSON = "hjson"
	FormatJSON  = "json"
	FormatAuto  = "auto"
)

// IsValidFormat reports whether format is an accepted --output-format value
// (auto, hjson, or json). The concrete write path accepts only hjson/json;
// auto is resolved to one of those by ResolveFormat.
func IsValidFormat(format string) bool {
	return format == FormatAuto || format == FormatHJSON || format == FormatJSON
}

// Policy wraps a parsed Headscale policy template. The tool's only job is to
// fill in the members of the "groups" section; everything else — comments,
// key order, formatting, and every other section — is preserved verbatim by
// mutating the HuJSON AST in place and packing it back out. Validating the
// rest of the policy is the user's and the server's responsibility.
//
// Output is either HuJSON (the default — comments and formatting preserved
// byte-for-byte) or plain RFC-8259 JSON, selected per WritePolicyToFile call.
// Headscale's policy loader reads HuJSON, so either result transfers as-is.
type Policy struct {
	// ast is the parsed template. Comments live in each node's BeforeExtra/
	// AfterExtra, so packing it reproduces the template byte-for-byte except
	// for the group values we replace.
	ast hujson.Value

	// staged holds the group members to write, keyed by the full group name
	// (e.g. "group:ops"). AppendGroups fills it; WritePolicyToFile applies it.
	staged map[string][]string

	// inputStrictJSON records whether the raw template parsed as strict
	// RFC-8259 JSON (no comments or trailing commas). FormatAuto uses it to
	// decide the output format.
	inputStrictJSON bool
}

// ReadPolicyFromFile parses a Headscale policy template from disk, keeping
// comments and formatting intact.
func (p *Policy) ReadPolicyFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ast, err := hujson.Parse(data)
	if err != nil {
		return err
	}
	p.ast = ast
	// json.Valid is the precise hjson-vs-json discriminator: it rejects the
	// comments and trailing commas that only HuJSON allows.
	p.inputStrictJSON = json.Valid(data)
	return nil
}

// ResolveFormat maps FormatAuto to a concrete format based on the input
// template: strict JSON in → json out, otherwise hjson. hjson and json (and
// any other value) are returned unchanged.
func (p *Policy) ResolveFormat(format string) string {
	if format != FormatAuto {
		return format
	}
	if p.inputStrictJSON {
		return FormatJSON
	}
	return FormatHJSON
}

// groupsObject returns the root object's "groups" member value (matched
// case-insensitively, so "groups" or "Groups"), or nil if absent.
func (p *Policy) groupsObject() *hujson.Object {
	root, ok := p.ast.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	for i := range root.Members {
		lit, ok := root.Members[i].Name.Value.(hujson.Literal)
		if !ok {
			continue
		}
		if strings.EqualFold(lit.String(), "groups") {
			obj, _ := root.Members[i].Value.Value.(*hujson.Object)
			return obj
		}
	}
	return nil
}

// GetGroupNames returns the bare group names declared in the template (the
// "group:" prefix stripped), used to query the identity source. Names without
// a prefix are skipped.
func (p *Policy) GetGroupNames() []string {
	obj := p.groupsObject()
	if obj == nil {
		return nil
	}
	var groups []string
	for i := range obj.Members {
		lit, ok := obj.Members[i].Name.Value.(hujson.Literal)
		if !ok {
			continue
		}
		parts := strings.Split(lit.String(), ":")
		if len(parts) >= 2 {
			groups = append(groups, parts[1])
		}
	}
	return groups
}

// AppendGroups stages group members to be written. Keys are full group names
// (e.g. "group:ops"), matching the template's group keys.
func (p *Policy) AppendGroups(groups map[string][]string) {
	if p.staged == nil {
		p.staged = make(map[string][]string)
	}
	for g, u := range groups {
		p.staged[g] = u
	}
}

// WritePolicyToFile applies the staged group members to the AST — replacing
// only those groups' value arrays — and writes the result to disk in the given
// format (FormatHJSON, FormatJSON, or FormatAuto). Groups not staged (e.g. not
// found in the source) keep their template value, comments, and formatting
// untouched.
func (p *Policy) WritePolicyToFile(path, format string) error {
	format = p.ResolveFormat(format)
	if format != FormatHJSON && format != FormatJSON {
		return fmt.Errorf("invalid output format %q: must be %q, %q, or %q", format, FormatAuto, FormatHJSON, FormatJSON)
	}

	if obj := p.groupsObject(); obj != nil && len(p.staged) > 0 {
		for i := range obj.Members {
			lit, ok := obj.Members[i].Name.Value.(hujson.Literal)
			if !ok {
				continue
			}
			members, staged := p.staged[lit.String()]
			if !staged {
				continue
			}
			// Replace only the value; the node's BeforeExtra/AfterExtra
			// (surrounding spacing and inline comments) are left intact.
			obj.Members[i].Value.Value = buildArray(members)
		}
	}

	data, err := p.serialize(format)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

// serialize renders the already-mutated AST in the requested format.
//   - FormatHJSON: byte-for-byte HuJSON (comments, trailing commas, $schema kept).
//   - FormatJSON:  RFC-8259 JSON, pretty-printed with 2-space indent, key order
//     preserved, $schema kept, comments and trailing commas removed.
func (p *Policy) serialize(format string) ([]byte, error) {
	if format == FormatHJSON {
		return p.ast.Pack(), nil
	}

	// Standardize rewrites the AST to valid JSON (comments and trailing commas
	// become whitespace); json.Indent then re-emits the token stream with a
	// clean 2-space indent, dropping that insignificant whitespace while
	// preserving object key order.
	p.ast.Standardize()
	var buf bytes.Buffer
	if err := json.Indent(&buf, p.ast.Pack(), "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n') // json.Indent does not add a trailing newline
	return buf.Bytes(), nil
}

// buildArray builds a HuJSON array of string literals. A nil or empty member
// list yields an empty array ([]) rather than null. Elements carry no extra
// whitespace, so they pack compact inline, e.g. ["alice@","bob@"].
func buildArray(members []string) *hujson.Array {
	elems := make([]hujson.Value, 0, len(members))
	for _, m := range members {
		elems = append(elems, hujson.Value{Value: hujson.String(m)})
	}
	return &hujson.Array{Elements: elems}
}
