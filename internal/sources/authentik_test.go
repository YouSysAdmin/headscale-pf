package sources

import (
	"testing"

	api "goauthentik.io/api/v3"
)

func ptr[T any](v T) *T { return &v }

func TestToGroup(t *testing.T) {
	t.Run("populates Users with @ suffix and nil-safe email", func(t *testing.T) {
		in := api.Group{
			Pk:   "pk-1",
			Name: "admins",
			UsersObj: []api.PartialUser{
				{Uid: "u1", Username: "alice", Email: ptr("alice@example.com")},
				{Uid: "u2", Username: "bob@corp", Email: ptr("bob@corp.example.com")},
				{Uid: "u3", Username: "carol", Email: nil}, // pre-B3 this would panic
			},
		}

		got := toGroup(in)
		if got.ID != "pk-1" || got.Name != "admins" {
			t.Errorf("identity wrong: %+v", got)
		}
		if len(got.Users) != 3 {
			t.Fatalf("expected 3 users, got %d", len(got.Users))
		}

		// alice: @ appended
		if got.Users[0].Username != "alice@" || got.Users[0].Email != "alice@example.com" {
			t.Errorf("alice mapped wrong: %+v", got.Users[0])
		}
		// bob: already has @, untouched
		if got.Users[1].Username != "bob@corp" {
			t.Errorf("bob double-suffixed: %q", got.Users[1].Username)
		}
		// carol: nil email handled gracefully
		if got.Users[2].Email != "" {
			t.Errorf("carol email should be empty, got %q", got.Users[2].Email)
		}
		if got.Users[2].Username != "carol@" {
			t.Errorf("carol username = %q, want carol@", got.Users[2].Username)
		}
	})

	t.Run("empty group returns non-nil empty Users (B4 contract)", func(t *testing.T) {
		got := toGroup(api.Group{Pk: "pk", Name: "empty", UsersObj: nil})
		if got.Users == nil {
			t.Errorf("Users must be non-nil so prepare.go can use Users == nil as the not-preloaded sentinel")
		}
		if len(got.Users) != 0 {
			t.Errorf("expected empty Users slice, got %v", got.Users)
		}
	})
}
