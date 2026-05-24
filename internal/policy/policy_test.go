package policy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

const sampleHJSON = `{
  // Comments are valid HJSON
  "groups": {
    "group:admins": [],
    "group:devs": [],
  },
  "tagOwners": {
    "tag:server": ["group:admins"],
  },
  "acls": [
    {
      "action": "accept",
      "proto":  "tcp",
      "src":    ["group:admins"],
      "dst":    ["tag:server:22"],
    },
  ],
}`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return p
}

// standardize converts HuJSON output (which may contain comments and trailing
// commas) into plain JSON so it can be decoded for structural assertions. It
// parses a copy because hujson aliases the input buffer and Standardize mutates
// it in place.
func standardize(t *testing.T, raw []byte) []byte {
	t.Helper()
	v, err := hujson.Parse(append([]byte(nil), raw...))
	if err != nil {
		t.Fatalf("parse output as HuJSON: %v\n%s", err, raw)
	}
	v.Standardize()
	return v.Pack()
}

// orderedKeys returns the keys of a JSON object in document order.
func orderedKeys(t *testing.T, data []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		t.Fatalf("expected JSON object, got %v", tok)
	}
	var keys []string
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			t.Fatalf("key token: %v", err)
		}
		keys = append(keys, kt.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("skip value: %v", err)
		}
	}
	return keys
}

// readWrite reads template, applies staged groups, writes, returns raw output.
func readWrite(t *testing.T, template string, staged map[string][]string) []byte {
	t.Helper()
	in := writeTemp(t, "in.hjson", template)
	out := filepath.Join(t.TempDir(), "out.json")
	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	if staged != nil {
		p.AppendGroups(staged)
	}
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return raw
}

// TestUnmodifiedTemplateRoundTripsByteForByte is the core guarantee: a template
// the tool doesn't modify packs back out exactly as written — comments, trailing
// commas, indentation and all.
func TestUnmodifiedTemplateRoundTripsByteForByte(t *testing.T) {
	tmpl := `{
  // top comment
  "groups": {
    "group:ops": ["ops@"], // ops are static here
  },
  "acls": [
    // acl rule
    {"action": "accept", "src": ["group:ops"], "dst": ["tag:x:22"]},
  ],
}`
	// No AppendGroups → nothing should change.
	got := readWrite(t, tmpl, nil)
	if string(got) != tmpl {
		t.Errorf("unmodified template not byte-identical:\n--- want ---\n%s\n--- got ---\n%s", tmpl, got)
	}
}

