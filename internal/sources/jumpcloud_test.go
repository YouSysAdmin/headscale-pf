package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	jcapiv1 "github.com/TheJumpCloud/jcapi-go/v1"
	jcapiv2 "github.com/TheJumpCloud/jcapi-go/v2"
)

// jcTestServer mimics the two JumpCloud endpoints the adapter relies on:
//
//	GET /usergroups                       (used by GetGroupByName, optional)
//	GET /usergroups/{group_id}/membership (used by GetGroupMembers, paginated)
//	GET /systemusers/{id}                 (used by getUserInfo, per-user fetch)
//
// Tests configure groupsByName and members per group, then read counters off
// the returned struct to assert pagination/concurrency behavior.
type jcTestServer struct {
	groupsByName map[string]string            // name -> group ID
	members      map[string][]string          // group ID -> user IDs (in order)
	users        map[string]map[string]string // user ID -> {"username","email"} fields
	failUserID   string                       // if non-empty, /systemusers/{id} for this ID returns 500
	memberCalls  int32                        // count of membership requests
	userCalls    int32                        // count of /systemusers/{id} requests
	queryLog     []string                     // captured raw query strings, for assertion
}

func (s *jcTestServer) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/usergroups":
			name := r.URL.Query().Get("filter")
			out := []map[string]string{}
			for groupName, id := range s.groupsByName {
				if name == "" || strings.Contains(name, groupName) {
					out = append(out, map[string]string{"id": id, "name": groupName, "type": "user_group"})
				}
			}
			_ = json.NewEncoder(w).Encode(out)

		case strings.HasPrefix(r.URL.Path, "/usergroups/") && strings.HasSuffix(r.URL.Path, "/membership"):
			atomic.AddInt32(&s.memberCalls, 1)
			s.queryLog = append(s.queryLog, r.URL.RawQuery)

			parts := strings.Split(r.URL.Path, "/")
			groupID := parts[2]

			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 100
			}
			skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))

			ids := s.members[groupID]
			lo := skip
			if lo > len(ids) {
				lo = len(ids)
			}
			hi := lo + limit
			if hi > len(ids) {
				hi = len(ids)
			}
			page := ids[lo:hi]

			out := make([]map[string]any, 0, len(page))
			for _, id := range page {
				out = append(out, map[string]any{"id": id, "paths": [][]any{}, "type": "user"})
			}
			_ = json.NewEncoder(w).Encode(out)

		case strings.HasPrefix(r.URL.Path, "/systemusers/"):
			atomic.AddInt32(&s.userCalls, 1)
			id := strings.TrimPrefix(r.URL.Path, "/systemusers/")
			if id == s.failUserID {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			u, ok := s.users[id]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"_id":      id,
				"username": u["username"],
				"email":    u["email"],
			})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	})
}

func newJCTestClient(t *testing.T, srv *httptest.Server) *Jumpcloud {
	t.Helper()
	cfgV1 := jcapiv1.NewConfiguration()
	cfgV1.BasePath = srv.URL
	cfgV2 := jcapiv2.NewConfiguration()
	cfgV2.BasePath = srv.URL
	return &Jumpcloud{
		V1:          jcapiv1.NewAPIClient(cfgV1),
		V1Auth:      context.Background(),
		V2:          jcapiv2.NewAPIClient(cfgV2),
		V2Auth:      context.Background(),
		ContentType: "application/json",
	}
}

func mkUsers(ids ...string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(ids))
	for _, id := range ids {
		out[id] = map[string]string{
			"username": id, // bare login; adapter must add @ suffix
			"email":    id + "@example.com",
		}
	}
	return out
}

// ids generates ["u0", "u1", ...] for compact pagination test setup.
func ids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("u%d", i)
	}
	return out
}

