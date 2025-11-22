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
	KeycloakRealm          string // Keycloak Realm
}

// NewSource init source
func NewSource(config SourceConfig) (Source, error) {
	switch config.Name {
	case "jc", "jumpcloud":
		return NewJCClient(config)
	case "ak", "authentik":
		return NewAuthentikClient(config)
	case "ldap", "ldaps":
		return NewLDAPClient(config)
	case "kk", "keycloak":
		return NewKeycloakClient(config)
	default:
		return nil, fmt.Errorf("unknown source name")
	}
}
