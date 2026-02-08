package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ============ DRY Helper Functions ============

// respondJSON sends a JSON response with proper error handling
func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// respondSuccess sends a simple success JSON response
func respondSuccess(w http.ResponseWriter) {
	respondJSON(w, map[string]interface{}{"success": true})
}

// respondError sends an HTTP error response
func respondError(w http.ResponseWriter, message string, code int) {
	http.Error(w, message, code)
}

// requireMethod checks if the request method matches, returns true if valid
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// requireUser gets the current user or sends an error response
// Returns nil if authentication failed
func (h *AuthHandler) requireUser(w http.ResponseWriter, r *http.Request) *User {
	user, err := h.getCurrentUser(r)
	if err != nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return nil
	}
	return user
}

// decodeJSONBody decodes JSON request body or sends error
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

// parseIDParam parses an int64 ID from string or sends error
func parseIDParam(w http.ResponseWriter, idStr, fieldName string) (int64, bool) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, "Invalid "+fieldName, http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// requireGroupMember checks if user is a member of the group
func (h *AuthHandler) requireGroupMember(w http.ResponseWriter, groupID, userID int64) bool {
	isMember, err := h.db.IsGroupMember(groupID, userID)
	if err != nil {
		respondError(w, "Failed to check membership", http.StatusInternalServerError)
		return false
	}
	if !isMember {
		respondError(w, "Not a member of this group", http.StatusForbidden)
		return false
	}
	return true
}

// requireGroupAdmin checks if user is an admin of the group
func (h *AuthHandler) requireGroupAdmin(w http.ResponseWriter, groupID, userID int64) bool {
	isAdmin, err := h.db.IsGroupAdmin(groupID, userID)
	if err != nil || !isAdmin {
		respondError(w, "Not authorized to perform this action", http.StatusForbidden)
		return false
	}
	return true
}

// requireNoteAccess verifies user has access to a note (owner or group member)
func (h *AuthHandler) requireNoteAccess(w http.ResponseWriter, note *Note, userID int64) bool {
	if note.GroupID.Valid {
		return h.requireGroupMember(w, note.GroupID.Int64, userID)
	}
	if note.UserID != userID {
		respondError(w, "Not authorized", http.StatusForbidden)
		return false
	}
	return true
}

// getNoteAndCheckAccess gets a note and verifies user has access to it
func (h *AuthHandler) getNoteAndCheckAccess(w http.ResponseWriter, noteID, userID int64) (*Note, bool) {
	note, err := h.db.GetNote(noteID)
	if err != nil {
		respondError(w, "Note not found", http.StatusNotFound)
		return nil, false
	}
	if !h.requireNoteAccess(w, note, userID) {
		return nil, false
	}
	return note, true
}

// getInviteAndVerifyUser gets an invite and verifies it belongs to the user
func (h *AuthHandler) getInviteAndVerifyUser(w http.ResponseWriter, inviteID int64, userEmail string) (*GroupInvite, bool) {
	invite, err := h.db.GetGroupInvite(inviteID)
	if err != nil {
		respondError(w, "Invite not found", http.StatusNotFound)
		return nil, false
	}
	if invite.Email != userEmail {
		respondError(w, "Invite not for this user", http.StatusForbidden)
		return nil, false
	}
	return invite, true
}

