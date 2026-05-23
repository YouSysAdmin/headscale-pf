package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
	"github.com/yousysadmin/headscale-pf/internal/models"
	"github.com/yousysadmin/headscale-pf/internal/sources"
)

// standardizeJSON strips comments/trailing commas from HuJSON output so it can
// be decoded with encoding/json for structural assertions. It parses a copy
// because hujson aliases the input buffer and Standardize mutates it in place.
func standardizeJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	v, err := hujson.Parse(append([]byte(nil), raw...))
	if err != nil {
		t.Fatalf("parse output as HuJSON: %v\n%s", err, raw)
	}
	v.Standardize()
	return v.Pack()
}

// stubSource implements sources.Source for end-to-end tests of preparePolicy.
// Each entry in groups maps a bare group name (without "group:" prefix) to
// the *models.Group GetGroupByName should return. A missing key returns nil
// (group not found in the source) — which exercises the static-group
// preservation path.
type stubSource struct {
	groups map[string]*models.Group
}

func (s *stubSource) GetGroupByName(name string) (*models.Group, error) {
	if g, ok := s.groups[name]; ok {
		return g, nil
	}
	return nil, nil
}

func (s *stubSource) GetGroupMembers(groupID string) ([]models.User, error) {
	for _, g := range s.groups {
		if g != nil && g.ID == groupID {
			return g.Users, nil
		}
	}
	return []models.User{}, nil
}

var _ sources.Source = (*stubSource)(nil)

