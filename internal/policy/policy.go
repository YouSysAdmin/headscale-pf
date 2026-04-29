package policy

import (
	"encoding/json"
	"net/netip"
	"os"
	"strings"

	v2Policy "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/tailscale/hujson"
)

var f v2Policy.Policy

// Policy extend Headscale policy
type Policy struct {
	Groups        map[string][]string     `json:"groups"`
	Hosts         map[string]netip.Prefix `json:"hosts"`
	TagOwners     map[string][]string     `json:"tagOwners"`
	ACLs          []ACL                   `json:"acls"`
	AutoApprovers AutoApprovers           `json:"autoApprovers"`
	SSHs          []SSH                   `json:"ssh"`
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
