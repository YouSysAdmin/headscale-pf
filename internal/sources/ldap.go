package sources

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"

	"github.com/yousysadmin/headscale-pf/internal/models"
)

// LDAP implements Source for LDAP directories (AD / OpenLDAP / JumpCloud LDAPaaS).
// It supports:
//   - groupOfNames / group via "member" / "uniqueMember" (DN-valued)
//   - posixGroup via "memberUid" (login name / uid-valued)
type LDAP struct {
	Addr     string // "host:389" or "host:636"
	UseTLS   bool   // true for LDAPS on 636 (preferred). If false, StartTLS is attempted.
	BindDN   string
	BindPass string
	BaseDN   string

	// Attribute preferences (override if your directory differs)
	GroupNameAttr      string   // usually "cn"
	UserEmailAttr      string   // AD: "mail" (or "userPrincipalName"); OpenLDAP/JumpCloud: "mail"
	UserLoginAttrs     []string // tried in order to produce username: e.g. ["sAMAccountName","uid","cn"]
	GroupMemberAttrs   []string // for DN members: ["member","uniqueMember"]
	PosixMemberUidAttr string   // for posixGroup usernames: "memberUid"
	UserObjectClasses  []string // e.g. ["person","organizationalPerson","user","inetOrgPerson"]
	GroupObjectClasses []string // e.g. ["groupOfNames","group","posixGroup"]

	// When true, if a member is a group DN, expand one level (NOT recursive).
	ExpandOneLevelNested bool

	// Domain used to synthesize an email when none is present (username@DefaultEmailDomain).
	DefaultEmailDomain string
}

// NewLDAPClient constructs an LDAP client with sensible defaults.
func NewLDAPClient(config SourceConfig) (*LDAP, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("ldap endpoint must be specified (e.g. ldap.jumpcloud.com:636)")
	}
	if config.LDAPBindDN == "" {
		return nil, fmt.Errorf("ldap bind DN must be specified (e.g. uid=svc,ou=Users,o=<ORG_ID>,dc=jumpcloud,dc=com)")
	}
	if config.LDAPBaseDN == "" {
		return nil, fmt.Errorf("ldap base DN must be specified (e.g. o=<ORG_ID>,dc=jumpcloud,dc=com)")
	}
	if config.LDAPBindPassword == "" {
		return nil, fmt.Errorf("ldap bind password must be specified")
	}
	if config.LDAPDefaultEmailDomain == "" {
		config.LDAPDefaultEmailDomain = "example.com"
	}

	return &LDAP{
		Addr:     config.Endpoint,
		UseTLS:   strings.HasSuffix(strings.ToLower(config.Endpoint), ":636"),
		BindDN:   config.LDAPBindDN,
		BaseDN:   config.LDAPBaseDN,
		BindPass: config.LDAPBindPassword,

		GroupNameAttr:        "cn",
		UserEmailAttr:        "mail",
		UserLoginAttrs:       []string{"sAMAccountName", "uid", "cn"},
		GroupMemberAttrs:     []string{"member", "uniqueMember"},
		PosixMemberUidAttr:   "memberUid",
		UserObjectClasses:    []string{"person", "organizationalPerson", "user", "inetOrgPerson"},
		GroupObjectClasses:   []string{"groupOfNames", "group", "posixGroup"},
		ExpandOneLevelNested: false,
		DefaultEmailDomain:   config.LDAPDefaultEmailDomain,
	}, nil
}

// GetGroupByName finds the first group whose GroupNameAttr (default "cn")
// exactly matches the provided groupName. It returns the group's DN as ID.
func (c *LDAP) GetGroupByName(groupName string) (*models.Group, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	filter := fmt.Sprintf(
		"(&(|%s)(%s=%s))",
		c.joinOC(c.GroupObjectClasses),
		ldap.EscapeFilter(c.GroupNameAttr),
		ldap.EscapeFilter(groupName),
	)

	req := ldap.NewSearchRequest(
		c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 10, false,
		filter,
		[]string{"dn", c.GroupNameAttr},
		nil,
	)

	sr, err := conn.SearchWithPaging(req, 50)
	if err != nil {
		return nil, fmt.Errorf("search group by name: %w", err)
	}
	if len(sr.Entries) == 0 {
		return nil, nil
	}
	entry := sr.Entries[0]
	name := entry.GetAttributeValue(c.GroupNameAttr)
	return &models.Group{
		ID:   entry.DN, // group ID = DN
		Name: name,
	}, nil
}

