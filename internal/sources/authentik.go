package sources

import (
	"context"
	"errors"
	"fmt"
	"github.com/yousysadmin/headscale-pf/internal/models"
	"github.com/yousysadmin/headscale-pf/pkg/tools"
	api "goauthentik.io/api/v3"
	"net/http"
	"net/url"
	"strings"
)

// Authentik source
type Authentik struct {
	V3    *api.APIClient
	group *models.Group
}

// NewAuthentikClient init Authentik source
func NewAuthentikClient(config SourceConfig) (Authentik, error) {
	if len(config.Token) <= 0 {
		return Authentik{}, errors.New("token is required")
	}
	if len(config.Endpoint) <= 0 {
		return Authentik{}, errors.New("endpoint is required")
	}

	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		return Authentik{}, err
	}

	scheme := endpoint.Scheme
	host := endpoint.Hostname()

	akConf := api.NewConfiguration()
	akConf.Debug = false
	akConf.Scheme = scheme
	akConf.Host = host
	akConf.HTTPClient = &http.Client{Transport: tools.GetTLSTransport(true)}
	if scheme == "https" {
		akConf.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", config.Token))
	}

	c := Authentik{V3: api.NewAPIClient(akConf), group: &models.Group{}}

	return c, nil
}

// GetGroupByName Get Authentik group by name
func (c Authentik) GetGroupByName(grounName string) (*models.Group, error) {
	req, _, err := c.V3.CoreApi.CoreGroupsList(context.Background()).
		Name(grounName).
		IncludeUsers(true).
		Execute()
	if err != nil {
		return nil, err
	}

	if len(req.Results) > 0 {
		group := &models.Group{}
		group.ID = req.Results[0].GetPk()
		group.Name = req.Results[0].GetName()

		for _, u := range req.Results[0].GetUsersObj() {
			group.Users = append(group.Users, models.User{ID: u.Uid, Email: *u.Email})
		}

		c.group.Users = group.Users

		return group, nil
	}

	return nil, nil
}

// GetGroupMembers Get Authentik group members
func (c Authentik) GetGroupMembers(groupId string, stripEmailDomain bool) ([]models.User, error) {
	if len(c.group.Users) > 0 {
		var users []models.User
		for _, u := range c.group.Users {
			if stripEmailDomain {
				u.Part = strings.Split(u.Email, "@")[0]
			} else {
				u.Part = u.Email
			}
			users = append(users, u)
		}
		return users, nil
	}
	return nil, nil
}

// GetUserInfo get Authentik user info
// For Authentik is not used because Authentik returns group members while getting group info
func (c Authentik) GetUserInfo(userId string, stripEmailDomain bool) (models.User, error) {
	return models.User{}, nil
}
