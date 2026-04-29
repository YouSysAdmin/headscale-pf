package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
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

func TestReadPolicyFromFile_ParsesHJSON(t *testing.T) {
	p := Policy{}
	if err := p.ReadPolicyFromFile(writeTemp(t, "in.hjson", sampleHJSON)); err != nil {
		t.Fatalf("ReadPolicyFromFile: %v", err)
	}
	if _, ok := p.Groups["group:admins"]; !ok {
		t.Errorf("expected group:admins in parsed Groups, got %#v", p.Groups)
	}
	if got := len(p.ACLs); got != 1 {
		t.Errorf("expected 1 ACL, got %d", got)
	}
}

func TestGetGroupNames_StripsGroupPrefix(t *testing.T) {
	p := Policy{Groups: map[string][]string{
		"group:admins": nil,
		"group:devs":   nil,
		"malformed":    nil, // no prefix → must be skipped
	}}
	got := p.GetGroupNames()
	sort.Strings(got)
	want := []string{"admins", "devs"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("GetGroupNames = %v, want %v", got, want)
	}
}

func TestAppendGroups_OverwritesExisting(t *testing.T) {
	p := Policy{Groups: map[string][]string{"group:admins": {"old@"}}}
	p.AppendGroups(map[string][]string{
		"group:admins": {"new@"},
		"group:devs":   {"alice@"},
	})
	if got := p.Groups["group:admins"]; len(got) != 1 || got[0] != "new@" {
		t.Errorf("group:admins = %v, want [new@]", got)
	}
	if _, ok := p.Groups["group:devs"]; !ok {
		t.Errorf("group:devs should be added")
	}
}

func TestAppendGroups_InitializesNilMap(t *testing.T) {
	p := Policy{}
	p.AppendGroups(map[string][]string{"group:devs": {"alice@"}})
	if _, ok := p.Groups["group:devs"]; !ok {
		t.Errorf("AppendGroups must allocate Groups when nil")
	}
}

func TestSanitize_NilGroupsBecomeEmptySlice(t *testing.T) {
	p := Policy{Groups: map[string][]string{"group:empty": nil}}
	p.sanitize()
	if got := p.Groups["group:empty"]; got == nil {
		t.Errorf("sanitize must replace nil with empty slice")
	}
	if p.SSHs == nil {
		t.Errorf("sanitize must initialize nil SSHs to empty slice")
	}
}

func TestWritePolicyToFile_NilGroupSerializesAsEmptyArray(t *testing.T) {
	p := Policy{Groups: map[string][]string{"group:empty": nil}}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("WritePolicyToFile: %v", err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	groups, ok := decoded["groups"].(map[string]any)
	if !ok {
		t.Fatalf("groups missing or wrong type in %s", raw)
	}
	if _, isArray := groups["group:empty"].([]any); !isArray {
		t.Errorf("group:empty should serialize as [], got %#v", groups["group:empty"])
	}
}

// TestReadRealPolicyHJSON_ParsesAllSections loads the real policy.hjson
// shipped at the repo root. It catches regressions in HJSON dialect
// support (comments, trailing commas) and in the Policy struct shape
// drifting away from the production template format.
func TestReadRealPolicyHJSON_ParsesAllSections(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	src := filepath.Join(repoRoot, "policy.hjson")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("policy.hjson not available: %v", err)
	}

	p := Policy{}
	if err := p.ReadPolicyFromFile(src); err != nil {
		t.Fatalf("real policy.hjson failed to parse: %v", err)
	}
	if len(p.Groups) < 30 {
		t.Errorf("expected real policy to define many groups, got %d", len(p.Groups))
	}
	if len(p.Hosts) < 30 {
		t.Errorf("expected real policy to define many hosts, got %d", len(p.Hosts))
	}
	if len(p.TagOwners) == 0 {
		t.Errorf("expected real policy to define tagOwners")
	}
	if len(p.ACLs) == 0 {
		t.Errorf("expected real policy to define acls")
	}
	if len(p.AutoApprovers.Routes) == 0 || len(p.AutoApprovers.ExitNode) == 0 {
		t.Errorf("expected real policy to define autoApprovers")
	}

	// Round-trip: write the real policy back out and confirm everything
	// non-group survives. This is the production guarantee.
	out := filepath.Join(t.TempDir(), "out.json")
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write real policy: %v", err)
	}
	raw, _ := os.ReadFile(out)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	// $schema is in the real template — must round-trip per the tool's
	// "fill groups, copy everything else" contract.
	if _, ok := decoded["$schema"]; !ok {
		t.Errorf("real policy.hjson round-trip dropped $schema; output: %s", raw[:min(200, len(raw))])
	}
}

