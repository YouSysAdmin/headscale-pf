package sources

import (
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func newLDAPForTest() *LDAP {
	return &LDAP{
		UserEmailAttr:      "mail",
		UserLoginAttrs:     []string{"sAMAccountName", "uid", "cn"},
		DefaultEmailDomain: "example.com",
	}
}

func entry(dn string, attrs map[string][]string) *ldap.Entry {
	out := &ldap.Entry{DN: dn}
	for k, v := range attrs {
		out.Attributes = append(out.Attributes, &ldap.EntryAttribute{Name: k, Values: v})
	}
	return out
}

func TestEntryToUser(t *testing.T) {
	cases := []struct {
		name         string
		attrs        map[string][]string
		dn           string
		wantID       string
		wantEmail    string
		wantUsername string
	}{
		{
			name: "mail attr present, uid as username",
			dn:   "uid=alice,ou=Users,dc=example,dc=com",
			attrs: map[string][]string{
				"uid":  {"alice"},
				"mail": {"alice@elsewhere.com"},
			},
			wantID:       "uid=alice,ou=Users,dc=example,dc=com",
			wantEmail:    "alice@elsewhere.com",
			wantUsername: "alice@",
		},
		{
			name: "no mail attr, synthesize from default domain",
			dn:   "uid=bob,ou=Users,dc=example,dc=com",
			attrs: map[string][]string{
				"uid": {"bob"},
			},
			wantEmail:    "bob@example.com",
			wantUsername: "bob@",
			wantID:       "uid=bob,ou=Users,dc=example,dc=com",
		},
		{
			name: "username already contains @, not double-suffixed",
			dn:   "uid=carol@corp,ou=Users,dc=example,dc=com",
			attrs: map[string][]string{
				"uid":  {"carol@corp"},
				"mail": {"carol@corp.example.com"},
			},
			wantEmail:    "carol@corp.example.com",
			wantUsername: "carol@corp",
			wantID:       "uid=carol@corp,ou=Users,dc=example,dc=com",
		},
		{
			name: "first non-empty UserLoginAttrs wins (sAMAccountName preferred over uid)",
			dn:   "CN=dave,OU=Users,DC=example,DC=com",
			attrs: map[string][]string{
				"sAMAccountName": {"dave"},
				"uid":            {"daveid"},
				"cn":             {"Dave Daveson"},
				"mail":           {"dave@example.com"},
			},
			wantUsername: "dave@",
			wantEmail:    "dave@example.com",
			wantID:       "CN=dave,OU=Users,DC=example,DC=com",
		},
		{
			name: "no login attrs and no mail → empty username, empty email (no synthesis without username)",
			dn:   "uid=ghost,ou=Users,dc=example,dc=com",
			attrs: map[string][]string{
				"description": {"placeholder only"},
			},
			wantUsername: "",
			wantEmail:    "",
			wantID:       "uid=ghost,ou=Users,dc=example,dc=com",
		},
	}

	c := newLDAPForTest()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.entryToUser(entry(tc.dn, tc.attrs))
			if got.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tc.wantID)
			}
			if got.Username != tc.wantUsername {
				t.Errorf("Username = %q, want %q", got.Username, tc.wantUsername)
			}
			if got.Email != tc.wantEmail {
				t.Errorf("Email = %q, want %q", got.Email, tc.wantEmail)
			}
		})
	}
}

func TestEntryToUser_NoDefaultDomain_NoSynthesis(t *testing.T) {
	c := newLDAPForTest()
	c.DefaultEmailDomain = ""
	got := c.entryToUser(entry("uid=erin,dc=x", map[string][]string{"uid": {"erin"}}))
	if got.Email != "" {
		t.Errorf("Email should not be synthesized when DefaultEmailDomain is empty: got %q", got.Email)
	}
	if got.Username != "erin@" {
		t.Errorf("Username should still get @ suffix: got %q", got.Username)
	}
}

func TestHasObjectClass(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		target string
		want   bool
	}{
		{"exact match", []string{"posixGroup", "top"}, "posixGroup", true},
		{"case insensitive", []string{"POSIXGROUP"}, "posixGroup", true},
		{"no false positive on substring", []string{"posixGroupExtended"}, "posixGroup", false},
		{"empty values", nil, "posixGroup", false},
		{"target absent", []string{"groupOfNames"}, "posixGroup", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasObjectClass(tc.values, tc.target); got != tc.want {
				t.Errorf("hasObjectClass(%v, %q) = %v, want %v", tc.values, tc.target, got, tc.want)
			}
		})
	}
}
