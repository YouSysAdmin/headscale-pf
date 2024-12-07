package sources

import (
	"fmt"
	"github.com/yousysadmin/headscale-pf/internal/models"
)

// Source interface
type Source interface {
	GetGroupByName(grounName string) (*models.Group, error)
	GetGroupMembers(groupId string, stripEmailDomain bool) ([]models.User, error)
	GetUserInfo(userId string, stripEmailDomain bool) (models.User, error)
}

// SourceConfig config source
type SourceConfig struct {
	Name     string // Name source name
	Endpoint string // Endpoint source endpoint
	Username string // Username source auth username
	Password string // Password source auth password
	Token    string // Token source auth token
}

// NewSource init source
func NewSource(config SourceConfig) (Source, error) {
	switch config.Name {
	case "jc":
		return NewJCClient(config)
	case "ak":
		return NewAuthentikClient(config)
	default:
		return nil, fmt.Errorf("unknown source name")
	}
}