func TestJumpcloud_GetGroupMembers_Pagination(t *testing.T) {
	groupID := "grp-1"
	allIDs := ids(150) // 1.5 pages at pageSize=100

	state := &jcTestServer{
		members: map[string][]string{groupID: allIDs},
		users:   mkUsers(allIDs...),
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	got, err := c.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(got) != 150 {
		t.Errorf("expected 150 users, got %d", len(got))
	}
	if state.memberCalls != 2 {
		t.Errorf("expected 2 membership pages (skip=0, skip=100), got %d", state.memberCalls)
	}
	if state.userCalls != 150 {
		t.Errorf("expected one /systemusers/* per ID, got %d", state.userCalls)
	}

	// Order must match the membership listing (parallel pool preserves order via index).
	for i, u := range got {
		want := fmt.Sprintf("u%d@", i)
		if u.Username != want {
			t.Errorf("user[%d].Username = %q, want %q", i, u.Username, want)
		}
	}
}

func TestJumpcloud_GetGroupMembers_DedupsAcrossPages(t *testing.T) {
	groupID := "grp-1"
	// page 1 has u0..u99; page 2 returns u99 again (overlap) plus u100.
	page1 := ids(100)
	page2 := []string{"u99", "u100"}
	combined := append(page1, page2...)

	state := &jcTestServer{
		members: map[string][]string{groupID: combined},
		users:   mkUsers(append(ids(100), "u100")...),
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	got, err := c.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}

	// 101 distinct IDs (u0..u100); u99 should appear exactly once.
	if len(got) != 101 {
		t.Errorf("expected 101 unique users after dedup, got %d", len(got))
	}
	if state.userCalls != 101 {
		t.Errorf("expected 101 user fetches (no duplicates), got %d", state.userCalls)
	}
}

func TestJumpcloud_GetGroupMembers_EmptyGroup(t *testing.T) {
	groupID := "grp-empty"
	state := &jcTestServer{members: map[string][]string{groupID: nil}}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	got, err := c.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if got == nil {
		t.Errorf("empty group must return non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 users, got %d", len(got))
	}
	if state.userCalls != 0 {
		t.Errorf("no per-user calls expected for empty group, got %d", state.userCalls)
	}
}

func TestJumpcloud_GetGroupMembers_PerUserErrorShortCircuits(t *testing.T) {
	groupID := "grp-1"
	memberIDs := ids(20)

	state := &jcTestServer{
		members:    map[string][]string{groupID: memberIDs},
		users:      mkUsers(memberIDs...),
		failUserID: "u5", // worker fetching u5 will get a 500
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	_, err := c.GetGroupMembers(groupID)
	if err == nil {
		t.Fatalf("expected error from /systemusers/u5 failure")
	}
}

func TestJumpcloud_GetGroupMembers_ExactPageSizeBoundary(t *testing.T) {
	// Exactly 100 users — a naive paginator that always re-queries while
	// page_size == limit could loop forever. Verify we stop after one call.
	groupID := "grp-1"
	memberIDs := ids(100)

	state := &jcTestServer{
		members: map[string][]string{groupID: memberIDs},
		users:   mkUsers(memberIDs...),
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	got, err := c.GetGroupMembers(groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers: %v", err)
	}
	if len(got) != 100 {
		t.Errorf("expected 100 users, got %d", len(got))
	}
	// Implementation requests page 1 (100 users), page 2 (0 users → break).
	// Either 1 or 2 calls is acceptable; >2 indicates a paging bug.
	if state.memberCalls > 2 {
		t.Errorf("too many membership calls for exact-pageSize boundary: %d", state.memberCalls)
	}
}

func TestJumpcloud_GetGroupByName_Found(t *testing.T) {
	state := &jcTestServer{
		groupsByName: map[string]string{"engineering": "grp-eng"},
	}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	got, err := c.GetGroupByName("engineering")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if got == nil {
		t.Fatalf("expected group, got nil")
	}
	if got.ID != "grp-eng" || got.Name != "engineering" {
		t.Errorf("group fields wrong: %+v", got)
	}
}

func TestJumpcloud_GetGroupByName_NotFound(t *testing.T) {
	state := &jcTestServer{groupsByName: map[string]string{}}
	srv := httptest.NewServer(state.handler(t))
	defer srv.Close()

	c := newJCTestClient(t, srv)
	got, err := c.GetGroupByName("ghost")
	if err != nil {
		t.Fatalf("GetGroupByName: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing group, got %+v", got)
	}
}
