package sources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// authentikTestServer mimics the Authentik endpoints the adapter uses:
//
//	GET /api/v3/core/groups/?name=X&include_users=true
//	GET /api/v3/core/groups/{uuid}/?include_users=true
//
// groupsByName feeds the list endpoint; groupsByPk feeds the retrieve endpoint.
type authentikTestServer struct {
	groupsByName map[string]akGroup
	groupsByPk   map[string]akGroup
	listCalls    int32
	retrieveCalls int32
	includeUsersSeen bool
}

type akGroup struct {
	Pk    string  `json:"pk"`
	Name  string  `json:"name"`
	Users []akUsr `json:"users_obj"`
}

type akUsr struct {
	Uid      string  `json:"uid"`
	Username string  `json:"username"`
	Email    *string `json:"email,omitempty"`
}

func (s *authentikTestServer) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("include_users") == "true" {
			s.includeUsersSeen = true
		}

		switch {
		case r.URL.Path == "/api/v3/core/groups/":
			atomic.AddInt32(&s.listCalls, 1)
			name := r.URL.Query().Get("name")
			results := []akGroup{}
			if g, ok := s.groupsByName[name]; ok {
				results = append(results, g)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pagination":   map[string]float32{"next": 0, "previous": 0, "count": float32(len(results)), "current": 1, "total_pages": 1, "start_index": 1, "end_index": float32(len(results))},
				"results":      results,
				"autocomplete": map[string]any{},
			})

		case strings.HasPrefix(r.URL.Path, "/api/v3/core/groups/") && strings.HasSuffix(r.URL.Path, "/"):
			atomic.AddInt32(&s.retrieveCalls, 1)
			parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
			pk := parts[len(parts)-1]
			g, ok := s.groupsByPk[pk]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(g)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	})
}

func newAuthentikTestClient(t *testing.T, srv *httptest.Server) *Authentik {
	t.Helper()
	c, err := NewAuthentikClient(SourceConfig{
		Token:                 "test-token",
		Endpoint:              srv.URL,
		InsecureSkipTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("NewAuthentikClient: %v", err)
	}
	return c
}

func sptr(s string) *string { return &s }

func TestAuthentik_GetGroupByName_PopulatesUsersInOneCall(t *testing.T) {
	state := &authentikTestServer{
		groupsByName: map[string]akGroup{
			"engineering": {
				Pk:   "pk-eng",
				Name: "engineering",
				Users: []akUsr{
					{Uid: "u1", Username: "alice", Email: sptr("alice@example.com")},
					{Uid: "u2", Username: "bob@corp", Email: sptr("bob@corp.example.com")},
				},
			},
		},
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newAuthentikTestClient(t, srv)
	g, err := c.GetGroupByName("engineering")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if g == nil {
		t.Fatalf("expected group, got nil")
	}
	if g.ID != "pk-eng" || g.Name != "engineering" {
		t.Errorf("identity wrong: %+v", g)
	}
	if len(g.Users) != 2 {
		t.Fatalf("expected 2 users in returned group (B4 contract: GetGroupByName populates Users), got %d", len(g.Users))
	}
	if g.Users[0].Username != "alice@" {
		t.Errorf("alice should get @ suffix, got %q", g.Users[0].Username)
	}
	if g.Users[1].Username != "bob@corp" {
		t.Errorf("bob should not be double-suffixed, got %q", g.Users[1].Username)
	}

	if state.listCalls != 1 {
		t.Errorf("expected exactly 1 list call (no fallback retrieve), got %d", state.listCalls)
	}
	if state.retrieveCalls != 0 {
		t.Errorf("expected no retrieve calls when GetGroupByName succeeds, got %d", state.retrieveCalls)
	}
	if !state.includeUsersSeen {
		t.Errorf("Authentik adapter must request include_users=true to populate Users in one call")
	}
}

func TestAuthentik_GetGroupByName_NotFound(t *testing.T) {
	state := &authentikTestServer{groupsByName: map[string]akGroup{}}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newAuthentikTestClient(t, srv)
	g, err := c.GetGroupByName("ghost")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if g != nil {
		t.Errorf("expected nil group, got %+v", g)
	}
}

func TestAuthentik_GetGroupByName_NilEmailDoesNotPanic(t *testing.T) {
	state := &authentikTestServer{
		groupsByName: map[string]akGroup{
			"team": {
				Pk:   "pk-team",
				Name: "team",
				Users: []akUsr{
					{Uid: "u1", Username: "noemail", Email: nil}, // pre-B3 → panic on *u.Email
				},
			},
		},
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newAuthentikTestClient(t, srv)
	g, err := c.GetGroupByName("team") // must not panic
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if len(g.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(g.Users))
	}
	if g.Users[0].Email != "" {
		t.Errorf("nil email should map to empty string, got %q", g.Users[0].Email)
	}
}

func TestAuthentik_GetGroupByName_EmptyGroupReturnsNonNilUsers(t *testing.T) {
	// B4 contract: prepare.go relies on Users == nil meaning "not preloaded".
	// An Authentik group with zero members must return Users = []User{}, not nil.
	state := &authentikTestServer{
		groupsByName: map[string]akGroup{
			"empty-team": {Pk: "pk-empty", Name: "empty-team", Users: []akUsr{}},
		},
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newAuthentikTestClient(t, srv)
	g, err := c.GetGroupByName("empty-team")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if g == nil {
		t.Fatalf("expected group, got nil")
	}
	if g.Users == nil {
		t.Errorf("Users must be non-nil empty slice (B4 sentinel contract)")
	}
	if len(g.Users) != 0 {
		t.Errorf("expected zero members, got %d", len(g.Users))
	}
}

func TestAuthentik_GetGroupMembers_FallbackByPK(t *testing.T) {
	state := &authentikTestServer{
		groupsByPk: map[string]akGroup{
			"pk-eng": {
				Pk:   "pk-eng",
				Name: "engineering",
				Users: []akUsr{
					{Uid: "u1", Username: "carol", Email: sptr("carol@example.com")},
				},
			},
		},
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newAuthentikTestClient(t, srv)
	users, err := c.GetGroupMembers("pk-eng")
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(users) != 1 || users[0].Username != "carol@" {
		t.Errorf("retrieve fallback wrong: %+v", users)
	}
	if state.retrieveCalls != 1 {
		t.Errorf("expected one /core/groups/{pk}/ call, got %d", state.retrieveCalls)
	}
}
