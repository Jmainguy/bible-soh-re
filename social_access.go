package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// socialScope is kept server-side; personal content belongs only to its owner.
type socialScope struct {
	GroupID sql.NullInt64
	UserID  int64
}

func (d *Database) socialScope(kind string, id int64) (socialScope, error) {
	var scope socialScope
	queries := map[string]string{
		"note":           "SELECT group_id, user_id FROM notes WHERE id = ?",
		"note_comment":   "SELECT n.group_id, n.user_id FROM notes n JOIN note_comments c ON c.note_id=n.id WHERE c.id = ?",
		"verse_comment":  "SELECT group_id, user_id FROM verse_comments WHERE id = ?",
		"prayer_request": "SELECT group_id, user_id FROM prayer_requests WHERE id = ?",
		"prayer_comment": "SELECT p.group_id, p.user_id FROM prayer_requests p JOIN prayer_comments c ON c.prayer_id=p.id WHERE c.id = ?",
	}
	query, ok := queries[kind]
	if !ok || id < 1 {
		return scope, fmt.Errorf("invalid target")
	}
	err := d.db.QueryRow(query, id).Scan(&scope.GroupID, &scope.UserID)
	return scope, err
}
func (d *Database) canAccessScope(scope socialScope, userID int64) bool {
	if scope.GroupID.Valid {
		member, err := d.IsGroupMember(scope.GroupID.Int64, userID)
		return err == nil && member
	}
	return scope.UserID != 0 && scope.UserID == userID
}
func (h *AuthHandler) requireSocialAccess(w http.ResponseWriter, kind string, id, userID int64) (socialScope, bool) {
	scope, err := h.db.socialScope(kind, id)
	if err != nil {
		respondError(w, "Content not found", http.StatusNotFound)
		return scope, false
	}
	if !h.db.canAccessScope(scope, userID) {
		respondError(w, "Not authorized", http.StatusForbidden)
		return scope, false
	}
	return scope, true
}
func validStudyPlan(week int, start, end, book string, first, last int) bool {
	a, e1 := time.Parse("2006-01-02", start)
	b, e2 := time.Parse("2006-01-02", end)
	if week < 1 || e1 != nil || e2 != nil || b.Before(a) || first < 1 || last < first {
		return false
	}
	for _, books := range [][]Book{kjvOT, kjvNT} {
		for _, candidate := range books {
			if candidate.Name == strings.TrimSpace(book) {
				return last <= len(candidate.ChapterLengths)
			}
		}
	}
	return false
}