// handleInviteAction processes accept/decline invite actions with common logic
func (h *AuthHandler) handleInviteAction(w http.ResponseWriter, r *http.Request, inviteID int64, action func(int64, int64) error) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	_, ok := h.getInviteAndVerifyUser(w, inviteID, user.Email)
	if !ok {
		return
	}

	if err := action(inviteID, user.ID); err != nil {
		log.Printf("Failed to process invite: %v", err)
		respondError(w, "Failed to process invite", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// ============ Handler Functions ============

// handleCreateGroup creates a new study group
func (h *AuthHandler) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Name         string   `json:"name"`
		LogoURL      string   `json:"logo_url"`
		InviteEmails []string `json:"invite_emails"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Name == "" {
		respondError(w, "Group name is required", http.StatusBadRequest)
		return
	}

	group, err := h.db.CreateStudyGroup(req.Name, req.LogoURL, user.ID)
	if err != nil {
		log.Printf("Failed to create group: %v", err)
		respondError(w, "Failed to create group", http.StatusInternalServerError)
		return
	}

	// Send invites to all specified emails
	if len(req.InviteEmails) > 0 {
		for _, email := range req.InviteEmails {
			if email == "" || email == user.Email {
				continue // Skip empty or self-invites
			}

			// Generate invite token
			token, err := generateSecureToken()
			if err != nil {
				log.Printf("Failed to generate token: %v", err)
				continue
			}
			_, err = h.db.CreateGroupInvite(group.ID, user.ID, email, token)
			if err != nil {
				log.Printf("Failed to create invite for %s: %v", email, err)
				// Continue with other invites even if one fails
			}
		}
	}

	respondJSON(w, group)
}

// handleGetUserGroups returns all groups for the current user
func (h *AuthHandler) handleGetUserGroups(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	groups, err := h.db.GetUserGroups(user.ID)
	if err != nil {
		log.Printf("Failed to get user groups: %v", err)
		respondError(w, "Failed to get groups", http.StatusInternalServerError)
		return
	}

	respondJSON(w, groups)
}

// handleGetGroupDetails returns group details including members
// DEPRECATED: Use handleGetGroupDetailsByID with RESTful routing instead
/*
func (h *AuthHandler) handleGetGroupDetails(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	groupIDStr := r.URL.Query().Get("groupId")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	// Check if user is a member
	isMember, err := h.db.IsGroupMember(groupID, user.ID)
	if err != nil {
		http.Error(w, "Failed to check membership", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "Not a member of this group", http.StatusForbidden)
		return
	}

	group, err := h.db.GetStudyGroup(groupID)
	if err != nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	members, err := h.db.GetGroupMembers(groupID)
	if err != nil {
		log.Printf("Failed to get group members: %v", err)
		http.Error(w, "Failed to get members", http.StatusInternalServerError)
		return
	}

	isAdmin, _ := h.db.IsGroupAdmin(groupID, user.ID)

	response := map[string]interface{}{
		"group":   group,
		"members": members,
		"isAdmin": isAdmin,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
*/

// handleInviteToGroup creates an invitation to a group
// DEPRECATED: Use handleInviteToGroupByID with RESTful routing instead
/*
func (h *AuthHandler) handleInviteToGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		GroupID int64  `json:"groupId"`
		Email   string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Check if user is admin of the group
	isAdmin, err := h.db.IsGroupAdmin(req.GroupID, user.ID)
	if err != nil || !isAdmin {
		http.Error(w, "Not authorized to invite members", http.StatusForbidden)
		return
	}

	// Generate invite token
	token, err := generateSecureToken()
	if err != nil {
		http.Error(w, "Failed to generate invite token", http.StatusInternalServerError)
		return
	}

	invite, err := h.db.CreateGroupInvite(req.GroupID, user.ID, req.Email, token)
	if err != nil {
		log.Printf("Failed to create invite: %v", err)
		http.Error(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	// In a real app, send email with invite link
	inviteLink := h.config.BaseURL + "/invite/" + token

	response := map[string]interface{}{
		"invite":     invite,
		"inviteLink": inviteLink,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
*/

// handleAcceptInvite accepts a group invitation
// DEPRECATED: Use handleAcceptInviteByID with RESTful routing instead
/*
func (h *AuthHandler) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	invite, err := h.db.GetGroupInviteByToken(req.Token)
	if err != nil {
		http.Error(w, "Invalid or expired invite", http.StatusNotFound)
		return
	}

	if err := h.db.AcceptGroupInvite(invite.ID, user.ID); err != nil {
		log.Printf("Failed to accept invite: %v", err)
		http.Error(w, "Failed to accept invite", http.StatusInternalServerError)
		return
	}

	group, _ := h.db.GetStudyGroup(invite.GroupID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"group":   group,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
*/

// handleGetPendingInvites returns all pending invites for current user
func (h *AuthHandler) handleGetPendingInvites(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	invites, err := h.db.GetPendingInvites(user.Email)
	if err != nil {
		log.Printf("Failed to get pending invites: %v", err)
		respondError(w, "Failed to get invites", http.StatusInternalServerError)
		return
	}

	respondJSON(w, invites)
}

// handleLeaveGroup allows a user to leave a group
// DEPRECATED: Not currently used, can be removed or uncommented if needed
/*
func (h *AuthHandler) handleLeaveGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		GroupID int64 `json:"groupId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := h.db.RemoveGroupMember(req.GroupID, user.ID); err != nil {
		log.Printf("Failed to leave group: %v", err)
		http.Error(w, "Failed to leave group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
*/

// handleCreateNote creates a new note (personal or group)
func (h *AuthHandler) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		GroupID *int64 `json:"group_id"`
		Book    string `json:"book"`
		Chapter int    `json:"chapter"`
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Book == "" || req.Chapter < 1 || req.Content == "" {
		respondError(w, "Invalid note data", http.StatusBadRequest)
		return
	}

	// Convert *int64 to sql.NullInt64
	var groupID sql.NullInt64
	if req.GroupID != nil {
		groupID.Int64 = *req.GroupID
		groupID.Valid = true

		// Verify user is a member of the group
		if !h.requireGroupMember(w, *req.GroupID, user.ID) {
			return
		}
	}

	note, err := h.db.CreateNote(user.ID, groupID, req.Book, req.Chapter, req.Content)
	if err != nil {
		log.Printf("Failed to create note: %v", err)
		respondError(w, "Failed to create note", http.StatusInternalServerError)
		return
	}

	respondJSON(w, note)
}

// handleGetNotes retrieves notes for a book/chapter
func (h *AuthHandler) handleGetNotes(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	book := r.URL.Query().Get("book")
	chapterStr := r.URL.Query().Get("chapter")
	groupIDStr := r.URL.Query().Get("group_id")

	chapter, err := strconv.Atoi(chapterStr)
	if err != nil || book == "" {
		respondError(w, "Invalid book or chapter", http.StatusBadRequest)
		return
	}

	var notes []*Note

	if groupIDStr != "" {
		// Get group notes
		groupID, ok := parseIDParam(w, groupIDStr, "group ID")
		if !ok {
			return
		}

		// Verify membership
		if !h.requireGroupMember(w, groupID, user.ID) {
			return
		}

		notes, err = h.db.GetGroupNotes(groupID, book, chapter)
		if err != nil {
			log.Printf("Failed to get group notes: %v", err)
			respondError(w, "Failed to get notes", http.StatusInternalServerError)
			return
		}
	} else {
		// Get personal notes
		notes, err = h.db.GetPersonalNotes(user.ID, book, chapter)
		if err != nil {
			log.Printf("Failed to get personal notes: %v", err)
			respondError(w, "Failed to get notes", http.StatusInternalServerError)
			return
		}
	}

	respondJSON(w, notes)
}

// handleUpdateNote updates a note
// DEPRECATED: Use handleUpdateNoteByID with RESTful routing instead
/*
func (h *AuthHandler) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		NoteID  int64  `json:"noteId"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := h.db.UpdateNote(req.NoteID, user.ID, req.Content); err != nil {
		log.Printf("Failed to update note: %v", err)
		http.Error(w, "Failed to update note", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
*/

// handleDeleteNote deletes a note
// DEPRECATED: Use handleDeleteNoteByID with RESTful routing instead
/*
func (h *AuthHandler) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	noteIDStr := r.URL.Query().Get("noteId")
	noteID, err := strconv.ParseInt(noteIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid note ID", http.StatusBadRequest)
		return
	}

	if err := h.db.DeleteNote(noteID, user.ID); err != nil {
		log.Printf("Failed to delete note: %v", err)
		http.Error(w, "Failed to delete note", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
*/

// handleCreateComment creates a comment on a note
func (h *AuthHandler) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		NoteID   int64  `json:"noteId"`
		ParentID *int64 `json:"parentId,omitempty"` // Optional parent comment ID for threading
		Content  string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Verify note exists and user has access
	note, ok := h.getNoteAndCheckAccess(w, req.NoteID, user.ID)
	if !ok {
		return
	}

	// Convert parent ID to sql.NullInt64
	var parentID sql.NullInt64
	if req.ParentID != nil {
		parentID = sql.NullInt64{Int64: *req.ParentID, Valid: true}
	}

	comment, err := h.db.CreateNoteComment(note.ID, user.ID, parentID, req.Content)
	if err != nil {
		log.Printf("Failed to create comment: %v", err)
		respondError(w, "Failed to create comment", http.StatusInternalServerError)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:   "note_comment",
		Action: "create",
		NoteID: int(req.NoteID),
		Data: map[string]interface{}{
			"comment_id": comment.ID,
		},
	})

	respondJSON(w, comment)
}

