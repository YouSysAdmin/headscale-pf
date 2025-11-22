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

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(p)

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	return nil
}

// AppendGroups append group to policy
func (p *Policy) AppendGroups(groups map[string][]string) {
	for g, u := range groups {
		p.Groups[g] = u
	}
}

// GetGroupNames get group names from policy file
func (p *Policy) GetGroupNames() []string {
	var groups []string
	for k, _ := range p.Groups {
		group := strings.Split(k, ":")[1]
		groups = append(groups, group)
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
