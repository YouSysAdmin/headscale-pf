package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
