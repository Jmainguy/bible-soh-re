package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type socialFixture struct {
	db           *Database
	h            *AuthHandler
	group, other *StudyGroup
}

func newSocialFixture(t *testing.T) socialFixture {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db")
	if pg := os.Getenv("TEST_DATABASE_URL"); pg != "" {
		admin, err := sql.Open("postgres", pg)
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
		if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); _ = admin.Close() })
		u, _ := url.Parse(pg)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	db, err := NewDatabase(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for id := 1; id <= 3; id++ {
		_, err = db.db.Exec("INSERT INTO users(id,email,username,first_name,last_name) VALUES(?,?,?,?,?)", id, fmt.Sprintf("user%d@example.test", id), fmt.Sprintf("user%d", id), "Test", "User")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.CreateSession(int64(id), fmt.Sprintf("session%d", id), time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	group, err := db.CreateStudyGroup("Study group", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateStudyGroup("Other group", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.db.Exec("INSERT INTO group_members(group_id,user_id,role) VALUES(?,2,'member')", group.ID)
	if err != nil {
		t.Fatal(err)
	}
	return socialFixture{db, NewAuthHandler(db, &AuthConfig{}), group, other}
}
func requestAs(t *testing.T, handler http.HandlerFunc, user int, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if user > 0 {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: fmt.Sprintf("session%d", user)})
	}
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != status {
		t.Fatalf("%s %s user%d: got %d want %d: %s", method, path, user, w.Code, status, w.Body.String())
	}
	return w
}
func TestSocialPrivacyAndReplies(t *testing.T) {
	f := newSocialFixture(t)
	for user := 1; user <= 2; user++ {
		requestAs(t, f.h.handleCreateVerseComment, user, "POST", "/api/verse-comments/create", `{"book":"Genesis","chapter":1,"verse":1,"content":"personal"}`, 200)
	}
	w := requestAs(t, f.h.handleGetVerseComments, 1, "GET", "/api/verse-comments/list?book=Genesis&chapter=1&verse=1", "", 200)
	var comments []VerseComment
	if err := json.Unmarshal(w.Body.Bytes(), &comments); err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].UserID != 1 {
		t.Fatalf("personal comments leaked: %s", w.Body.String())
	}
	requestAs(t, f.h.handleCreateVerseComment, 2, "POST", "/api/verse-comments/create", fmt.Sprintf(`{"book":"Genesis","chapter":1,"verse":1,"parent_id":%d,"content":"reply"}`, comments[0].ID), 400)
	note, err := f.db.CreateNote(1, sql.NullInt64{Int64: f.group.ID, Valid: true}, "Genesis", 1, "group note")
	if err != nil {
		t.Fatal(err)
	}
	requestAs(t, f.h.handleGetNotes, 3, "GET", fmt.Sprintf("/api/notes/group?group_id=%d&book=Genesis&chapter=1", f.group.ID), "", 403)
	requestAs(t, f.h.handleCreateComment, 2, "POST", "/api/comments/create", fmt.Sprintf(`{"noteId":%d,"content":"encouragement"}`, note.ID), 200)
	requestAs(t, f.h.handleToggleReaction, 3, "POST", "/api/reactions/toggle", fmt.Sprintf(`{"target_type":"note","target_id":%d,"emoji":"🙏"}`, note.ID), 403)
	requestAs(t, f.h.handleToggleReaction, 2, "POST", "/api/reactions/toggle", fmt.Sprintf(`{"target_type":"note","target_id":%d,"emoji":"🙏"}`, note.ID), 200)
	requestAs(t, f.h.handleGetReactionsSummary, 3, "GET", fmt.Sprintf("/api/reactions/summary?target_type=note&target_id=%d", note.ID), "", 403)
	requestAs(t, f.h.handleToggleReaction, 2, "POST", "/api/reactions/toggle", fmt.Sprintf(`{"target_type":"note","target_id":%d,"emoji":"'x'"}`, note.ID), 400)
	otherNote, _ := f.db.CreateNote(2, sql.NullInt64{}, "Genesis", 1, "mine")
	parent, _ := f.db.CreateNoteComment(note.ID, 1, sql.NullInt64{}, "parent")
	requestAs(t, f.h.handleCreateComment, 2, "POST", "/api/comments/create", fmt.Sprintf(`{"noteId":%d,"parentId":%d,"content":"wrong thread"}`, otherNote.ID, parent.ID), 400)
	child, _ := f.db.CreateNoteComment(note.ID, 2, sql.NullInt64{Int64: parent.ID, Valid: true}, "child")
	if err := f.db.DeleteNoteComment(parent.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.GetNoteComment(child.ID); err == nil {
		t.Fatal("reply orphaned after parent deletion")
	}
	scope, _ := f.db.socialScope("note", note.ID)
	if !f.db.canAccessScope(scope, 2) || f.db.canAccessScope(scope, 3) {
		t.Fatal("incorrect live update audience")
	}
}
func TestStudyPlanLifecycleAndAuthorization(t *testing.T) {
	f := newSocialFixture(t)
	body := fmt.Sprintf(`{"group_id":%d,"week_number":1,"start_date":"2026-09-22","end_date":"2026-09-29","book":"Genesis","start_chapter":1,"end_chapter":3}`, f.group.ID)
	requestAs(t, f.h.handleCreateStudyPlan, 2, "POST", "/api/study-plans/create", body, 403)
	w := requestAs(t, f.h.handleCreateStudyPlan, 1, "POST", "/api/study-plans/create", body, 200)
	var plan StudyPlan
	if err := json.Unmarshal(w.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	forged := fmt.Sprintf(`{"id":%d,"group_id":%d,"week_number":2,"start_date":"2026-09-22","end_date":"2026-09-29","book":"Genesis","start_chapter":1,"end_chapter":3}`, plan.ID, f.other.ID)
	requestAs(t, f.h.handleUpdateStudyPlan, 3, "PUT", "/api/study-plans/update", forged, 403)
	requestAs(t, f.h.handleDeleteStudyPlan, 3, "DELETE", "/api/study-plans/delete", forged, 403)
	requestAs(t, f.h.handleUpdateStudyPlan, 1, "PUT", "/api/study-plans/update", strings.ReplaceAll(forged, `"end_chapter":3`, `"end_chapter":999`), 400)
	requestAs(t, f.h.handleUpdateStudyPlan, 1, "PUT", "/api/study-plans/update", forged, 200)
	requestAs(t, f.h.handleDeleteStudyPlan, 1, "DELETE", "/api/study-plans/delete", forged, 200)
	if _, err := f.db.GetStudyPlan(plan.ID); err == nil {
		t.Fatal("plan not deleted")
	}
}
func TestInviteLifecycleAndPrayers(t *testing.T) {
	f := newSocialFixture(t)
	path := fmt.Sprintf("/api/groups/%d/invite", f.group.ID)
	requestAs(t, f.h.handleGroupsRESTful, 3, "POST", path, `{"username":"user3@example.test"}`, 403)
	requestAs(t, f.h.handleGroupsRESTful, 2, "POST", path, `{"username":"user3@example.test"}`, 403)
	w := requestAs(t, f.h.handleGroupsRESTful, 1, "POST", path, `{"username":"user3@example.test"}`, 200)
	var invite GroupInvite
	if err := json.Unmarshal(w.Body.Bytes(), &invite); err != nil {
		t.Fatal(err)
	}
	accept := fmt.Sprintf("/api/groups/invites/%d/accept", invite.ID)
	requestAs(t, f.h.handleGroupsRESTful, 2, "POST", accept, "", 403)
	requestAs(t, f.h.handleGroupsRESTful, 3, "POST", accept, "", 200)
	member, _ := f.db.IsGroupMember(f.group.ID, 3)
	if !member {
		t.Fatal("invite did not join group")
	}
	requestAs(t, f.h.handleGroupsRESTful, 1, "POST", path, `{"username":"future@example.test"}`, 200)
	w = requestAs(t, f.h.handleCreatePrayerRequest, 2, "POST", "/api/prayers/create", fmt.Sprintf(`{"group_id":%d,"title":"Prayer","content":"Please pray"}`, f.group.ID), 200)
	var prayer PrayerRequest
	if err := json.Unmarshal(w.Body.Bytes(), &prayer); err != nil {
		t.Fatal(err)
	}
	requestAs(t, f.h.handlePrayersRESTful, 2, "POST", fmt.Sprintf("/api/prayers/%d/comments", prayer.ID), `{"content":"Praying"}`, 200)
	requestAs(t, f.h.handlePrayersRESTful, 2, "PATCH", fmt.Sprintf("/api/prayers/%d/status", prayer.ID), `{"status":"answered","answer_explanation":"Thank you"}`, 200)
	requestAs(t, f.h.handlePrayersRESTful, 2, "POST", fmt.Sprintf("/api/prayers/%d/archive", prayer.ID), "", 200)
	requestAs(t, f.h.handlePrayersRESTful, 1, "POST", fmt.Sprintf("/api/prayers/%d/restore", prayer.ID), "", 200)
	requestAs(t, f.h.HandleWebSocket, 0, "GET", "/ws", "", 401)
}

func TestWebSocketPrivateAudience(t *testing.T) {
	f := newSocialFixture(t)
	oldHub := hub
	live := &Hub{clients: make(map[*Client]bool), broadcast: make(chan BroadcastMessage, 16), register: make(chan *Client), unregister: make(chan *Client), stop: make(chan struct{})}
	hub = live
	done := make(chan struct{})
	go func() { live.Run(); close(done) }()
	server := httptest.NewServer(http.HandlerFunc(f.h.HandleWebSocket))
	defer server.Close()
	defer func() { close(live.stop); <-done; hub = oldHub }()
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	clients := make([]*websocket.Conn, 0, 3)
	for user := 1; user <= 3; user++ {
		headers := http.Header{"Cookie": []string{fmt.Sprintf("%s=session%d", sessionCookieName, user)}}
		conn, _, err := websocket.DefaultDialer.Dial(url, headers)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, conn)
	}
	defer func() {
		for _, conn := range clients {
			_ = conn.Close()
		}
	}()
	// Ensure all registrations have been processed before enqueueing an event.
	deadline := time.Now().Add(time.Second)
	for {
		live.mu.RLock()
		n := len(live.clients)
		live.mu.RUnlock()
		if n == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("clients not registered")
		}
		time.Sleep(time.Millisecond)
	}
	BroadcastUpdate(BroadcastMessage{Type: "prayer", Action: "update", Scope: socialScope{GroupID: sql.NullInt64{Int64: f.group.ID, Valid: true}}})
	for _, conn := range clients[:2] {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		var msg BroadcastMessage
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != "prayer" {
			t.Fatal("wrong message")
		}
	}
	_ = clients[2].SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := clients[2].ReadMessage(); err == nil {
		t.Fatal("outsider received private event")
	}
	headers := http.Header{"Cookie": []string{sessionCookieName + "=session1"}, "Origin": []string{"https://unrelated.example"}}
	if conn, response, err := websocket.DefaultDialer.Dial(url, headers); err == nil {
		_ = conn.Close()
		t.Fatal("cross-origin socket accepted")
	} else if response.StatusCode != 403 {
		t.Fatalf("expected origin rejection, got %d", response.StatusCode)
	}
}

func TestPostgresLiveUpdates(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration test")
	}
	f := newSocialFixture(t)
	originalHub := hub
	originalPublish := publishLive
	hub = &Hub{broadcast: make(chan BroadcastMessage, 4)}
	t.Cleanup(func() { hub = originalHub; publishLive = originalPublish })
	stop, err := f.db.startLiveUpdates(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	BroadcastUpdate(BroadcastMessage{Type: "prayer", Action: "update", Scope: socialScope{GroupID: sql.NullInt64{Int64: f.group.ID, Valid: true}, UserID: 1}})
	select {
	case msg := <-hub.broadcast:
		if msg.Type != "prayer" || msg.Scope.GroupID.Int64 != f.group.ID {
			t.Fatalf("invalid event: %+v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no PostgreSQL notification received")
	}
}
