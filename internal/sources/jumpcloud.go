package sources

import (
	"context"
	"errors"
	"fmt"
	"github.com/yousysadmin/headscale-pf/internal/models"
	"strings"

	jcapiv1 "github.com/TheJumpCloud/jcapi-go/v1"
	jcapiv2 "github.com/TheJumpCloud/jcapi-go/v2"
)

// Jumpcloud source
type Jumpcloud struct {
	V1          *jcapiv1.APIClient
	V1Auth      context.Context
	V2          *jcapiv2.APIClient
	V2Auth      context.Context
	ContentType string
}

// NewJCClient init Jumpcloud source
func NewJCClient(config SourceConfig) (Jumpcloud, error) {
	if len(config.Token) <= 0 {
		return Jumpcloud{}, errors.New("token is required")
	}

	c := Jumpcloud{}
	c.V1 = jcapiv1.NewAPIClient(jcapiv1.NewConfiguration())
	c.V1Auth = context.WithValue(context.TODO(), jcapiv1.ContextAPIKey, jcapiv1.APIKey{
		Key: config.Token,
	})

	c.V2 = jcapiv2.NewAPIClient(jcapiv2.NewConfiguration())
	c.V2Auth = context.WithValue(context.TODO(), jcapiv2.ContextAPIKey, jcapiv2.APIKey{
		Key: config.Token,
	})

	c.ContentType = "application/json"

	return c, nil
}

// GetGroupByName Get Jumpcloud group by name
func (c Jumpcloud) GetGroupByName(grounName string) (*models.Group, error) {
	filter := map[string]interface{}{
		"filter": []string{fmt.Sprintf("name:eq:%s", grounName)},
		"limit":  int32(100),
	}

	group, _, err := c.V2.UserGroupsApi.GroupsUserList(c.V2Auth, c.ContentType, c.ContentType, filter)
	if err != nil {
		return nil, err
	}

	if len(group) != 0 {
		return &models.Group{
			ID:   group[0].Id,
			Name: group[0].Name,
		}, nil
	}

	return nil, nil
}

// GetGroupMembers Get Jumpcloud group members
func (c Jumpcloud) GetGroupMembers(groupId string, stripEmailDomain bool) ([]models.User, error) {
	var users []models.User

	options := map[string]interface{}{
		"limit": int32(100),
	}

	groupUsers, _, err := c.V2.UserGroupsApi.GraphUserGroupMembership(c.V2Auth, groupId, c.ContentType, c.ContentType, options)
	if err != nil {
		return nil, err
	}

	for _, u := range groupUsers {
		user, err := c.GetUserInfo(u.Id, stripEmailDomain)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
}

// GetUserInfo get Jumpcloud user info
func (c Jumpcloud) GetUserInfo(userId string, stripEmailDomain bool) (models.User, error) {
	options := map[string]interface{}{
		"limit": int32(100),
	}

	user, _, err := c.V1.SystemusersApi.SystemusersGet(c.V1Auth, userId, c.ContentType, c.ContentType, options)
	if err != nil {
		return models.User{}, err
	}

	var userName string
	if stripEmailDomain {
		userName = strings.Split(user.Email, "@")[0] + "@"
	} else {
		userName = user.Email
	}

	userInfo := models.User{
		ID:    user.Id,
		Email: user.Email,
		Part:  userName,
	}

	return userInfo, nil
}