// handleGetComments retrieves comments for a note
func (h *AuthHandler) handleGetComments(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	noteID, ok := parseIDParam(w, r.URL.Query().Get("noteId"), "note ID")
	if !ok {
		return
	}

	// Verify access to note
	note, err := h.db.GetNote(noteID)
	if err != nil {
		respondError(w, "Note not found", http.StatusNotFound)
		return
	}

	if !h.requireNoteAccess(w, note, user.ID) {
		return
	}

	comments, err := h.db.GetNoteComments(noteID)
	if err != nil {
		log.Printf("Failed to get comments: %v", err)
		respondError(w, "Failed to get comments", http.StatusInternalServerError)
		return
	}

	respondJSON(w, comments)
}

// handleDeleteComment deletes a comment
func (h *AuthHandler) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	commentID, ok := parseIDParam(w, r.URL.Query().Get("commentId"), "comment ID")
	if !ok {
		return
	}

	if err := h.db.DeleteNoteComment(commentID, user.ID); err != nil {
		log.Printf("Failed to delete comment: %v", err)
		respondError(w, "Failed to delete comment", http.StatusInternalServerError)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:   "note_comment",
		Action: "delete",
		Data: map[string]interface{}{
			"comment_id": commentID,
		},
	})

	respondSuccess(w)
}

// handleCommentsRESTful handles /api/comments/* routes with path parameters
func (h *AuthHandler) handleCommentsRESTful(w http.ResponseWriter, r *http.Request) {
	// Parse the path: /api/comments/{id}/{action}
	path := r.URL.Path[len("/api/comments/"):]

	// Handle /api/comments/create
	if path == "create" {
		h.handleCreateComment(w, r)
		return
	}

	// Handle /api/comments/list
	if path == "list" {
		h.handleGetComments(w, r)
		return
	}

	// Handle /api/comments/delete
	if path == "delete" {
		h.handleDeleteComment(w, r)
		return
	}

	// Parse comment ID from path for /api/comments/{id}/* routes
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	commentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid comment ID", http.StatusBadRequest)
		return
	}

	action := parts[1]

	switch action {
	case "update":
		h.handleUpdateCommentByID(w, r, commentID)
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// handleUpdateCommentByID updates a comment (only by owner)
func (h *AuthHandler) handleUpdateCommentByID(w http.ResponseWriter, r *http.Request, commentID int64) {
	if !requireMethod(w, r, http.MethodPatch) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.db.UpdateNoteComment(commentID, user.ID, req.Content); err != nil {
		log.Printf("Failed to update comment: %v", err)
		respondError(w, "Failed to update comment", http.StatusInternalServerError)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:   "note_comment",
		Action: "update",
		Data: map[string]interface{}{
			"comment_id": commentID,
		},
	})

	respondSuccess(w)
}

// ============ RESTful Route Handlers with Path Parameters ============

// handleGroupsRESTful handles /api/groups/* routes with path parameters
func (h *AuthHandler) handleGroupsRESTful(w http.ResponseWriter, r *http.Request) {
	// Parse the path: /api/groups/{id}/{action}
	path := r.URL.Path[len("/api/groups/"):]

	// Handle /api/groups/list
	if path == "list" {
		h.handleGetUserGroups(w, r)
		return
	}

	// Handle /api/groups/create
	if path == "create" {
		h.handleCreateGroup(w, r)
		return
	}

	// Handle /api/groups/invites
	if path == "invites" {
		h.handleGetPendingInvites(w, r)
		return
	}

	// Parse group ID from path for /api/groups/{id}/* routes
	var groupID int64
	var action string

	// Split path to get ID and action
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	// Check if first part is "invites" for /api/groups/invites/{id}/accept or /api/groups/invites/{id}/decline
	if parts[0] == "invites" && len(parts) >= 3 {
		inviteID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			http.Error(w, "Invalid invite ID", http.StatusBadRequest)
			return
		}

		if parts[2] == "accept" {
			h.handleAcceptInviteByID(w, r, inviteID)
			return
		}

		if parts[2] == "decline" {
			h.handleDeclineInviteByID(w, r, inviteID)
			return
		}

		http.Error(w, "Unknown invite action", http.StatusNotFound)
		return
	}

	// Parse as group ID
	var err error
	groupID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	// No action means get group details
	if len(parts) == 1 {
		h.handleGetGroupDetailsByID(w, r, groupID)
		return
	}

	action = parts[1]

	switch action {
	case "members":
		h.handleGetGroupMembersByID(w, r, groupID)
	case "invite":
		h.handleInviteToGroupByID(w, r, groupID)
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// handleNotesRESTful handles /api/notes/* routes with path parameters
func (h *AuthHandler) handleNotesRESTful(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/notes/"):]

	// Handle /api/notes/create
	if path == "create" {
		h.handleCreateNote(w, r)
		return
	}

	// Handle /api/notes/personal and /api/notes/group
	if path == "personal" || path == "group" {
		h.handleGetNotes(w, r)
		return
	}

	// Parse note ID for /api/notes/{id}/* routes
	parts := splitPath(path)
	if len(parts) == 0 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	noteID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid note ID", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		// GET /api/notes/{id}
		h.handleGetNoteByID(w, r, noteID)
		return
	}

	action := parts[1]
	switch action {
	case "update":
		h.handleUpdateNoteByID(w, r, noteID)
	case "delete":
		h.handleDeleteNoteByID(w, r, noteID)
	case "comments":
		h.handleGetCommentsForNote(w, r, noteID)
	default:
		http.Error(w, "Unknown action", http.StatusNotFound)
	}
}