// TestPreparePolicy_EndToEnd reproduces the full pipeline: HJSON template in,
// JSON policy out, with a stub Source. It locks in the contract that real
// users rely on:
//   - groups present in the source are overwritten with source data
//   - groups NOT present in the source keep their HJSON-defined members
//   - empty source results serialize as [] not null
//   - hosts / tagOwners / autoApprovers / acls round-trip verbatim
//   - sections absent from the template are not injected into the output
func TestPreparePolicy_EndToEnd(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "policy.hjson")
	out := filepath.Join(tmp, "current.json")

	template := `{
  "$schema": "./schemas/tailscale-acl.json-schema.json",
  "groups": {
    // ops is not in the source — must stay as-is
    "group:ops": ["ops@"],

    // network-all is in the source — must be overwritten
    "group:network-all": ["stale@"],

    // empty in template, empty in source — must emit []
    "group:network-eks-admin-prod": [],

    // empty in template, populated in source — overwritten
    "group:corp-vpn": [],
  },
  "Hosts": {
    "app-ca-prod-vpc": "10.2.0.0/16",
  },
  "tagOwners": {
    "tag:prod-vpn": ["group:ops"],
  },
  "autoApprovers": {
    "routes":   { "10.0.0.0/8": ["group:ops"] },
    "exitNode": ["group:ops"],
  },
  "acls": [
    {
      "action": "accept",
      "proto":  "",
      "src":    ["group:network-all"],
      "dst":    ["tag:prod-vpn:*"],
    },
  ],
}`
	if err := os.WriteFile(in, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	stub := &stubSource{groups: map[string]*models.Group{
		"network-all": {
			ID:   "id-network-all",
			Name: "network-all",
			Users: []models.User{
				{ID: "u1", Username: "alice@"},
				{ID: "u2", Username: "bob@"},
			},
		},
		"network-eks-admin-prod": {
			ID:    "id-eks-admin",
			Name:  "network-eks-admin-prod",
			Users: []models.User{}, // group exists, no members
		},
		"corp-vpn": {
			ID:   "id-corp-vpn",
			Name: "corp-vpn",
			Users: []models.User{
				{ID: "u3", Username: "carol@"},
			},
		},
		// "ops" intentionally absent — group not found in source
	}}

	// preparePolicy reads the package-level path vars set by cobra at runtime.
	prevIn, prevOut := inputPolicyFile, outputPolicyFile
	inputPolicyFile, outputPolicyFile = in, out
	t.Cleanup(func() { inputPolicyFile, outputPolicyFile = prevIn, prevOut })

	logCh := make(chan string, 16)
	done := make(chan struct{})
	go func() {
		for range logCh {
		}
		close(done)
	}()

	if err := preparePolicy(stub, logCh); err != nil {
		t.Fatalf("preparePolicy: %v", err)
	}
	close(logCh)
	<-done

	rawOut, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	// Output is HuJSON (keeps template comments); standardize to decode it.
	clean := standardizeJSON(t, rawOut)

	var got struct {
		Groups        map[string][]string `json:"groups"`
		Hosts         map[string]string   `json:"hosts"`
		TagOwners     map[string][]string `json:"tagOwners"`
		ACLs          []map[string]any    `json:"acls"`
		AutoApprovers struct {
			Routes   map[string][]string `json:"routes"`
			ExitNode []string            `json:"exitNode"`
		} `json:"autoApprovers"`
		SSH    []any `json:"ssh"`
		Schema any   `json:"$schema"`
	}
	if err := json.Unmarshal(clean, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	// Static group preserved (not in source)
	if v := got.Groups["group:ops"]; len(v) != 1 || v[0] != "ops@" {
		t.Errorf("group:ops should be preserved from template, got %v", v)
	}

	// Dynamic group overwritten (in source)
	want := []string{"alice@", "bob@"}
	if v := got.Groups["group:network-all"]; len(v) != len(want) || v[0] != want[0] || v[1] != want[1] {
		t.Errorf("group:network-all should be overwritten by source: got %v want %v", v, want)
	}

	// Empty group in source serializes as []
	if v, ok := got.Groups["group:network-eks-admin-prod"]; !ok || v == nil || len(v) != 0 {
		t.Errorf("group:network-eks-admin-prod should be empty slice, got %#v (ok=%v)", v, ok)
	}

	// Source-populated previously-empty group
	if v := got.Groups["group:corp-vpn"]; len(v) != 1 || v[0] != "carol@" {
		t.Errorf("group:corp-vpn should reflect source members, got %v", v)
	}

	// Hosts preserved verbatim (value intact; key kept as written).
	// Decoding is case-insensitive, so "Hosts" from the template still loads.
	if got.Hosts["app-ca-prod-vpc"] != "10.2.0.0/16" {
		t.Errorf("hosts entry lost: %v", got.Hosts)
	}

	// tagOwners + autoApprovers preserved
	if v := got.TagOwners["tag:prod-vpn"]; len(v) != 1 || v[0] != "group:ops" {
		t.Errorf("tagOwners lost: %v", v)
	}
	if got.AutoApprovers.ExitNode == nil || got.AutoApprovers.ExitNode[0] != "group:ops" {
		t.Errorf("autoApprovers.exitNode lost: %v", got.AutoApprovers.ExitNode)
	}
	if v := got.AutoApprovers.Routes["10.0.0.0/8"]; len(v) != 1 || v[0] != "group:ops" {
		t.Errorf("autoApprovers.routes lost: %v", v)
	}

	// ACLs preserved
	if len(got.ACLs) != 1 {
		t.Errorf("ACLs lost: got %d", len(got.ACLs))
	}

	// ssh absent from the template must NOT be injected into the output.
	// The tool only fills groups; it never fabricates sections.
	var rawMap map[string]any
	if err := json.Unmarshal(clean, &rawMap); err != nil {
		t.Fatalf("unmarshal output map: %v", err)
	}
	if _, present := rawMap["ssh"]; present {
		t.Errorf("ssh not in template must not appear in output, got: %s", rawOut)
	}

	// Template comments survive into the output (HuJSON passthrough).
	for _, c := range []string{"// ops is not in the source", "// network-all is in the source"} {
		if !strings.Contains(string(rawOut), c) {
			t.Errorf("template comment %q lost from output:\n%s", c, rawOut)
		}
	}

	// $schema is template-only editor metadata — it must NOT leak into
	// the policy file Headscale loads.
	if got.Schema != nil {
		t.Errorf("$schema must be dropped on output (template-only); got %v", got.Schema)
	}
}

// TestFlagDefaultsDoNotLeakEnvSecrets guards against secrets like PF_TOKEN
// or PF_LDAP_BIND_PASSWORD ending up in cobra's --help output. The flag
// defaults must be empty strings; env-var resolution happens at PreRun.
func TestFlagDefaultsDoNotLeakEnvSecrets(t *testing.T) {
	for _, name := range []string{
		"token",
		"ldap-bind-password",
		"ldap-bind-dn",
		"ldap-base-dn",
		"endpoint",
		"source",
		"keycloak-realm",
		"ldap-default-email-domain",
	} {
		f := cliCmd.PersistentFlags().Lookup(name)
		if f == nil {
			t.Errorf("flag %q not registered", name)
			continue
		}
		if f.DefValue != "" {
			t.Errorf("flag --%s has non-empty default %q; secrets must not be baked into --help. "+
				"Set the default to \"\" and read the env var from PersistentPreRun.", name, f.DefValue)
		}
	}
}

// TestPreparePolicy_NoGroupsInTemplate guards the early-out path: if the
// HJSON template defines no group: prefixed keys, preparePolicy must error
// rather than silently writing an empty policy.
func TestPreparePolicy_NoGroupsInTemplate(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "empty.hjson")
	out := filepath.Join(tmp, "out.json")
	if err := os.WriteFile(in, []byte(`{"groups": {}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	prevIn, prevOut := inputPolicyFile, outputPolicyFile
	inputPolicyFile, outputPolicyFile = in, out
	t.Cleanup(func() { inputPolicyFile, outputPolicyFile = prevIn, prevOut })

	logCh := make(chan string, 4)
	go func() {
		for range logCh {
		}
	}()
	defer close(logCh)

	err := preparePolicy(&stubSource{}, logCh)
	if err == nil {
		t.Fatalf("expected error when template has no groups")
	}
}
