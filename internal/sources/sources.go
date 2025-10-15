package sources

import (
	"fmt"

	"github.com/yousysadmin/headscale-pf/internal/models"
)

// Source interface
type Source interface {
	GetGroupByName(grounName string) (*models.Group, error)
	GetGroupMembers(groupId string) ([]models.User, error)
	GetUserInfo(userId string) (models.User, error)
}

// SourceConfig config source
type SourceConfig struct {
	Name                   string // Name source name
	Endpoint               string // Endpoint source endpoint
	Token                  string // Token source auth token
	LDAPBindPassword       string // LDAP bind password
	LDAPBindDN             string // LDAP BindDN
	LDAPBaseDN             string // LDAP BaseDN
	LDAPDefaultEmailDomain string // Default email domain what used for synthesize an email when none is present (username@DefaultEmailDomain).
}

// NewSource init source
func NewSource(config SourceConfig) (Source, error) {
	switch config.Name {
	case "jc":
		return NewJCClient(config)
	case "ak":
		return NewAuthentikClient(config)
	case "ldap", "ldaps":
		return NewLDAPClient(config)
	default:
		return nil, fmt.Errorf("unknown source name")
	}
}