// Helper handlers with ID parameters

func (h *AuthHandler) handleGetGroupDetailsByID(w http.ResponseWriter, r *http.Request, groupID int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if !h.requireGroupMember(w, groupID, user.ID) {
		return
	}

	group, err := h.db.GetStudyGroup(groupID)
	if err != nil {
		respondError(w, "Group not found", http.StatusNotFound)
		return
	}

	respondJSON(w, group)
}

func (h *AuthHandler) handleGetGroupMembersByID(w http.ResponseWriter, r *http.Request, groupID int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if !h.requireGroupMember(w, groupID, user.ID) {
		return
	}

	members, err := h.db.GetGroupMembers(groupID)
	if err != nil {
		respondError(w, "Failed to get members", http.StatusInternalServerError)
		return
	}

	respondJSON(w, members)
}

func (h *AuthHandler) handleInviteToGroupByID(w http.ResponseWriter, r *http.Request, groupID int64) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Username string `json:"username"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Find user by username or email
	invitedUser, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		// Try by email
		invitedUser, err = h.db.GetUserByEmail(req.Username)
		if err != nil {
			respondError(w, "User not found", http.StatusNotFound)
			return
		}
	}

	// Check if user is trying to invite themselves
	if invitedUser.ID == user.ID {
		respondError(w, "Cannot invite yourself", http.StatusBadRequest)
		return
	}

	// Check if user is already a member
	isMember, err := h.db.IsGroupMember(groupID, invitedUser.ID)
	if err != nil {
		respondError(w, "Failed to check membership", http.StatusInternalServerError)
		return
	}
	if isMember {
		respondError(w, "User is already a member of this group", http.StatusBadRequest)
		return
	}

	// Create invite
	token, err := generateSecureToken()
	if err != nil {
		respondError(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	invite, err := h.db.CreateGroupInvite(groupID, user.ID, invitedUser.Email, token)
	if err != nil {
		log.Printf("Failed to create invite: %v", err)
		respondError(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	respondJSON(w, invite)
}

func (h *AuthHandler) handleAcceptInviteByID(w http.ResponseWriter, r *http.Request, inviteID int64) {
	h.handleInviteAction(w, r, inviteID, h.db.AcceptGroupInvite)
}

func (h *AuthHandler) handleDeclineInviteByID(w http.ResponseWriter, r *http.Request, inviteID int64) {
	h.handleInviteAction(w, r, inviteID, func(id, _ int64) error {
		return h.db.DeclineGroupInvite(id)
	})
}

func (h *AuthHandler) handleGetNoteByID(w http.ResponseWriter, r *http.Request, noteID int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	note, ok := h.getNoteAndCheckAccess(w, noteID, user.ID)
	if !ok {
		return
	}

	respondJSON(w, note)
}

func (h *AuthHandler) handleUpdateNoteByID(w http.ResponseWriter, r *http.Request, noteID int64) {
	if !requireMethod(w, r, "PUT") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.db.UpdateNote(noteID, user.ID, req.Content); err != nil {
		log.Printf("Failed to update note: %v", err)
		respondError(w, "Failed to update note", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

func (h *AuthHandler) handleDeleteNoteByID(w http.ResponseWriter, r *http.Request, noteID int64) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if err := h.db.DeleteNote(noteID, user.ID); err != nil {
		log.Printf("Failed to delete note: %v", err)
		respondError(w, "Failed to delete note", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

func (h *AuthHandler) handleGetCommentsForNote(w http.ResponseWriter, r *http.Request, noteID int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	// Verify user has access to the note
	_, ok := h.getNoteAndCheckAccess(w, noteID, user.ID)
	if !ok {
		return
	}

	comments, err := h.db.GetNoteComments(noteID)
	if err != nil {
		respondError(w, "Failed to get comments", http.StatusInternalServerError)
		return
	}

	respondJSON(w, comments)
}

// splitPath splits a URL path by "/"
func splitPath(path string) []string {
	var parts []string
	for _, p := range splitString(path, '/') {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func splitString(s string, sep rune) []string {
	var result []string
	var current string
	for _, c := range s {
		if c == sep {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// handleUploadLogo handles file upload for group logos
func (h *AuthHandler) handleUploadLogo(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if h.requireUser(w, r) == nil {
		return
	}

	// Parse multipart form (10MB max)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondError(w, "File too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("logo")
	if err != nil {
		respondError(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Error closing file: %v", err)
		}
	}()

	// Validate file type (only images)
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		respondError(w, "Only image files are allowed", http.StatusBadRequest)
		return
	}

	// Create uploads directory if it doesn't exist
	uploadsDir := "static/uploads"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Printf("Failed to create uploads directory: %v", err)
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	token, err := generateSecureToken()
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	filename := token + ext
	filepath := filepath.Join(uploadsDir, filename)

	// Save file
	dst, err := os.Create(filepath)
	if err != nil {
		log.Printf("Failed to create file: %v", err)
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := dst.Close(); err != nil {
			log.Printf("Error closing destination file: %v", err)
		}
	}()

	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Failed to save file: %v", err)
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Return the URL
	logoURL := "/uploads/" + filename
	respondJSON(w, map[string]string{"url": logoURL})
}

// handleCreateStudyPlan creates a new study plan for a group
func (h *AuthHandler) handleCreateStudyPlan(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		GroupID      int64  `json:"group_id"`
		WeekNumber   int    `json:"week_number"`
		StartDate    string `json:"start_date"`
		EndDate      string `json:"end_date"`
		Book         string `json:"book"`
		StartChapter int    `json:"start_chapter"`
		EndChapter   int    `json:"end_chapter"`
		Description  string `json:"description"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Verify user is group admin
	if !h.requireGroupAdmin(w, req.GroupID, user.ID) {
		return
	}

	plan, err := h.db.CreateStudyPlan(req.GroupID, req.WeekNumber, req.StartDate, req.EndDate,
		req.Book, req.StartChapter, req.EndChapter, req.Description)
	if err != nil {
		log.Printf("Failed to create study plan: %v", err)
		respondError(w, "Failed to create study plan", http.StatusInternalServerError)
		return
	}

	respondJSON(w, plan)
}

