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
		NoteID  int64  `json:"noteId"`
		Content string `json:"content"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	// Verify note exists and user has access
	note, ok := h.getNoteAndCheckAccess(w, req.NoteID, user.ID)
	if !ok {
		return
	}

	comment, err := h.db.CreateNoteComment(note.ID, user.ID, req.Content)
	if err != nil {
		log.Printf("Failed to create comment: %v", err)
		respondError(w, "Failed to create comment", http.StatusInternalServerError)
		return
	}

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
