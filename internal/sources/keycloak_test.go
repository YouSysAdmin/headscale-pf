package sources

import (
	"testing"

	gocloak "github.com/Nerzal/gocloak/v13"
)

func TestToModelUser(t *testing.T) {
	cases := []struct {
		name         string
		in           *gocloak.User
		wantID       string
		wantEmail    string
		wantUsername string
	}{
		{
			name: "all fields populated",
			in: &gocloak.User{
				ID:       gocloak.StringP("id-1"),
				Email:    gocloak.StringP("alice@example.com"),
				Username: gocloak.StringP("alice"),
			},
			wantID:       "id-1",
			wantEmail:    "alice@example.com",
			wantUsername: "alice@",
		},
		{
			name: "username already contains @ — no double suffix",
			in: &gocloak.User{
				ID:       gocloak.StringP("id-2"),
				Email:    gocloak.StringP("bob@example.com"),
				Username: gocloak.StringP("bob@example.com"),
			},
			wantID:       "id-2",
			wantEmail:    "bob@example.com",
			wantUsername: "bob@example.com",
		},
		{
			name: "nil email and nil id are safe (gocloak.PString returns \"\")",
			in: &gocloak.User{
				Username: gocloak.StringP("carol"),
			},
			wantID:       "",
			wantEmail:    "",
			wantUsername: "carol@",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toModelUser(tc.in)
			if got.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tc.wantID)
			}
			if got.Email != tc.wantEmail {
				t.Errorf("Email = %q, want %q", got.Email, tc.wantEmail)
			}
			if got.Username != tc.wantUsername {
				t.Errorf("Username = %q, want %q", got.Username, tc.wantUsername)
			}
		})
	}
}