// TestHostsCaseInsensitive_EmitsLowercase confirms that "Hosts" (capital H,
// as used in policy.hjson) loads into the Hosts field, and that on emit the
// JSON tag forces lowercase "hosts" — matching what current.json shows.
func TestHostsCaseInsensitive_EmitsLowercase(t *testing.T) {
	in := writeTemp(t, "in.hjson", `{
  "Hosts": { "vpc-a": "10.0.0.0/16" }
}`)
	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := p.Hosts["vpc-a"].String(); got != "10.0.0.0/16" {
		t.Fatalf("Hosts[vpc-a] = %q, want 10.0.0.0/16", got)
	}

	out := filepath.Join(t.TempDir(), "out.json")
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, _ := os.ReadFile(out)
	if !strings.Contains(string(raw), `"hosts":`) {
		t.Errorf("output should emit lowercase hosts, got: %s", raw)
	}
	if strings.Contains(string(raw), `"Hosts":`) {
		t.Errorf("output must not preserve capital Hosts, got: %s", raw)
	}
}

// TestRoundTrip_PreservesHostsTagOwnersAutoApprovers verifies the structural
// fields beyond Groups survive a load/save cycle. Mirrors the shape of
// current.json so future struct changes can't silently drop sections.
func TestRoundTrip_PreservesHostsTagOwnersAutoApprovers(t *testing.T) {
	in := writeTemp(t, "in.hjson", `{
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
    {
      "action": "accept",
      "proto":  "tcp",
      "src":    ["group:ops"],
      "dst":    ["tag:bastion:22"],
    },
  ],
}`)
	out := filepath.Join(t.TempDir(), "out.json")

	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write: %v", err)
	}

	q := Policy{}
	if err := q.ReadPolicyFromFile(out); err != nil {
		t.Fatalf("re-read: %v", err)
	}

	if got := q.Hosts["app-ca-prod-vpc"].String(); got != "10.2.0.0/16" {
		t.Errorf("netip.Prefix lost on round-trip: %q", got)
	}
	if got, want := q.TagOwners["tag:prod-vpn"], []string{"group:ops"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("tagOwners lost: %v", got)
	}
	if got := q.AutoApprovers.ExitNode; len(got) != 1 || got[0] != "group:ops" {
		t.Errorf("autoApprovers.exitNode lost: %v", got)
	}
	if got := q.AutoApprovers.Routes["10.0.0.0/8"]; len(got) != 1 || got[0] != "group:ops" {
		t.Errorf("autoApprovers.routes lost: %v", got)
	}
	if len(q.ACLs) != 1 {
		t.Fatalf("ACLs lost: %v", q.ACLs)
	}
	got := q.ACLs[0]
	if got.Action != "accept" || got.Protocol != "tcp" {
		t.Errorf("ACL fields lost: %+v", got)
	}
	if len(got.Sources) != 1 || got.Sources[0] != "group:ops" {
		t.Errorf("ACL src lost: %v", got.Sources)
	}
	if len(got.Destinations) != 1 || got.Destinations[0] != "tag:bastion:22" {
		t.Errorf("ACL dst lost: %v", got.Destinations)
	}
}

