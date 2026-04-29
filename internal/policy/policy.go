package policy

import (
	"encoding/json"
	"net/netip"
	"os"
	"strings"

	"github.com/tailscale/hujson"
)

// Policy extend Headscale policy. Known fields are typed so the tool can
// reason about them; any additional top-level field present in the input
// (e.g. "$schema", or new Headscale fields not yet modeled here) is captured
// verbatim in extra and re-emitted unchanged. The tool's contract is to fill
// group users — everything else round-trips as-is.
type Policy struct {
	Groups        map[string][]string     `json:"groups"`
	Hosts         map[string]netip.Prefix `json:"hosts"`
	TagOwners     map[string][]string     `json:"tagOwners"`
	ACLs          []ACL                   `json:"acls"`
	AutoApprovers AutoApprovers           `json:"autoApprovers"`
	SSHs          []SSH                   `json:"ssh"`

	// extra holds top-level keys that aren't represented above, so that
	// ReadPolicyFromFile + WritePolicyToFile preserve them.
	extra map[string]json.RawMessage
}

// knownTopLevelKeys lists JSON keys (lower-cased) covered by typed fields.
// Anything outside this set goes into Policy.extra. Encoding/json matches
// keys case-insensitively on unmarshal, so we lower-case here too.
var knownTopLevelKeys = map[string]struct{}{
	"groups":        {},
	"hosts":         {},
	"tagowners":     {},
	"acls":          {},
	"autoapprovers": {},
	"ssh":           {},
}

// templateOnlyKeys are top-level keys that exist in the HJSON template for
// editor/IDE tooling but are not part of the Headscale policy. They are
// dropped on read so they never appear in the JSON output.
var templateOnlyKeys = map[string]struct{}{
	"$schema": {},
}

// UnmarshalJSON populates typed fields and captures everything else into
// extra so unknown fields round-trip through WritePolicyToFile.
func (p *Policy) UnmarshalJSON(data []byte) error {
	type alias Policy
	if err := json.Unmarshal(data, (*alias)(p)); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	extra := make(map[string]json.RawMessage)
	for k, v := range raw {
		if _, known := knownTopLevelKeys[strings.ToLower(k)]; known {
			continue
		}
		if _, drop := templateOnlyKeys[k]; drop {
			continue
		}
		extra[k] = v
	}
	if len(extra) > 0 {
		p.extra = extra
	}
	return nil
}

// MarshalJSON merges the typed fields with extra so that unknown top-level
// keys captured during UnmarshalJSON are emitted alongside the known ones.
func (p *Policy) MarshalJSON() ([]byte, error) {
	type alias Policy
	known, err := json.Marshal((*alias)(p))
	if err != nil {
		return nil, err
	}
	if len(p.extra) == 0 {
		return known, nil
	}
	merged := make(map[string]json.RawMessage, len(p.extra)+8)
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	for k, v := range p.extra {
		merged[k] = v
	}
	return json.Marshal(merged)
}

type ACL struct {
	Action       string   `json:"action"`
	Protocol     string   `json:"proto"`
	Sources      []string `json:"src"`
	Destinations []string `json:"dst"`
}

type AutoApprovers struct {
	Routes   map[string][]string `json:"routes"`
	ExitNode []string            `json:"exitNode"`
}

type SSH struct {
	Action       string   `json:"action"`
	Sources      []string `json:"src"`
	Destinations []string `json:"dst"`
	Users        []string `json:"users"`
	CheckPeriod  string   `json:"checkPeriod,omitempty"`
}

// ReadPolicyFromFile read Headscale policy from file
func (p *Policy) ReadPolicyFromFile(path string) error {
	policyData, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ast, err := hujson.Parse(policyData)
	if err != nil {
		return err
	}
	ast.Standardize()
	data := ast.Pack()

	err = json.Unmarshal(data, &p)

	return err
}

// WritePolicyToFile write Headscale policy from file
func (p *Policy) WritePolicyToFile(path string) error {
	p.sanitize()

	data, err := json.Marshal(p)
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

// AppendGroups append group to policy
func (p *Policy) AppendGroups(groups map[string][]string) {
	if p.Groups == nil {
		p.Groups = make(map[string][]string)
	}
	for g, u := range groups {
		p.Groups[g] = u
	}
}

// GetGroupNames get group names from policy file
func (p *Policy) GetGroupNames() []string {
	var groups []string
	for k := range p.Groups {
		parts := strings.Split(k, ":")
		if len(parts) >= 2 {
			group := parts[1]
			groups = append(groups, group)
		}
	}

	return groups
}

// sanitize prevents set `null` as value for empty groups and ssh
// `group:"example": null` -> `group:"example": []`
func (p *Policy) sanitize() {
	if p.SSHs == nil {
		p.SSHs = []SSH{}
	}
	for k, v := range p.Groups {
		if v == nil {
			p.Groups[k] = []string{}
		}
	}
}