// GetGroupMembers returns users in the group identified by groupID (expected to be a DN).
// For posixGroup, it resolves memberUid logins to user entries.
// For groupOfNames/group, it resolves each member DN to a user entry.
// If ExpandOneLevelNested is true, a member that is itself a group will be expanded one level.
func (c *LDAP) GetGroupMembers(groupID string) ([]models.User, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Load the group entry by DN
	groupReq := ldap.NewSearchRequest(
		groupID, // group's DN
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 10, false,
		"(objectClass=*)",
		append([]string{"objectClass", c.PosixMemberUidAttr}, c.GroupMemberAttrs...),
		nil,
	)
	gsr, err := conn.Search(groupReq)
	if err != nil || len(gsr.Entries) == 0 {
		return nil, fmt.Errorf("resolve group DN %q: %w", groupID, err)
	}
	group := gsr.Entries[0]
	oc := strings.ToLower(strings.Join(group.GetAttributeValues("objectClass"), " "))

	users := make([]models.User, 0)

	// posixGroup via memberUid (already usernames)
	if strings.Contains(oc, "posixgroup") {
		memberUids := group.GetAttributeValues(c.PosixMemberUidAttr)
		for _, u := range memberUids {
			user, err := c.lookupUserByLogin(conn, u)
			if err != nil {
				// Skip missing users, keep going
				continue
			}
			users = append(users, user)
		}
		return users, nil
	}

	// member/uniqueMember DNs
	memberDNs := make([]string, 0, 32)
	for _, a := range c.GroupMemberAttrs {
		memberDNs = append(memberDNs, group.GetAttributeValues(a)...)
	}

	// Expand one level of nested groups
	if c.ExpandOneLevelNested && len(memberDNs) > 0 {
		expanded := make([]string, 0, len(memberDNs))
		for _, dn := range memberDNs {
			isGroup, err := c.dnIsGroup(conn, dn)
			if err != nil {
				continue
			}
			if !isGroup {
				expanded = append(expanded, dn)
				continue
			}
			// Pull inner members (only one level)
			req := ldap.NewSearchRequest(
				dn,
				ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 10, false,
				"(objectClass=*)",
				c.GroupMemberAttrs,
				nil,
			)
			sr, err := conn.Search(req)
			if err != nil || len(sr.Entries) == 0 {
				continue
			}
			for _, a := range c.GroupMemberAttrs {
				expanded = append(expanded, sr.Entries[0].GetAttributeValues(a)...)
			}
		}
		memberDNs = expanded
	}

	// Resolve DN members to users with a timeout guard
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, dn := range memberDNs {
		select {
		case <-ctx.Done():
			return users, ctx.Err()
		default:
		}
		u, err := c.lookupUserByDN(conn, dn)
		if err == nil {
			users = append(users, u)
		}
	}
	return users, nil
}

// GetUserInfo resolves a user by ID. If userID looks like a DN, it loads that DN.
// Otherwise it treats userID as a login (sAMAccountName/uid/cn—config-driven) and searches.
func (c *LDAP) GetUserInfo(userID string) (models.User, error) {
	conn, err := c.connect()
	if err != nil {
		return models.User{}, err
	}
	defer conn.Close()

	// DN path
	if strings.Contains(userID, "=") && strings.Contains(userID, ",") {
		return c.lookupUserByDN(conn, userID)
	}
	// Otherwise treat as login name (uid/sAMAccountName/cn)
	return c.lookupUserByLogin(conn, userID)
}