// TestStaticGroupPreservedWhenSourceMisses guards the contract behind
// group:ops in current.json: groups not provided to AppendGroups keep
// whatever the HJSON template specified.
func TestStaticGroupPreservedWhenSourceMisses(t *testing.T) {
	in := writeTemp(t, "in.hjson", `{
  "groups": {
    "group:ops":         ["ops@"],
    "group:network-all": ["alice@"],
  }
}`)
	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}

	// Simulate prepare.go: only network-all came back from the source,
	// ops was not found and is therefore not in the AppendGroups input.
	p.AppendGroups(map[string][]string{
		"group:network-all": {"alice@", "bob@"},
	})

	if got := p.Groups["group:ops"]; len(got) != 1 || got[0] != "ops@" {
		t.Errorf("static group:ops dropped when source missed it: %v", got)
	}
	if got := p.Groups["group:network-all"]; len(got) != 2 {
		t.Errorf("dynamic group:network-all not overwritten: %v", got)
	}
}

// TestSchemaFieldPreserved enforces the tool's contract: only group user
// lists are mutated. The $schema reference must round-trip from input to
// output unchanged.
func TestSchemaFieldPreserved(t *testing.T) {
	in := writeTemp(t, "in.hjson", `{
  "$schema": "./schemas/tailscale-acl.json-schema.json",
  "groups": { "group:ops": ["ops@"] }
}`)
	out := filepath.Join(t.TempDir(), "out.json")

	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, _ := os.ReadFile(out)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	got, ok := decoded["$schema"].(string)
	if !ok {
		t.Fatalf("$schema missing from output: %s", raw)
	}
	if want := "./schemas/tailscale-acl.json-schema.json"; got != want {
		t.Errorf("$schema mutated: got %q want %q", got, want)
	}
}

// TestUnknownTopLevelFieldsPreserved guards future Headscale policy fields
// not yet modeled in the Policy struct: they must still pass through.
// "all other policy [is] copied to result as-is" — only group user lists
// are tool-managed.
func TestUnknownTopLevelFieldsPreserved(t *testing.T) {
	in := writeTemp(t, "in.hjson", `{
  "$schema":             "x",
  "randomizeClientPort": true,
  "nodeAttrs": [
    { "target": ["*"], "attr": ["funnel"] },
  ],
  "groups": { "group:ops": ["ops@"] }
}`)
	out := filepath.Join(t.TempDir(), "out.json")

	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	// Tool would mutate groups here; round-trip should keep the rest intact.
	p.AppendGroups(map[string][]string{"group:ops": {"alice@"}})
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, _ := os.ReadFile(out)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["$schema"] != "x" {
		t.Errorf("$schema lost: %v", decoded["$schema"])
	}
	if decoded["randomizeClientPort"] != true {
		t.Errorf("randomizeClientPort lost: %v", decoded["randomizeClientPort"])
	}
	if _, ok := decoded["nodeAttrs"]; !ok {
		t.Errorf("nodeAttrs lost from %s", raw)
	}
	if got := decoded["groups"].(map[string]any)["group:ops"].([]any); len(got) != 1 || got[0] != "alice@" {
		t.Errorf("group:ops should reflect AppendGroups, got %v", got)
	}
}

func TestRoundTrip_HJSONInJSONOut(t *testing.T) {
	in := writeTemp(t, "in.hjson", sampleHJSON)
	out := filepath.Join(t.TempDir(), "out.json")

	p := Policy{}
	if err := p.ReadPolicyFromFile(in); err != nil {
		t.Fatalf("read: %v", err)
	}
	p.AppendGroups(map[string][]string{"group:admins": {"alice@example.com"}})
	if err := p.WritePolicyToFile(out); err != nil {
		t.Fatalf("write: %v", err)
	}

	q := Policy{}
	if err := q.ReadPolicyFromFile(out); err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if got := q.Groups["group:admins"]; len(got) != 1 || got[0] != "alice@example.com" {
		t.Errorf("round-trip lost AppendGroups data: %v", got)
	}
	if len(q.ACLs) != 1 {
		t.Errorf("round-trip lost ACLs")
	}
}
