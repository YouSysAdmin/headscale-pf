package policy

import (
	"os"
	"strings"

	"github.com/tailscale/hujson"
)

// Policy wraps a parsed Headscale policy template. The tool's only job is to
// fill in the members of the "groups" section; everything else — comments,
// key order, formatting, and every other section — is preserved verbatim by
// mutating the HuJSON AST in place and packing it back out. Validating the
// rest of the policy is the user's and the server's responsibility.
//
// Output is HuJSON (it may contain comments). Headscale's policy loader reads
// HuJSON, so the result is ready to transfer to the server as-is.
type Policy struct {
	// ast is the parsed template. Comments live in each node's BeforeExtra/
	// AfterExtra, so packing it reproduces the template byte-for-byte except
	// for the group values we replace.
	ast hujson.Value

	// staged holds the group members to write, keyed by the full group name
	// (e.g. "group:ops"). AppendGroups fills it; WritePolicyToFile applies it.
	staged map[string][]string
}

// schemaKey is an editor-only top-level field (a JSON Schema reference for IDE
// validation of the HJSON template). Headscale doesn't recognize it, so it is
// dropped on read and never reaches the output.
const schemaKey = "$schema"

// ReadPolicyFromFile parses a Headscale policy template from disk, keeping
// comments and formatting intact, and drops the editor-only $schema field.
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
	p.dropSchema()
	return nil
}

// dropSchema removes the top-level $schema member from the root object so it
// never appears in the packed output.
func (p *Policy) dropSchema() {
	root, ok := p.ast.Value.(*hujson.Object)
	if !ok {
		return
	}
	filtered := root.Members[:0]
	for _, m := range root.Members {
		if lit, ok := m.Name.Value.(hujson.Literal); ok && lit.String() == schemaKey {
			continue
		}
		filtered = append(filtered, m)
	}
	root.Members = filtered
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
// only those groups' value arrays — and writes the packed HuJSON to disk.
// Groups not staged (e.g. not found in the source) keep their template value,
// comments, and formatting untouched.
func (p *Policy) WritePolicyToFile(path string) error {
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

	data := p.ast.Pack()

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
