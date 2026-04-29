package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yousysadmin/headscale-pf/internal/models"
	"github.com/yousysadmin/headscale-pf/pkg/tools"
	api "goauthentik.io/api/v3"
)

// Authentik source
type Authentik struct {
	V3 *api.APIClient
}

// NewAuthentikClient init Authentik source
func NewAuthentikClient(config SourceConfig) (*Authentik, error) {
	if len(config.Token) <= 0 {
		return nil, errors.New("token is required")
	}
	if len(config.Endpoint) <= 0 {
		return nil, errors.New("endpoint is required")
	}

	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, err
	}

	transport, err := tools.GetTLSTransport(config.InsecureSkipTLSVerify)
	if err != nil {
		return nil, fmt.Errorf("authentik: build TLS transport: %w", err)
	}

	akConf := api.NewConfiguration()
	akConf.Debug = false
	akConf.Scheme = endpoint.Scheme
	akConf.Host = endpoint.Hostname()
	akConf.HTTPClient = &http.Client{Transport: transport}
	akConf.AddDefaultHeader("Authorization", fmt.Sprintf("Bearer %s", config.Token))

	return &Authentik{V3: api.NewAPIClient(akConf)}, nil
}

// GetGroupByName fetches the group and its members in a single API call.
// The returned Group has Users populated (possibly empty but never nil) so
// the caller can skip GetGroupMembers.
func (c *Authentik) GetGroupByName(groupName string) (*models.Group, error) {
	req, _, err := c.V3.CoreApi.CoreGroupsList(context.Background()).
		Name(groupName).
		IncludeUsers(true).
		Execute()
	if err != nil {
		return nil, err
	}

	if len(req.Results) == 0 {
		return nil, nil
	}

	return toGroup(req.Results[0]), nil
}

// GetGroupMembers re-fetches the group by PK and returns its members. Acts as
// a fallback for callers that hold a groupID without the embedded users.
func (c *Authentik) GetGroupMembers(groupID string) ([]models.User, error) {
	g, _, err := c.V3.CoreApi.CoreGroupsRetrieve(context.Background(), groupID).
		IncludeUsers(true).
		Execute()
	if err != nil {
		return nil, err
	}
	return toGroup(*g).Users, nil
}

// GetUserInfo is unused for Authentik because GetGroupByName returns members.
func (c *Authentik) GetUserInfo(userID string) (models.User, error) {
	return models.User{}, nil
}

// toGroup converts an Authentik Group response to models.Group with users
// populated. Users is always non-nil (empty slice when group has no members)
// so the caller can distinguish "preloaded but empty" from "not preloaded".
func toGroup(g api.Group) *models.Group {
	users := make([]models.User, 0, len(g.GetUsersObj()))
	for _, u := range g.GetUsersObj() {
		userName := u.Username
		if !strings.Contains(userName, "@") {
			userName += "@"
		}
		email := ""
		if u.Email != nil {
			email = *u.Email
		}
		users = append(users, models.User{
			ID:       u.Uid,
			Email:    email,
			Username: userName,
		})
	}
	return &models.Group{
		ID:    g.GetPk(),
		Name:  g.GetName(),
		Users: users,
	}
}
