package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Reaction represents an emoji reaction to a note, comment, or prayer comment
type Reaction struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	TargetType string    `json:"target_type"` // "note", "note_comment", "prayer_comment"
	TargetID   int64     `json:"target_id"`
	Emoji      string    `json:"emoji"`
	CreatedAt  time.Time `json:"created_at"`
	// User profile info
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	ProfilePictureURL string `json:"profile_picture_url"`
	Username          string `json:"username"`
}

// ReactionSummary represents aggregated reactions for display
type ReactionSummary struct {
	Emoji      string  `json:"emoji"`
	Count      int     `json:"count"`
	UserIDs    []int64 `json:"user_ids"`
	HasReacted bool    `json:"has_reacted"` // Whether current user has reacted with this emoji
}

// AddReaction adds or updates a reaction (upsert)
func (d *Database) AddReaction(userID int64, targetType string, targetID int64, emoji string) error {
	// Validate target type
	validTypes := map[string]bool{"note": true, "note_comment": true, "prayer_comment": true, "prayer_request": true, "verse_comment": true}
	if !validTypes[targetType] {
		return fmt.Errorf("invalid target type: %s", targetType)
	}

	query := `
		INSERT INTO reactions (user_id, target_type, target_id, emoji)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, target_type, target_id, emoji) DO NOTHING
	`
	_, err := d.db.Exec(query, userID, targetType, targetID, emoji)
	return err
}

// RemoveReaction removes a specific reaction
func (d *Database) RemoveReaction(userID int64, targetType string, targetID int64, emoji string) error {
	query := `
		DELETE FROM reactions
		WHERE user_id = ? AND target_type = ? AND target_id = ? AND emoji = ?
	`
	_, err := d.db.Exec(query, userID, targetType, targetID, emoji)
	return err
}

// GetReactions gets all reactions for a target with user info
func (d *Database) GetReactions(targetType string, targetID int64) ([]Reaction, error) {
	query := `
		SELECT 
			r.id,
			r.user_id,
			r.target_type,
			r.target_id,
			r.emoji,
			r.created_at,
			u.first_name,
			u.last_name,
			u.profile_picture_url,
			u.username
		FROM reactions r
		JOIN users u ON r.user_id = u.id
		WHERE r.target_type = ? AND r.target_id = ?
		ORDER BY r.created_at DESC
	`

	rows, err := d.db.Query(query, targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var reactions []Reaction
	for rows.Next() {
		var r Reaction
		err := rows.Scan(
			&r.ID,
			&r.UserID,
			&r.TargetType,
			&r.TargetID,
			&r.Emoji,
			&r.CreatedAt,
			&r.FirstName,
			&r.LastName,
			&r.ProfilePictureURL,
			&r.Username,
		)
		if err != nil {
			return nil, err
		}
		reactions = append(reactions, r)
	}

	return reactions, nil
}

// GetReactionsSummary gets aggregated reactions for a target
func (d *Database) GetReactionsSummary(targetType string, targetID int64, currentUserID int64) ([]ReactionSummary, error) {
	query := `
		SELECT 
			emoji,
			COUNT(*) as count,
			GROUP_CONCAT(user_id) as user_ids,
			MAX(CASE WHEN user_id = ? THEN 1 ELSE 0 END) as has_reacted
		FROM reactions
		WHERE target_type = ? AND target_id = ?
		GROUP BY emoji
		ORDER BY count DESC, emoji ASC
	`

	rows, err := d.db.Query(query, currentUserID, targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var summaries []ReactionSummary
	for rows.Next() {
		var s ReactionSummary
		var userIDsStr sql.NullString
		var hasReacted int

		err := rows.Scan(&s.Emoji, &s.Count, &userIDsStr, &hasReacted)
		if err != nil {
			return nil, err
		}

		s.HasReacted = hasReacted == 1

		// Parse user IDs
		if userIDsStr.Valid && userIDsStr.String != "" {
			userIDStrs := splitUserIDs(userIDsStr.String)
			for _, idStr := range userIDStrs {
				if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
					s.UserIDs = append(s.UserIDs, id)
				}
			}
		}

		summaries = append(summaries, s)
	}

	return summaries, nil
}

// API Handlers

// handleToggleReaction toggles a reaction (add if not exists, remove if exists)
func (s *AuthHandler) handleToggleReaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := s.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	var req struct {
		TargetType string `json:"target_type"`
		TargetID   int64  `json:"target_id"`
		Emoji      string `json:"emoji"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate emoji (basic check - just ensure it's not empty and reasonable length)
	if req.Emoji == "" || len(req.Emoji) > 10 {
		http.Error(w, "Invalid emoji", http.StatusBadRequest)
		return
	}

	// Check if reaction already exists
	reactions, err := s.db.GetReactions(req.TargetType, req.TargetID)
	if err != nil {
		http.Error(w, "Failed to get reactions", http.StatusInternalServerError)
		return
	}

	exists := false
	for _, reaction := range reactions {
		if reaction.UserID == userID && reaction.Emoji == req.Emoji {
			exists = true
			break
		}
	}

	if exists {
		// Remove reaction (toggle off)
		err = s.db.RemoveReaction(userID, req.TargetType, req.TargetID, req.Emoji)
	} else {
		// Add reaction (toggle on)
		err = s.db.AddReaction(userID, req.TargetType, req.TargetID, req.Emoji)
	}

	if err != nil {
		http.Error(w, "Failed to toggle reaction", http.StatusInternalServerError)
		return
	}

	// Broadcast update to all connected clients
	BroadcastUpdate(BroadcastMessage{
		Type:   "reaction",
		Action: "update",
		Data: map[string]interface{}{
			"target_type": req.TargetType,
			"target_id":   req.TargetID,
			"emoji":       req.Emoji,
		},
	})

	// Return updated summary
	summary, err := s.db.GetReactionsSummary(req.TargetType, req.TargetID, userID)
	if err != nil {
		http.Error(w, "Failed to get reaction summary", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// handleGetReactionsSummary gets reaction summary for a target
func (s *AuthHandler) handleGetReactionsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, err := s.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	userID := user.ID

	targetType := r.URL.Query().Get("target_type")
	targetIDStr := r.URL.Query().Get("target_id")

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid target_id", http.StatusBadRequest)
		return
	}

	summary, err := s.db.GetReactionsSummary(targetType, targetID, userID)
	if err != nil {
		http.Error(w, "Failed to get reactions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// splitUserIDs splits a comma-separated string of user IDs
func splitUserIDs(s string) []string {
	if s == "" {
		return nil
	}
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if start < i {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
