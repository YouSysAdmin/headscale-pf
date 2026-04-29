package sources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// keycloakTestServer mocks the two Keycloak admin REST endpoints the
// adapter uses:
//
//	GET /admin/realms/{realm}/groups?search={name}
//	GET /admin/realms/{realm}/groups/{groupID}/members?first=N&max=N
type keycloakTestServer struct {
	groups        []map[string]string         // {"id","name"} — name-search returns substring matches
	members       map[string][]map[string]any // groupID -> users (in order)
	memberCalls   int32
	failsBeforeOK int32 // /members returns 500 this many times before succeeding
}

func (s *keycloakTestServer) handler(t *testing.T, realm string) http.Handler {
	t.Helper()
	prefix := "/admin/realms/" + realm + "/groups"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == prefix:
			search := r.URL.Query().Get("search")
			out := []map[string]string{}
			for _, g := range s.groups {
				if search == "" || strings.Contains(g["name"], search) {
					out = append(out, g)
				}
			}
			_ = json.NewEncoder(w).Encode(out)

		case strings.HasPrefix(r.URL.Path, prefix+"/") && strings.HasSuffix(r.URL.Path, "/members"):
			atomic.AddInt32(&s.memberCalls, 1)

			if remaining := atomic.LoadInt32(&s.failsBeforeOK); remaining > 0 {
				atomic.AddInt32(&s.failsBeforeOK, -1)
				http.Error(w, "transient", http.StatusInternalServerError)
				return
			}

			parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix+"/"), "/")
			groupID := parts[0]

			first, _ := strconv.Atoi(r.URL.Query().Get("first"))
			max, _ := strconv.Atoi(r.URL.Query().Get("max"))
			if max == 0 {
				max = 200
			}

			users := s.members[groupID]
			lo := first
			if lo > len(users) {
				lo = len(users)
			}
			hi := lo + max
			if hi > len(users) {
				hi = len(users)
			}
			_ = json.NewEncoder(w).Encode(users[lo:hi])

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	})
}

func newKeycloakTestClient(t *testing.T, srv *httptest.Server, realm string) *Keycloak {
	t.Helper()
	c, err := NewKeycloakClient(SourceConfig{
		Token:         "test-token",
		Endpoint:      srv.URL,
		KeycloakRealm: realm,
	})
	if err != nil {
		t.Fatalf("NewKeycloakClient: %v", err)
	}
	return c
}

func mkKCUser(id, username, email string) map[string]any {
	return map[string]any{"id": id, "username": username, "email": email}
}

func mkKCUsers(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		id := strconv.Itoa(i)
		out[i] = mkKCUser("u"+id, "user"+id, "user"+id+"@example.com")
	}
	return out
}

func TestKeycloak_GetGroupByName_Found(t *testing.T) {
	state := &keycloakTestServer{
		groups: []map[string]string{
			{"id": "kc-eng", "name": "engineering"},
		},
	}
	srv := httptest.NewServer(state.handler(t, "myrealm"))
	defer srv.Close()

	c := newKeycloakTestClient(t, srv, "myrealm")
	g, err := c.GetGroupByName("engineering")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if g == nil || g.ID != "kc-eng" || g.Name != "engineering" {
		t.Errorf("group wrong: %+v", g)
	}
}

func TestKeycloak_GetGroupByName_SearchSubstringMatchRejected(t *testing.T) {
	// Keycloak's `search` is substring; the adapter must filter to exact-name
	// matches so "eng" does not return "engineering" as if it matched.
	state := &keycloakTestServer{
		groups: []map[string]string{
			{"id": "kc-eng", "name": "engineering"},
		},
	}
	srv := httptest.NewServer(state.handler(t, "myrealm"))
	defer srv.Close()

	c := newKeycloakTestClient(t, srv, "myrealm")
	g, err := c.GetGroupByName("eng")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if g != nil {
		t.Errorf("substring match should be rejected; got %+v", g)
	}
}

func TestKeycloak_GetGroupByName_NotFound(t *testing.T) {
	state := &keycloakTestServer{groups: []map[string]string{}}
	srv := httptest.NewServer(state.handler(t, "myrealm"))
	defer srv.Close()

	c := newKeycloakTestClient(t, srv, "myrealm")
	g, err := c.GetGroupByName("ghost")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if g != nil {
		t.Errorf("expected nil, got %+v", g)
	}
}

func TestKeycloak_GetGroupMembers_PaginatesAndDedups(t *testing.T) {
	groupID := "kc-eng"
	// 350 users → 2 full pages of 200 + 1 partial (page size is 200 in adapter).
	users := mkKCUsers(350)
	state := &keycloakTestServer{
		members: map[string][]map[string]any{groupID: users},
	}
	srv := httptest.NewServer(state.handler(t, "myrealm"))
	defer srv.Close()

	c := newKeycloakTestClient(t, srv, "myrealm")
	got, err := c.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(got) != 350 {
		t.Errorf("expected 350 users, got %d", len(got))
	}
	// adapter pageSize=200; 350 users → page 1 (200), page 2 (150 < 200 → break).
	if state.memberCalls < 2 {
		t.Errorf("expected at least 2 paginated calls, got %d", state.memberCalls)
	}

	// Order is preserved.
	for i, u := range got {
		want := "user" + strconv.Itoa(i) + "@"
		if u.Username != want {
			t.Errorf("user[%d].Username = %q, want %q", i, u.Username, want)
			break
		}
	}
}

func TestKeycloak_GetGroupMembers_RetriesOnTransient500(t *testing.T) {
	// The adapter's fetchGroupMembersPage retries up to 5 times with backoff.
	// First two calls return 500; the third must succeed.
	groupID := "kc-eng"
	state := &keycloakTestServer{
		members:       map[string][]map[string]any{groupID: mkKCUsers(3)},
		failsBeforeOK: 2,
	}
	srv := httptest.NewServer(state.handler(t, "myrealm"))
	defer srv.Close()

	c := newKeycloakTestClient(t, srv, "myrealm")
	got, err := c.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers should retry past transient failures: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 users after retry, got %d", len(got))
	}
}

func TestKeycloak_GetGroupMembers_EmptyGroup(t *testing.T) {
	state := &keycloakTestServer{members: map[string][]map[string]any{"kc-empty": nil}}
	srv := httptest.NewServer(state.handler(t, "myrealm"))
	defer srv.Close()

	c := newKeycloakTestClient(t, srv, "myrealm")
	got, err := c.GetGroupMembers("kc-empty")
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if got == nil {
		t.Errorf("empty group should return non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 users, got %d", len(got))
	}
}
