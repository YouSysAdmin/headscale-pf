package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gocloak "github.com/Nerzal/gocloak/v13"
	"github.com/yousysadmin/headscale-pf/internal/models"
)

type Keycloak struct {
	client *gocloak.GoCloak
	realm  string
	token  string
}

func NewKeycloakClient(config SourceConfig) (Keycloak, error) {
	if len(config.Token) <= 0 {
		return Keycloak{}, errors.New("token is required")
	}
	if len(config.Endpoint) <= 0 {
		return Keycloak{}, errors.New("endpoint is required")
	}
	if len(config.KeycloakRealm) <= 0 {
		return Keycloak{}, errors.New("realm is required")
	}

	return Keycloak{
		client: gocloak.NewClient(config.Endpoint),
		realm:  config.KeycloakRealm,
		token:  config.Token,
	}, nil
}

// GetGroupByName Get Keycloak group by name
func (kc Keycloak) GetGroupByName(name string) (*models.Group, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	group, err := kc.client.GetGroups(ctx, kc.token, kc.realm, gocloak.GetGroupsParams{
		Search: gocloak.StringP(name),
	})
	if err != nil {
		return nil, err
	}

	if len(group) != 0 && gocloak.PString(group[0].Name) == name {
		return &models.Group{
			ID:   gocloak.PString(group[0].ID),
			Name: gocloak.PString(group[0].Name),
		}, nil
	}

	return nil, nil
}

// GetGroupMembers gets ALL Keycloak group members (handles pagination)
func (kc Keycloak) GetGroupMembers(groupID string, stripEmailDomain bool) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	const pageSize = 200 // page size
	first := 0

	out := make([]models.User, 0, pageSize)
	seen := make(map[string]struct{}, pageSize)

	for {
		users, err := kc.fetchGroupMembersPage(ctx, groupID, first, pageSize)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			break
		}

		for _, u := range users {
			mu := toModelUser(u, stripEmailDomain)
			if _, ok := seen[mu.ID]; ok {
				continue
			}
			seen[mu.ID] = struct{}{}
			out = append(out, mu)
		}

		first += len(users)
		// if len of user list < pageSize that is the last page
		// break paginatin
		if len(users) < pageSize {
			break
		}
	}

	return out, nil
}

// GetUserInfo get Keycloak user info [UNUSED]
func (kc Keycloak) GetUserInfo(userID string, stripEmailDomain bool) (models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, err := kc.client.GetUserByID(ctx, kc.token, kc.realm, userID)
	if err != nil {
		return models.User{}, err
	}
	return toModelUser(u, stripEmailDomain), nil
}

// pagination
func (kc *Keycloak) fetchGroupMembersPage(ctx context.Context, groupID string, first, max int) ([]*gocloak.User, error) {
	const (
		maxAttempts = 5
		baseBackoff = 300 * time.Millisecond
	)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		users, err := kc.client.GetGroupMembers(
			ctx, kc.token, kc.realm, groupID,
			gocloak.GetGroupsParams{
				First: gocloak.IntP(first),
				Max:   gocloak.IntP(max),
			},
		)
		if err == nil {
			return users, nil
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		lastErr = err
		time.Sleep(time.Duration(attempt) * baseBackoff)
	}
	return nil, fmt.Errorf("keycloak: failed to get members (first=%d,max=%d): %w", first, max, lastErr)
}

// Mapping keycloak user to models.User{}
func toModelUser(u *gocloak.User, stripEmailDomain bool) models.User {
	email := gocloak.PString(u.Email)

	var userName string
	if stripEmailDomain {
		userName = strings.Split(email, "@")[0] + "@"
	} else {
		userName = email
	}

	return models.User{
		ID:    gocloak.PString(u.ID),
		Email: email,
		Part:  userName,
	}
}