// connect dials the LDAP server and performs a simple bind.
// If UseTLS is false, it attempts StartTLS (best-effort).
func (c *LDAP) connect() (*ldap.Conn, error) {
	var conn *ldap.Conn
	var err error
	if c.UseTLS {
		conn, err = ldap.DialTLS("tcp", c.Addr, &tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		conn, err = ldap.Dial("tcp", c.Addr)
		if err == nil {
			_ = conn.StartTLS(&tls.Config{InsecureSkipVerify: true})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("ldap dial: %w", err)
	}
	if err := conn.Bind(c.BindDN, c.BindPass); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ldap bind: %w", err)
	}
	return conn, nil
}

// joinOC builds an LDAP filter fragment that ORs multiple objectClass checks.
// For example, ["groupOfNames","group"] => "(objectClass=groupOfNames)(objectClass=group)"
// The caller typically wraps this with an enclosing "(| ... )".
func (c *LDAP) joinOC(classes []string) string {
	parts := make([]string, 0, len(classes))
	for _, oc := range classes {
		parts = append(parts, fmt.Sprintf("(objectClass=%s)", ldap.EscapeFilter(oc)))
	}
	return strings.Join(parts, "")
}

// dnIsGroup returns true if the entry at the given DN has an objectClass that
// matches any name in GroupObjectClasses. It uses a base-object search to avoid
// scanning the tree.
func (c *LDAP) dnIsGroup(conn *ldap.Conn, dn string) (bool, error) {
	req := ldap.NewSearchRequest(
		dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 5, false,
		"(objectClass=*)", []string{"objectClass"}, nil,
	)
	sr, err := conn.Search(req)
	if err != nil || len(sr.Entries) == 0 {
		return false, err
	}
	oc := strings.ToLower(strings.Join(sr.Entries[0].GetAttributeValues("objectClass"), " "))
	for _, g := range c.GroupObjectClasses {
		if strings.Contains(oc, strings.ToLower(g)) {
			return true, nil
		}
	}
	return false, nil
}

// lookupUserByDN fetches a user entry by its DN (base-object search) and maps
// it into models.User via entryToUser.
func (c *LDAP) lookupUserByDN(conn *ldap.Conn, dn string) (models.User, error) {
	req := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 5, false,
		"(objectClass=*)",
		append([]string{"dn", c.UserEmailAttr}, c.UserLoginAttrs...),
		nil,
	)
	sr, err := conn.Search(req)
	if err != nil || len(sr.Entries) == 0 {
		return models.User{}, fmt.Errorf("user DN %q not found", dn)
	}
	e := sr.Entries[0]
	return c.entryToUser(e), nil
}

// lookupUserByLogin searches the directory subtree (BaseDN) for a user whose
// login attributes (UserLoginAttrs) equal the provided login. It limits results
// to avoid ambiguity and returns the first match mapped via entryToUser.
func (c *LDAP) lookupUserByLogin(conn *ldap.Conn, login string) (models.User, error) {
	// Build an OR filter across allowed user objectClasses and login attrs
	loginFilterParts := make([]string, 0, len(c.UserLoginAttrs))
	for _, a := range c.UserLoginAttrs {
		loginFilterParts = append(loginFilterParts, fmt.Sprintf("(%s=%s)", ldap.EscapeFilter(a), ldap.EscapeFilter(login)))
	}
	filter := fmt.Sprintf("(&(|%s)(|%s))", c.joinOC(c.UserObjectClasses), strings.Join(loginFilterParts, ""))

	req := ldap.NewSearchRequest(
		c.BaseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 10, false,
		filter,
		append([]string{"dn", c.UserEmailAttr}, c.UserLoginAttrs...),
		nil,
	)
	sr, err := conn.SearchWithPaging(req, 50)
	if err != nil || len(sr.Entries) == 0 {
		return models.User{}, fmt.Errorf("user %q not found", login)
	}
	return c.entryToUser(sr.Entries[0]), nil
}

// entryToUser converts an LDAP entry into models.User.
// It picks the first non-empty attribute from UserLoginAttrs as "username" and
// prefers UserEmailAttr for the email. If the email is absent but a username is
// available and DefaultEmailDomain is set, it synthesizes username@DefaultEmailDomain.
func (c *LDAP) entryToUser(e *ldap.Entry) models.User {
	// Username (preferred attr in order)
	var userName string
	for _, a := range c.UserLoginAttrs {
		if v := e.GetAttributeValue(a); v != "" {
			userName = v
			if !strings.Contains(userName, "@") {
				userName += "@"
			}
			break
		}
	}

	// Email (fallback: synthesize from username)
	email := e.GetAttributeValue(c.UserEmailAttr)
	if email == "" && userName != "" && c.DefaultEmailDomain != "" {
		email = fmt.Sprintf("%s%s", userName, c.DefaultEmailDomain)
	}
	// Map to models.User. Adjust field names if they differ.
	return models.User{
		ID:       e.DN, // user ID = DN
		Email:    email,
		Username: userName,
	}
}