// TestCommentsPreservedWhenGroupsFilled confirms comments survive even when the
// tool rewrites a group's members.
func TestCommentsPreservedWhenGroupsFilled(t *testing.T) {
	tmpl := `{
  // policy for prod
  "groups": {
    "group:ops": [], // filled from source
  },
  "hosts": { "h": "10.0.0.0/8" }, // network
}`
	got := readWrite(t, tmpl, map[string][]string{"group:ops": {"alice@", "bob@"}})
	s := string(got)
	for _, want := range []string{"// policy for prod", "// filled from source", "// network"} {
		if !strings.Contains(s, want) {
			t.Errorf("comment %q lost:\n%s", want, got)
		}
	}
	// The group must reflect the staged members.
	clean := standardize(t, got)
	var decoded struct {
		Groups map[string][]string `json:"groups"`
	}
	if err := json.Unmarshal(clean, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v := decoded.Groups["group:ops"]; len(v) != 2 || v[0] != "alice@" || v[1] != "bob@" {
		t.Errorf("group:ops = %v, want [alice@ bob@]", v)
	}
}

// TestStaticGroupPreservedWhenSourceMisses: a group not staged keeps its
// template members and its inline comment.
func TestStaticGroupPreservedWhenSourceMisses(t *testing.T) {
	tmpl := `{
  "groups": {
    "group:ops":         ["ops@"], // static, not in source
    "group:network-all": ["stale@"],
  }
}`
	// Only network-all comes back from the source.
	got := readWrite(t, tmpl, map[string][]string{"group:network-all": {"alice@", "bob@"}})
	if !strings.Contains(string(got), "// static, not in source") {
		t.Errorf("static group comment lost:\n%s", got)
	}
	clean := standardize(t, got)
	var decoded struct {
		Groups map[string][]string `json:"groups"`
	}
	if err := json.Unmarshal(clean, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v := decoded.Groups["group:ops"]; len(v) != 1 || v[0] != "ops@" {
		t.Errorf("static group:ops dropped: %v", v)
	}
	if v := decoded.Groups["group:network-all"]; len(v) != 2 || v[0] != "alice@" {
		t.Errorf("group:network-all not overwritten: %v", v)
	}
}

func TestReadPolicyFromFile_ParsesHJSON(t *testing.T) {
	in := writeTemp(t, "in.hjson", sampleHJSON)
	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("ReadPolicyFromFile: %v", err)
	}
	got := p.GetGroupNames()
	sort.Strings(got)
	want := []string{"admins", "devs"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GetGroupNames = %v, want %v", got, want)
	}
}

func TestGetGroupNames_StripsPrefixSkipsMalformed(t *testing.T) {
	in := writeTemp(t, "in.hjson", `{
  "groups": {
    "group:admins": [],
    "group:devs":   [],
    "malformed":    [],
  }
}`)
	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	got := p.GetGroupNames()
	sort.Strings(got)
	want := []string{"admins", "devs"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GetGroupNames = %v, want %v", got, want)
	}
}

func TestAppendGroups_OverwritesAndAddsStaged(t *testing.T) {
	tmpl := `{ "groups": { "group:admins": ["old@"], "group:devs": [] } }`
	got := readWrite(t, tmpl, map[string][]string{
		"group:admins": {"new@"},
		"group:devs":   {"alice@"},
	})
	clean := standardize(t, got)
	var decoded struct {
		Groups map[string][]string `json:"groups"`
	}
	if err := json.Unmarshal(clean, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v := decoded.Groups["group:admins"]; len(v) != 1 || v[0] != "new@" {
		t.Errorf("group:admins = %v, want [new@]", v)
	}
	if v := decoded.Groups["group:devs"]; len(v) != 1 || v[0] != "alice@" {
		t.Errorf("group:devs = %v, want [alice@]", v)
	}
}

func TestEmptyGroupSerializesAsArray(t *testing.T) {
	tmpl := `{ "groups": { "group:empty": ["placeholder@"] } }`
	// Stage an empty member list (group exists in source, no members).
	got := readWrite(t, tmpl, map[string][]string{"group:empty": {}})
	clean := standardize(t, got)
	var decoded map[string]any
	if err := json.Unmarshal(clean, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	groups := decoded["groups"].(map[string]any)
	if _, isArray := groups["group:empty"].([]any); !isArray {
		t.Errorf("group:empty should serialize as [], got %#v", groups["group:empty"])
	}
}

// TestTopLevelOrderPreserved: top-level sections keep template order.
func TestTopLevelOrderPreserved(t *testing.T) {
	// Deliberately non-alphabetical.
	tmpl := `{
  "tagOwners": { "tag:a": ["group:ops"] },
  "groups":    { "group:ops": ["ops@"] },
  "acls":      [],
  "hosts":     { "h": "10.0.0.0/8" },
}`
	got := readWrite(t, tmpl, map[string][]string{"group:ops": {"alice@"}})
	keys := orderedKeys(t, standardize(t, got))
	want := []string{"tagOwners", "groups", "acls", "hosts"}
	if len(keys) != len(want) {
		t.Fatalf("top-level order = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("top-level order = %v, want %v", keys, want)
			break
		}
	}
}

// TestGroupOrderPreserved: entries inside the groups object keep template order.
func TestGroupOrderPreserved(t *testing.T) {
	tmpl := `{
  "groups": {
    "group:zeta":  ["z@"],
    "group:alpha": ["a@"],
    "group:mid":   ["m@"],
  }
}`
	got := readWrite(t, tmpl, map[string][]string{"group:alpha": {"alice@"}})
	clean := standardize(t, got)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(clean, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := orderedKeys(t, top["groups"])
	want := []string{"group:zeta", "group:alpha", "group:mid"}
	if len(keys) != len(want) {
		t.Fatalf("group order = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("group order = %v, want %v (must not be sorted)", keys, want)
			break
		}
	}
}

// TestNestedFieldsPreserved: fields nested inside non-group sections survive,
// since the whole AST is passed through untouched.
func TestNestedFieldsPreserved(t *testing.T) {
	tmpl := `{
  "groups": { "group:ops": ["ops@"] },
  "ssh": [
    {
      "action":      "accept",
      "src":         ["group:ops"],
      "dst":         ["tag:server"],
      "users":       ["root"],
      "checkPeriod": "5m",
      "acceptEnv":   ["LANG", "LC_*"],
      "futureField": { "nested": true },
    },
  ],
}`
	got := readWrite(t, tmpl, map[string][]string{"group:ops": {"alice@"}})
	clean := standardize(t, got)
	var decoded map[string]any
	if err := json.Unmarshal(clean, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ssh, ok := decoded["ssh"].([]any)
	if !ok || len(ssh) != 1 {
		t.Fatalf("ssh lost: %s", clean)
	}
	rule := ssh[0].(map[string]any)
	if env, _ := rule["acceptEnv"].([]any); len(env) != 2 || env[0] != "LANG" || env[1] != "LC_*" {
		t.Errorf("ssh.acceptEnv lost: %v", rule["acceptEnv"])
	}
	if rule["checkPeriod"] != "5m" {
		t.Errorf("ssh.checkPeriod lost: %v", rule["checkPeriod"])
	}
	if ff, _ := rule["futureField"].(map[string]any); ff == nil || ff["nested"] != true {
		t.Errorf("unknown nested field lost: %v", rule["futureField"])
	}
}

// TestHostsCasePreserved: the template's key casing is kept verbatim.
func TestHostsCasePreserved(t *testing.T) {
	tmpl := `{
  "groups": { "group:ops": ["ops@"] },
  "Hosts":  { "vpc-a": "10.0.0.0/16" }
}`
	got := readWrite(t, tmpl, map[string][]string{"group:ops": {"alice@"}})
	if !strings.Contains(string(got), `"Hosts"`) {
		t.Errorf("capital Hosts not preserved verbatim:\n%s", got)
	}
}

func TestRoundTrip_PreservesHostsTagOwnersAutoApprovers(t *testing.T) {
	tmpl := `{
  "groups": { "group:ops": ["ops@"] },
  "Hosts": {
    "app-ca-prod-vpc": "10.2.0.0/16",
    "eks-ca-prod-vpc":       "10.18.0.0/16",
  },
  "tagOwners": {
    "tag:prod-vpn": ["group:ops"],
    "tag:bastion":  ["group:ops"],
  },
  "autoApprovers": {
    "routes":  { "10.0.0.0/8": ["group:ops"] },
    "exitNode": ["group:ops"],
  },
  "acls": [
    {"action": "accept", "proto": "tcp", "src": ["group:ops"], "dst": ["tag:bastion:22"]},
  ],
}`
	got := readWrite(t, tmpl, map[string][]string{"group:ops": {"alice@"}})
	clean := standardize(t, got)

	var q map[string]any
	if err := json.Unmarshal(clean, &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hosts := q["Hosts"].(map[string]any)
	if hosts["app-ca-prod-vpc"] != "10.2.0.0/16" {
		t.Errorf("hosts lost: %v", hosts)
	}
	tagOwners := q["tagOwners"].(map[string]any)
	if v, _ := tagOwners["tag:prod-vpn"].([]any); len(v) != 1 || v[0] != "group:ops" {
		t.Errorf("tagOwners lost: %v", tagOwners["tag:prod-vpn"])
	}
	aa := q["autoApprovers"].(map[string]any)
	if en, _ := aa["exitNode"].([]any); len(en) != 1 || en[0] != "group:ops" {
		t.Errorf("autoApprovers.exitNode lost: %v", aa["exitNode"])
	}
	routes := aa["routes"].(map[string]any)
	if r, _ := routes["10.0.0.0/8"].([]any); len(r) != 1 || r[0] != "group:ops" {
		t.Errorf("autoApprovers.routes lost: %v", routes["10.0.0.0/8"])
	}
	acls := q["acls"].([]any)
	if len(acls) != 1 {
		t.Fatalf("acls lost: %v", acls)
	}
	acl := acls[0].(map[string]any)
	if acl["action"] != "accept" || acl["proto"] != "tcp" {
		t.Errorf("acl fields lost: %+v", acl)
	}
	if dst, _ := acl["dst"].([]any); len(dst) != 1 || dst[0] != "tag:bastion:22" {
		t.Errorf("acl dst lost: %v", acl["dst"])
	}
}

// TestReadRealPolicyHJSON_ParsesAndRoundTrips loads the real policy.hjson at the
// repo root (if present) and confirms it parses, packs back to valid HuJSON,
// keeps many groups, and drops $schema.
func TestReadRealPolicyHJSON_ParsesAndRoundTrips(t *testing.T) {
	src := filepath.Join("..", "..", "policy.hjson")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("policy.hjson not available: %v", err)
	}
	p := Policy{}
	if err := p.ReadPolicyFromFile(src); err != nil {
		t.Fatalf("real policy.hjson failed to parse: %v", err)
	}
	if len(p.GetGroupNames()) < 30 {
		t.Errorf("expected real policy to define many groups, got %d", len(p.GetGroupNames()))
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write real policy: %v", err)
	}
	raw, _ := os.ReadFile(out)
	// Output must still be parseable HuJSON.
	if _, err := hujson.Parse(raw); err != nil {
		t.Errorf("real policy output is not valid HuJSON: %v", err)
	}
}
