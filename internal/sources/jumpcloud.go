package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/yousysadmin/headscale-pf/internal/models"

	jcapiv1 "github.com/TheJumpCloud/jcapi-go/v1"
	jcapiv2 "github.com/TheJumpCloud/jcapi-go/v2"
)

// jcUserFetchWorkers caps concurrent SystemusersGet calls. Tuned to be
// well below JumpCloud's documented rate limit while still giving a
// meaningful speedup over the previous serial implementation.
const jcUserFetchWorkers = 8

// Jumpcloud source
type Jumpcloud struct {
	V1          *jcapiv1.APIClient
	V1Auth      context.Context
	V2          *jcapiv2.APIClient
	V2Auth      context.Context
	ContentType string
}

// NewJCClient init Jumpcloud source
func NewJCClient(config SourceConfig) (*Jumpcloud, error) {
	if len(config.Token) <= 0 {
		return nil, errors.New("token is required")
	}

	c := &Jumpcloud{}
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
func (c *Jumpcloud) GetGroupByName(groupName string) (*models.Group, error) {
	filter := map[string]any{
		"filter": []string{fmt.Sprintf("name:eq:%s", groupName)},
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

// GetGroupMembers gets ALL JumpCloud group members (handles pagination).
// The JumpCloud membership endpoint returns only user IDs, so each ID is
// resolved via getUserInfo. Lookups run through a bounded worker pool to
// avoid the N+1 latency of a serial loop while staying inside rate limits.
func (c *Jumpcloud) GetGroupMembers(groupID string) ([]models.User, error) {
	const pageSize int32 = 100
	skip := int32(0)
	seen := make(map[string]struct{})
	ids := make([]string, 0)

	for {
		opts := map[string]any{
			"limit":  pageSize,
			"skip":   skip,
			"fields": []string{"id"},
		}

		groupUsers, _, err := c.V2.UserGroupsApi.
			GraphUserGroupMembership(c.V2Auth, groupID, c.ContentType, c.ContentType, opts)
		if err != nil {
			return nil, err
		}
		if len(groupUsers) == 0 {
			break
		}

		for _, u := range groupUsers {
			if _, ok := seen[u.Id]; ok {
				continue
			}
			seen[u.Id] = struct{}{}
			ids = append(ids, u.Id)
		}

		if int32(len(groupUsers)) < pageSize {
			break
		}
		skip += int32(len(groupUsers))
	}

	return c.fetchUsersConcurrent(ids)
}

// fetchUsersConcurrent resolves user IDs in parallel with a bounded worker
// pool. The first error short-circuits and is returned to the caller.
func (c *Jumpcloud) fetchUsersConcurrent(ids []string) ([]models.User, error) {
	if len(ids) == 0 {
		return []models.User{}, nil
	}

	users := make([]models.User, len(ids))
	errs := make([]error, len(ids))

	sem := make(chan struct{}, jcUserFetchWorkers)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, id string) {
			defer wg.Done()
			defer func() { <-sem }()
			u, err := c.getUserInfo(id)
			if err != nil {
				errs[i] = err
				return
			}
			users[i] = u
		}(i, id)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return users, nil
}

// getUserInfo fetches a Jumpcloud user by ID. Used internally by
// GetGroupMembers to enrich the lightweight membership listing.
func (c *Jumpcloud) getUserInfo(userID string) (models.User, error) {
	options := map[string]any{
		"limit": int32(100),
	}

	user, _, err := c.V1.SystemusersApi.SystemusersGet(c.V1Auth, userID, c.ContentType, c.ContentType, options)
	if err != nil {
		return models.User{}, err
	}

	userName := user.Username
	if !strings.Contains(userName, "@") {
		userName += "@"
	}

	userInfo := models.User{
		ID:       user.Id,
		Email:    user.Email,
		Username: userName,
	}

	return userInfo, nil
}