// handleGetGroupStudyPlans retrieves all study plans for a group
func (h *AuthHandler) handleGetGroupStudyPlans(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	groupID, ok := parseIDParam(w, r.URL.Query().Get("group_id"), "group ID")
	if !ok {
		return
	}

	// Verify user is group member
	if !h.requireGroupMember(w, groupID, user.ID) {
		return
	}

	plans, err := h.db.GetGroupStudyPlans(groupID)
	if err != nil {
		respondError(w, "Failed to get study plans", http.StatusInternalServerError)
		return
	}

	respondJSON(w, plans)
}

// handleUpdateStudyPlan updates an existing study plan
func (h *AuthHandler) handleUpdateStudyPlan(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		ID           int64  `json:"id"`
		GroupID      int64  `json:"group_id"`
		WeekNumber   int    `json:"week_number"`
		StartDate    string `json:"start_date"`
		EndDate      string `json:"end_date"`
		Book         string `json:"book"`
		StartChapter int    `json:"start_chapter"`
		EndChapter   int    `json:"end_chapter"`
		Description  string `json:"description"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Verify user is group admin
	if !h.requireGroupAdmin(w, req.GroupID, user.ID) {
		return
	}

	err := h.db.UpdateStudyPlan(req.ID, req.WeekNumber, req.StartDate, req.EndDate,
		req.Book, req.StartChapter, req.EndChapter, req.Description)
	if err != nil {
		log.Printf("Failed to update study plan: %v", err)
		respondError(w, "Failed to update study plan", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleDeleteStudyPlan deletes a study plan
func (h *AuthHandler) handleDeleteStudyPlan(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		ID      int64 `json:"id"`
		GroupID int64 `json:"group_id"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Verify user is group admin
	if !h.requireGroupAdmin(w, req.GroupID, user.ID) {
		return
	}

	err := h.db.DeleteStudyPlan(req.ID)
	if err != nil {
		log.Printf("Failed to delete study plan: %v", err)
		respondError(w, "Failed to delete study plan", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ============ Prayer Request Handlers ============

// handleCreatePrayerRequest creates a new prayer request for a group
func (h *AuthHandler) handleCreatePrayerRequest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		GroupID int64  `json:"group_id"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Title == "" || req.Content == "" {
		respondError(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	// Verify user is group member
	if !h.requireGroupMember(w, req.GroupID, user.ID) {
		return
	}

	prayer, err := h.db.CreatePrayerRequest(req.GroupID, user.ID, req.Title, req.Content)
	if err != nil {
		log.Printf("Failed to create prayer request: %v", err)
		respondError(w, "Failed to create prayer request", http.StatusInternalServerError)
		return
	}

	respondJSON(w, prayer)
}

// handleGetGroupPrayerRequests retrieves all prayer requests for a group
func (h *AuthHandler) handleGetGroupPrayerRequests(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	groupID, ok := parseIDParam(w, r.URL.Query().Get("group_id"), "group ID")
	if !ok {
		return
	}

	// Verify user is group member
	if !h.requireGroupMember(w, groupID, user.ID) {
		return
	}

	// Optional status filter
	status := r.URL.Query().Get("status")
	if status != "" && status != "active" && status != "answered" && status != "archived" {
		respondError(w, "Invalid status filter", http.StatusBadRequest)
		return
	}

	prayers, err := h.db.GetGroupPrayerRequests(groupID, status)
	if err != nil {
		log.Printf("Failed to get prayer requests: %v", err)
		respondError(w, "Failed to get prayer requests", http.StatusInternalServerError)
		return
	}

	respondJSON(w, prayers)
}

// handleGetPrayerRequest retrieves a single prayer request
func (h *AuthHandler) handleGetPrayerRequest(w http.ResponseWriter, r *http.Request, prayerID int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	prayer, err := h.db.GetPrayerRequest(prayerID)
	if err != nil {
		respondError(w, "Prayer request not found", http.StatusNotFound)
		return
	}

	// Verify user is group member
	if !h.requireGroupMember(w, prayer.GroupID, user.ID) {
		return
	}

	respondJSON(w, prayer)
}

// handleUpdatePrayerRequest updates a prayer request
func (h *AuthHandler) handleUpdatePrayerRequest(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, "PUT") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Title == "" || req.Content == "" {
		respondError(w, "Title and content are required", http.StatusBadRequest)
		return
	}

	if err := h.db.UpdatePrayerRequest(prayerID, user.ID, req.Title, req.Content); err != nil {
		log.Printf("Failed to update prayer request: %v", err)
		respondError(w, "Failed to update prayer request", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// handleUpdatePrayerRequestStatus updates the status of a prayer request
func (h *AuthHandler) handleUpdatePrayerRequestStatus(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, "PATCH") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Status            string `json:"status"`
		AnswerExplanation string `json:"answer_explanation"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.db.UpdatePrayerRequestStatus(prayerID, user.ID, req.Status, req.AnswerExplanation); err != nil {
		log.Printf("Failed to update prayer request status: %v", err)
		respondError(w, "Failed to update prayer request status", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// handleUpdatePrayerAnswerExplanation updates the answer explanation of a prayer request
func (h *AuthHandler) handleUpdatePrayerAnswerExplanation(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, "PATCH") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		AnswerExplanation string `json:"answer_explanation"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if err := h.db.UpdatePrayerAnswerExplanation(prayerID, user.ID, req.AnswerExplanation); err != nil {
		log.Printf("Failed to update answer explanation: %v", err)
		respondError(w, "Failed to update answer explanation", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// handleMarkPrayerAsUnanswered marks a prayer as unanswered (clears answer and sets to active)
func (h *AuthHandler) handleMarkPrayerAsUnanswered(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, "POST") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if err := h.db.MarkPrayerAsUnanswered(prayerID, user.ID); err != nil {
		log.Printf("Failed to mark prayer as unanswered: %v", err)
		respondError(w, "Failed to mark prayer as unanswered", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// handleDeletePrayerRequest deletes a prayer request
func (h *AuthHandler) handleDeletePrayerRequest(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if err := h.db.DeletePrayerRequest(prayerID, user.ID); err != nil {
		log.Printf("Failed to delete prayer request: %v", err)
		respondError(w, "Failed to delete prayer request", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// handleArchivePrayerRequest archives a prayer request
func (h *AuthHandler) handleArchivePrayerRequest(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if err := h.db.ArchivePrayerRequest(prayerID, user.ID); err != nil {
		log.Printf("Failed to archive prayer request: %v", err)
		respondError(w, err.Error(), http.StatusForbidden)
		return
	}

	respondSuccess(w)
}

// handleRestorePrayerRequest restores an archived prayer request
func (h *AuthHandler) handleRestorePrayerRequest(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	if err := h.db.RestorePrayerRequest(int(prayerID), int(user.ID)); err != nil {
		log.Printf("Failed to restore prayer request: %v", err)
		respondError(w, err.Error(), http.StatusForbidden)
		return
	}

	respondSuccess(w)
}

// handleGetArchivedPrayerRequests returns archived prayer requests for a group
func (h *AuthHandler) handleGetArchivedPrayerRequests(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	groupIDStr := r.URL.Query().Get("group_id")
	if groupIDStr == "" {
		respondError(w, "group_id is required", http.StatusBadRequest)
		return
	}

	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil {
		respondError(w, "Invalid group_id", http.StatusBadRequest)
		return
	}

	// Get archived prayers for the group
	prayers, err := h.db.GetGroupPrayerRequests(groupID, "archived")
	if err != nil {
		log.Printf("Failed to get archived prayer requests: %v", err)
		respondError(w, "Failed to get archived prayer requests", http.StatusInternalServerError)
		return
	}

	respondJSON(w, prayers)
}

// handleCreatePrayerComment creates a comment on a prayer request
func (h *AuthHandler) handleCreatePrayerComment(w http.ResponseWriter, r *http.Request, prayerID int64) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Content == "" {
		respondError(w, "Content is required", http.StatusBadRequest)
		return
	}

	// Verify prayer request exists and user is group member
	prayer, err := h.db.GetPrayerRequest(prayerID)
	if err != nil {
		respondError(w, "Prayer request not found", http.StatusNotFound)
		return
	}

	if !h.requireGroupMember(w, prayer.GroupID, user.ID) {
		return
	}

	comment, err := h.db.CreatePrayerComment(prayerID, user.ID, req.Content)
	if err != nil {
		log.Printf("Failed to create prayer comment: %v", err)
		respondError(w, "Failed to create comment", http.StatusInternalServerError)
		return
	}

	respondJSON(w, comment)
}

// handleGetPrayerComments retrieves comments for a prayer request
func (h *AuthHandler) handleGetPrayerComments(w http.ResponseWriter, r *http.Request, prayerID int64) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	// Verify prayer request exists and user is group member
	prayer, err := h.db.GetPrayerRequest(prayerID)
	if err != nil {
		respondError(w, "Prayer request not found", http.StatusNotFound)
		return
	}

	if !h.requireGroupMember(w, prayer.GroupID, user.ID) {
		return
	}

	comments, err := h.db.GetPrayerComments(prayerID)
	if err != nil {
		log.Printf("Failed to get prayer comments: %v", err)
		respondError(w, "Failed to get comments", http.StatusInternalServerError)
		return
	}

	respondJSON(w, comments)
}

// handleDeletePrayerComment deletes a comment on a prayer request
func (h *AuthHandler) handleDeletePrayerComment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	commentID, ok := parseIDParam(w, r.URL.Query().Get("commentId"), "comment ID")
	if !ok {
		return
	}

	if err := h.db.DeletePrayerComment(commentID, user.ID); err != nil {
		log.Printf("Failed to delete prayer comment: %v", err)
		respondError(w, "Failed to delete comment", http.StatusInternalServerError)
		return
	}

	respondSuccess(w)
}

// handlePrayersRESTful handles /api/prayers/* routes with path parameters
func (h *AuthHandler) handlePrayersRESTful(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/prayers/"):]

	// Handle /api/prayers/create
	if path == "create" {
		h.handleCreatePrayerRequest(w, r)
		return
	}

	// Handle /api/prayers/group (list prayers for a group)
	if path == "group" {
		h.handleGetGroupPrayerRequests(w, r)
		return
	}

	// Handle /api/prayers/archived (list archived prayers for a group)
	if path == "archived" {
		h.handleGetArchivedPrayerRequests(w, r)
		return
	}

	// Parse prayer ID for /api/prayers/{id}/* routes
	parts := splitPath(path)
	if len(parts) == 0 {
		respondError(w, "Invalid path", http.StatusBadRequest)
		return
	}

	prayerID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, "Invalid prayer request ID", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		// GET /api/prayers/{id}
		h.handleGetPrayerRequest(w, r, prayerID)
		return
	}

	action := parts[1]
	switch action {
	case "update":
		h.handleUpdatePrayerRequest(w, r, prayerID)
	case "status":
		h.handleUpdatePrayerRequestStatus(w, r, prayerID)
	case "answer":
		h.handleUpdatePrayerAnswerExplanation(w, r, prayerID)
	case "unanswered":
		h.handleMarkPrayerAsUnanswered(w, r, prayerID)
	case "archive":
		h.handleArchivePrayerRequest(w, r, prayerID)
	case "restore":
		h.handleRestorePrayerRequest(w, r, prayerID)
	case "delete":
		h.handleDeletePrayerRequest(w, r, prayerID)
	case "comments":
		if len(parts) == 2 {
			if r.Method == http.MethodPost {
				h.handleCreatePrayerComment(w, r, prayerID)
			} else {
				h.handleGetPrayerComments(w, r, prayerID)
			}
		} else {
			respondError(w, "Unknown action", http.StatusNotFound)
		}
	default:
		respondError(w, "Unknown action", http.StatusNotFound)
	}
}

// ============ Profile Handlers ============

// handleGetProfile returns the current user's profile
func (h *AuthHandler) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	respondJSON(w, user)
}

// handleUpdateProfile updates the current user's profile
func (h *AuthHandler) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var data struct {
		FirstName         string `json:"first_name"`
		LastName          string `json:"last_name"`
		ProfilePictureURL string `json:"profile_picture_url"`
		LocationCity      string `json:"location_city"`
		LocationState     string `json:"location_state"`
	}

	if !decodeJSONBody(w, r, &data) {
		return
	}

	// Update profile in database
	if err := h.db.UpdateProfile(user.ID, data.FirstName, data.LastName, data.ProfilePictureURL, data.LocationCity, data.LocationState); err != nil {
		log.Printf("Failed to update profile: %v", err)
		respondError(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	// Return updated user
	updatedUser, err := h.db.GetUserByID(user.ID)
	if err != nil {
		respondError(w, "Failed to get updated user", http.StatusInternalServerError)
		return
	}

	respondJSON(w, updatedUser)
}

// handleUpdateDefaultTranslation updates the current user's default Bible translation
func (h *AuthHandler) handleUpdateDefaultTranslation(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var data struct {
		Translation string `json:"translation"`
	}

	if !decodeJSONBody(w, r, &data) {
		return
	}

	if data.Translation == "" {
		respondError(w, "Translation is required", http.StatusBadRequest)
		return
	}

	// Update default translation in database
	if err := h.db.UpdateDefaultTranslation(user.ID, data.Translation); err != nil {
		log.Printf("Failed to update default translation: %v", err)
		respondError(w, "Failed to update default translation", http.StatusInternalServerError)
		return
	}

	// Return updated user
	updatedUser, err := h.db.GetUserByID(user.ID)
	if err != nil {
		respondError(w, "Failed to get updated user", http.StatusInternalServerError)
		return
	}

	respondJSON(w, updatedUser)
}

// handleUpdateFilters updates a user's OSIS filter preferences
func (h *AuthHandler) handleUpdateFilters(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var filters OSISFilters
	if !decodeJSONBody(w, r, &filters) {
		return
	}

	// Update filters in database
	if err := h.db.UpdateFilters(user.ID, filters); err != nil {
		log.Printf("Failed to update filters: %v", err)
		respondError(w, "Failed to update filters", http.StatusInternalServerError)
		return
	}

	// Return updated user with new filter preferences
	updatedUser, err := h.db.GetUserByID(user.ID)
	if err != nil {
		log.Printf("Failed to get updated user: %v", err)
		respondError(w, "Failed to get updated user", http.StatusInternalServerError)
		return
	}

	respondJSON(w, updatedUser)
}

// handleGetFilters returns a user's current filter preferences
func (h *AuthHandler) handleGetFilters(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	// Convert User filter fields to OSISFilters struct
	filters := OSISFilters{
		ShowStrongs:    user.FilterStrongs,
		ShowFootnotes:  user.FilterFootnotes,
		ShowScripref:   user.FilterScripref,
		ShowHeadings:   user.FilterHeadings,
		ShowRedLetters: user.FilterRedLetters,
		ShowLemma:      user.FilterLemma,
		ShowMorph:      user.FilterMorph,
		ShowXlit:       user.FilterXlit,
	}

	respondJSON(w, map[string]interface{}{
		"filters": filters,
	})
}

// handleUploadProfilePicture handles profile picture uploads
func (h *AuthHandler) handleUploadProfilePicture(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	// Parse multipart form (max 10MB)
	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		respondError(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("profile_picture")
	if err != nil {
		respondError(w, "Failed to get file from form", http.StatusBadRequest)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/gif" && contentType != "image/webp" {
		respondError(w, "Invalid file type. Only JPEG, PNG, GIF, and WebP are allowed", http.StatusBadRequest)
		return
	}

	// Create uploads directory if it doesn't exist
	uploadsDir := "static/uploads/profile-pictures"
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Printf("Failed to create uploads directory: %v", err)
		respondError(w, "Failed to create uploads directory", http.StatusInternalServerError)
		return
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	filename := filepath.Join(uploadsDir, strconv.FormatInt(user.ID, 10)+ext)

	// Create the file
	dst, err := os.Create(filename)
	if err != nil {
		log.Printf("Failed to create file: %v", err)
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := dst.Close(); err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()

	// Copy the uploaded file to the destination
	if _, err := io.Copy(dst, file); err != nil {
		log.Printf("Failed to save file: %v", err)
		respondError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Update profile picture URL in database
	profilePictureURL := "/uploads/profile-pictures/" + strconv.FormatInt(user.ID, 10) + ext
	if err := h.db.UpdateProfile(user.ID, user.FirstName, user.LastName, profilePictureURL, user.LocationCity, user.LocationState); err != nil {
		log.Printf("Failed to update profile picture URL: %v", err)
		respondError(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"url": profilePictureURL})
}

// handleVerseCommentsRESTful handles /api/verse-comments/* routes
func (h *AuthHandler) handleVerseCommentsRESTful(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/verse-comments/"):]

	// Handle /api/verse-comments/create
	if path == "create" {
		h.handleCreateVerseComment(w, r)
		return
	}

	// Handle /api/verse-comments/list
	if path == "list" {
		h.handleGetVerseComments(w, r)
		return
	}

	// Parse comment ID for /api/verse-comments/{id}/* routes
	parts := splitPath(path)
	if len(parts) == 0 {
		respondError(w, "Invalid path", http.StatusBadRequest)
		return
	}

	commentID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, "Invalid comment ID", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		respondError(w, "Action required", http.StatusBadRequest)
		return
	}

	action := parts[1]
	switch action {
	case "update":
		h.handleUpdateVerseComment(w, r, commentID)
	case "delete":
		h.handleDeleteVerseComment(w, r, commentID)
	default:
		respondError(w, "Unknown action", http.StatusNotFound)
	}
}

// handleCreateVerseComment creates a new verse comment
func (h *AuthHandler) handleCreateVerseComment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "POST") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		GroupID  *int64 `json:"group_id"`
		Book     string `json:"book"`
		Chapter  int    `json:"chapter"`
		Verse    int    `json:"verse"`
		ParentID *int64 `json:"parent_id"`
		Content  string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Book == "" || req.Chapter < 1 || req.Verse < 1 || req.Content == "" {
		respondError(w, "Book, chapter, verse, and content are required", http.StatusBadRequest)
		return
	}

	var groupID sql.NullInt64
	if req.GroupID != nil {
		groupID = sql.NullInt64{Int64: *req.GroupID, Valid: true}
		// Verify user is member of the group
		if !h.requireGroupMember(w, *req.GroupID, user.ID) {
			return
		}
	}

	var parentID sql.NullInt64
	if req.ParentID != nil {
		parentID = sql.NullInt64{Int64: *req.ParentID, Valid: true}
	}

	comment, err := h.db.CreateVerseComment(groupID, user.ID, req.Book, req.Chapter, req.Verse, parentID, req.Content)
	if err != nil {
		log.Printf("Failed to create verse comment: %v", err)
		respondError(w, "Failed to create comment", http.StatusInternalServerError)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:    "verse_comment",
		Action:  "create",
		Book:    req.Book,
		Chapter: req.Chapter,
		Verse:   req.Verse,
		Data: map[string]interface{}{
			"comment_id": comment.ID,
			"group_id":   req.GroupID,
		},
	})

	respondJSON(w, comment)
}

// handleGetVerseComments retrieves comments for a verse
func (h *AuthHandler) handleGetVerseComments(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, "GET") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	book := r.URL.Query().Get("book")
	chapterStr := r.URL.Query().Get("chapter")
	verseStr := r.URL.Query().Get("verse")
	groupIDStr := r.URL.Query().Get("group_id")

	if book == "" || chapterStr == "" || verseStr == "" {
		respondError(w, "Book, chapter, and verse are required", http.StatusBadRequest)
		return
	}

	chapter, err := strconv.Atoi(chapterStr)
	if err != nil || chapter < 1 {
		respondError(w, "Invalid chapter number", http.StatusBadRequest)
		return
	}

	verse, err := strconv.Atoi(verseStr)
	if err != nil || verse < 1 {
		respondError(w, "Invalid verse number", http.StatusBadRequest)
		return
	}

	var groupID sql.NullInt64
	if groupIDStr != "" {
		gid, err := strconv.ParseInt(groupIDStr, 10, 64)
		if err != nil {
			respondError(w, "Invalid group ID", http.StatusBadRequest)
			return
		}
		groupID = sql.NullInt64{Int64: gid, Valid: true}
		// Verify user is member of the group
		if !h.requireGroupMember(w, gid, user.ID) {
			return
		}
	}

	comments, err := h.db.GetVerseComments(book, chapter, verse, groupID)
	if err != nil {
		log.Printf("Failed to get verse comments: %v", err)
		respondError(w, "Failed to get comments", http.StatusInternalServerError)
		return
	}

	respondJSON(w, comments)
}

// handleUpdateVerseComment updates a verse comment
func (h *AuthHandler) handleUpdateVerseComment(w http.ResponseWriter, r *http.Request, commentID int64) {
	if !requireMethod(w, r, "PATCH") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	var req struct {
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.Content == "" {
		respondError(w, "Content is required", http.StatusBadRequest)
		return
	}

	// Get comment details before updating (for broadcast)
	comment, err := h.db.GetVerseComment(commentID)
	if err != nil {
		log.Printf("Failed to get verse comment: %v", err)
		respondError(w, "Comment not found", http.StatusNotFound)
		return
	}

	err = h.db.UpdateVerseComment(commentID, user.ID, req.Content)
	if err != nil {
		log.Printf("Failed to update verse comment: %v", err)
		respondError(w, err.Error(), http.StatusForbidden)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:    "verse_comment",
		Action:  "update",
		Book:    comment.Book,
		Chapter: comment.Chapter,
		Verse:   comment.Verse,
		Data: map[string]interface{}{
			"comment_id": commentID,
		},
	})

	respondSuccess(w)
}

// handleDeleteVerseComment deletes a verse comment
func (h *AuthHandler) handleDeleteVerseComment(w http.ResponseWriter, r *http.Request, commentID int64) {
	if !requireMethod(w, r, "DELETE") {
		return
	}

	user := h.requireUser(w, r)
	if user == nil {
		return
	}

	// Get comment details before deleting (for broadcast)
	comment, err := h.db.GetVerseComment(commentID)
	if err != nil {
		log.Printf("Failed to get verse comment: %v", err)
		respondError(w, "Comment not found", http.StatusNotFound)
		return
	}

	err = h.db.DeleteVerseComment(commentID, user.ID)
	if err != nil {
		log.Printf("Failed to delete verse comment: %v", err)
		respondError(w, err.Error(), http.StatusForbidden)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:    "verse_comment",
		Action:  "delete",
		Book:    comment.Book,
		Chapter: comment.Chapter,
		Verse:   comment.Verse,
		Data: map[string]interface{}{
			"comment_id": commentID,
		},
	})

	respondSuccess(w)
}
