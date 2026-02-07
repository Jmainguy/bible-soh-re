package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// PrayerRequest represents a prayer request in a group
type PrayerRequest struct {
	ID                int64      `json:"id"`
	GroupID           int64      `json:"group_id"`
	UserID            int64      `json:"user_id"`
	Title             string     `json:"title"`
	Content           string     `json:"content"`
	Status            string     `json:"status"` // "active" or "answered" only (archived state tracked by archived_at)
	AnswerExplanation string     `json:"answer_explanation,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ArchivedAt        *time.Time `json:"archived_at"`         // nil for not archived, non-nil for archived
	Username          string     `json:"username"`            // For backwards compatibility
	FirstName         string     `json:"first_name"`          // For display
	LastName          string     `json:"last_name"`           // For display
	ProfilePictureURL string     `json:"profile_picture_url"` // For display
}

// PrayerComment represents a comment/prayer on a prayer request
type PrayerComment struct {
	ID                int64     `json:"id"`
	PrayerID          int64     `json:"prayer_id"`
	UserID            int64     `json:"user_id"`
	Content           string    `json:"content"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Username          string    `json:"username"`            // For backwards compatibility
	FirstName         string    `json:"first_name"`          // For display
	LastName          string    `json:"last_name"`           // For display
	ProfilePictureURL string    `json:"profile_picture_url"` // For display
}

// CreatePrayerRequest creates a new prayer request
func (d *Database) CreatePrayerRequest(groupID, userID int64, title, content string) (*PrayerRequest, error) {
	result, err := d.db.Exec(
		`INSERT INTO prayer_requests (group_id, user_id, title, content, status) VALUES (?, ?, ?, ?, 'active')`,
		groupID, userID, title, content,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create prayer request: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get prayer request ID: %w", err)
	}

	return d.GetPrayerRequest(id)
}

// GetPrayerRequest retrieves a prayer request by ID
func (d *Database) GetPrayerRequest(id int64) (*PrayerRequest, error) {
	prayer := &PrayerRequest{}
	err := d.db.QueryRow(
		`SELECT pr.id, pr.group_id, pr.user_id, pr.title, pr.content, pr.status,
		        pr.answer_explanation, pr.created_at, pr.updated_at, pr.archived_at, u.username, u.first_name, u.last_name
		 FROM prayer_requests pr
		 INNER JOIN users u ON pr.user_id = u.id
		 WHERE pr.id = ?`,
		id,
	).Scan(&prayer.ID, &prayer.GroupID, &prayer.UserID, &prayer.Title, &prayer.Content,
		&prayer.Status, &prayer.AnswerExplanation, &prayer.CreatedAt, &prayer.UpdatedAt, &prayer.ArchivedAt, &prayer.Username, &prayer.FirstName, &prayer.LastName)
	if err != nil {
		return nil, err
	}
	return prayer, nil
}

// GetGroupPrayerRequests retrieves all prayer requests for a group
func (d *Database) GetGroupPrayerRequests(groupID int64, status string) ([]*PrayerRequest, error) {
	query := `SELECT pr.id, pr.group_id, pr.user_id, pr.title, pr.content, pr.status,
	                 pr.answer_explanation, pr.created_at, pr.updated_at, pr.archived_at, u.username, u.first_name, u.last_name, u.profile_picture_url
	          FROM prayer_requests pr
	          INNER JOIN users u ON pr.user_id = u.id
	          WHERE pr.group_id = ?`

	var rows *sql.Rows
	var err error

	if status == "archived" {
		// Archived prayers are identified by archived_at being set
		query += ` AND pr.archived_at IS NOT NULL ORDER BY pr.created_at DESC`
		rows, err = d.db.Query(query, groupID)
	} else if status != "" {
		// Active or answered prayers must not be archived
		query += ` AND pr.archived_at IS NULL AND pr.status = ? ORDER BY pr.created_at DESC`
		rows, err = d.db.Query(query, groupID, status)
	} else {
		// All non-archived prayers
		query += ` AND pr.archived_at IS NULL ORDER BY pr.created_at DESC`
		rows, err = d.db.Query(query, groupID)
	}

	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	return scanPrayerRequests(rows)
}

// UpdatePrayerRequest updates a prayer request's content
func (d *Database) UpdatePrayerRequest(id, userID int64, title, content string) error {
	result, err := d.db.Exec(
		`UPDATE prayer_requests SET title = ?, content = ?, updated_at = CURRENT_TIMESTAMP 
		 WHERE id = ? AND user_id = ?`,
		title, content, id, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update prayer request: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("prayer request not found or not authorized")
	}

	return nil
}

// UpdatePrayerRequestStatus updates the status of a prayer request
// Status can only be 'active' or 'answered'. Archiving is handled separately via archived_at.
// Allows prayer author or group admins/owners to update status
func (d *Database) UpdatePrayerRequestStatus(id, userID int64, status, answerExplanation string) error {
	// Validate status - only 'active' or 'answered'
	if status != "active" && status != "answered" {
		return fmt.Errorf("invalid status: %s (must be 'active' or 'answered')", status)
	}

	// Get the prayer request to check authorization
	prayer, err := d.GetPrayerRequest(id)
	if err != nil {
		return fmt.Errorf("prayer request not found: %w", err)
	}

	// Check if user is the prayer author
	isAuthor := prayer.UserID == userID

	// Check if user is admin or owner of the group
	var role string
	err = d.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
		prayer.GroupID, userID).Scan(&role)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check group membership: %w", err)
	}

	isAdminOrOwner := (role == "admin" || role == "owner")

	// Only allow update if user is author OR admin/owner
	if !isAuthor && !isAdminOrOwner {
		return fmt.Errorf("only the prayer author or group admins can update prayer status")
	}

	var result sql.Result
	if status == "answered" {
		// When marking as answered, store the answer explanation and preserve archived_at
		result, err = d.db.Exec(
			`UPDATE prayer_requests SET status = ?, answer_explanation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, answerExplanation, id,
		)
	} else {
		// When setting to active, clear archived_at to unarchive
		result, err = d.db.Exec(
			`UPDATE prayer_requests SET status = ?, archived_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			status, id,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to update prayer request status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("prayer request not found")
	}

	return nil
}

// DeletePrayerRequest deletes a prayer request
func (d *Database) DeletePrayerRequest(id, userID int64) error {
	result, err := d.db.Exec(
		`DELETE FROM prayer_requests WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete prayer request: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("prayer request not found or not authorized")
	}

	return nil
}

// CreatePrayerComment creates a comment on a prayer request
func (d *Database) CreatePrayerComment(prayerID, userID int64, content string) (*PrayerComment, error) {
	result, err := d.db.Exec(
		`INSERT INTO prayer_comments (prayer_id, user_id, content) VALUES (?, ?, ?)`,
		prayerID, userID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create prayer comment: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get comment ID: %w", err)
	}

	return d.GetPrayerComment(id)
}

// GetPrayerComment retrieves a prayer comment by ID
func (d *Database) GetPrayerComment(id int64) (*PrayerComment, error) {
	comment := &PrayerComment{}
	err := d.db.QueryRow(
		`SELECT pc.id, pc.prayer_id, pc.user_id, pc.content, pc.created_at, pc.updated_at, u.username, u.first_name, u.last_name
		 FROM prayer_comments pc
		 INNER JOIN users u ON pc.user_id = u.id
		 WHERE pc.id = ?`,
		id,
	).Scan(&comment.ID, &comment.PrayerID, &comment.UserID, &comment.Content,
		&comment.CreatedAt, &comment.UpdatedAt, &comment.Username, &comment.FirstName, &comment.LastName)
	if err != nil {
		return nil, err
	}
	return comment, nil
}

// GetPrayerComments retrieves all comments for a prayer request
func (d *Database) GetPrayerComments(prayerID int64) ([]*PrayerComment, error) {
	rows, err := d.db.Query(
		`SELECT pc.id, pc.prayer_id, pc.user_id, pc.content, pc.created_at, pc.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM prayer_comments pc
		 INNER JOIN users u ON pc.user_id = u.id
		 WHERE pc.prayer_id = ?
		 ORDER BY pc.created_at ASC`,
		prayerID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	return scanPrayerComments(rows)
}

// DeletePrayerComment deletes a prayer comment
func (d *Database) DeletePrayerComment(id, userID int64) error {
	result, err := d.db.Exec(
		`DELETE FROM prayer_comments WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete prayer comment: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("comment not found or not authorized")
	}

	return nil
}

// Helper functions

func scanPrayerRequests(rows *sql.Rows) ([]*PrayerRequest, error) {
	var prayers []*PrayerRequest
	for rows.Next() {
		prayer := &PrayerRequest{}
		if err := rows.Scan(&prayer.ID, &prayer.GroupID, &prayer.UserID, &prayer.Title,
			&prayer.Content, &prayer.Status, &prayer.AnswerExplanation, &prayer.CreatedAt, &prayer.UpdatedAt, &prayer.ArchivedAt,
			&prayer.Username, &prayer.FirstName, &prayer.LastName, &prayer.ProfilePictureURL); err != nil {
			return nil, err
		}
		prayers = append(prayers, prayer)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prayers, nil
}

// AutoArchiveOldPrayers archives prayer requests that are older than 2 weeks
// This includes both 'active' and 'answered' prayers
func (d *Database) AutoArchiveOldPrayers() error {
	twoWeeksAgo := time.Now().AddDate(0, 0, -14)

	// Set archived_at without changing status, so answered prayers remain answered
	query := `UPDATE prayer_requests 
		SET archived_at = ?, updated_at = ?
		WHERE archived_at IS NULL AND created_at < ?`

	now := time.Now()
	_, err := d.db.Exec(query, now, now, twoWeeksAgo)
	return err
}

// ArchivePrayerRequest archives a prayer request by setting archived_at
// Allows prayer author or group admins/owners to archive
func (d *Database) ArchivePrayerRequest(id, userID int64) error {
	// Get the prayer request to check authorization
	prayer, err := d.GetPrayerRequest(id)
	if err != nil {
		return fmt.Errorf("prayer request not found: %w", err)
	}

	// Check if user is the prayer author
	isAuthor := prayer.UserID == userID

	// Check if user is admin or owner of the group
	var role string
	err = d.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
		prayer.GroupID, userID).Scan(&role)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check group membership: %w", err)
	}

	isAdminOrOwner := (role == "admin" || role == "owner")

	// Only allow archive if user is author OR admin/owner
	if !isAuthor && !isAdminOrOwner {
		return fmt.Errorf("only the prayer author or group admins can archive prayers")
	}

	// Set archived_at timestamp
	now := time.Now()
	result, err := d.db.Exec(
		`UPDATE prayer_requests SET archived_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	)

	if err != nil {
		return fmt.Errorf("failed to archive prayer request: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("prayer request not found")
	}

	return nil
}

// UpdatePrayerAnswerExplanation updates the answer explanation of a prayer request
// Only the prayer author or group admins/owners can update answer explanations
func (d *Database) UpdatePrayerAnswerExplanation(prayerID int64, userID int64, answerExplanation string) error {
	// Get the prayer request to check authorization
	prayer, err := d.GetPrayerRequest(prayerID)
	if err != nil {
		return fmt.Errorf("prayer request not found: %w", err)
	}

	// Check if user is the prayer author
	isAuthor := prayer.UserID == userID

	// Check if user is admin or owner of the group
	var role string
	err = d.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
		prayer.GroupID, userID).Scan(&role)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check group membership: %w", err)
	}

	isAdminOrOwner := (role == "admin" || role == "owner")

	// Only allow update if user is author OR admin/owner
	if !isAuthor && !isAdminOrOwner {
		return fmt.Errorf("only the prayer author or group admins can update answer explanations")
	}

	// Update answer explanation and set status to answered
	_, err = d.db.Exec(
		`UPDATE prayer_requests SET answer_explanation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		answerExplanation, prayerID,
	)

	if err != nil {
		return fmt.Errorf("failed to update answer explanation: %w", err)
	}

	return nil
}

// MarkPrayerAsUnanswered clears the answer explanation and sets status back to active
// Preserves archived_at so archived prayers remain archived
// Only the prayer author or group admins/owners can mark as unanswered
func (d *Database) MarkPrayerAsUnanswered(prayerID int64, userID int64) error {
	// Get the prayer request to check authorization
	prayer, err := d.GetPrayerRequest(prayerID)
	if err != nil {
		return fmt.Errorf("prayer request not found: %w", err)
	}

	// Check if user is the prayer author
	isAuthor := prayer.UserID == userID

	// Check if user is admin or owner of the group
	var role string
	err = d.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
		prayer.GroupID, userID).Scan(&role)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check group membership: %w", err)
	}

	isAdminOrOwner := (role == "admin" || role == "owner")

	// Only allow update if user is author OR admin/owner
	if !isAuthor && !isAdminOrOwner {
		return fmt.Errorf("only the prayer author or group admins can mark prayer as unanswered")
	}

	// Clear answer explanation and set status back to active (preserve archived_at)
	_, err = d.db.Exec(
		`UPDATE prayer_requests SET answer_explanation = '', status = 'active', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		prayerID,
	)

	if err != nil {
		return fmt.Errorf("failed to mark prayer as unanswered: %w", err)
	}

	return nil
}

// RestorePrayerRequest restores an archived prayer request, giving it a fresh 2-week run
// Only the prayer author or group admins/owners can restore prayers
func (d *Database) RestorePrayerRequest(prayerID, userID int) error {
	// First, get the prayer request to check the author and group
	prayer, err := d.GetPrayerRequest(int64(prayerID))
	if err != nil {
		return err
	}

	// Check if user is the prayer author
	isAuthor := int(prayer.UserID) == userID

	// Check if user is admin or owner of the group
	var role string
	err = d.db.QueryRow(`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
		prayer.GroupID, userID).Scan(&role)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	isAdminOrOwner := (role == "admin" || role == "owner")

	// Only allow restore if user is author OR admin/owner
	if !isAuthor && !isAdminOrOwner {
		return fmt.Errorf("only the prayer author or group admins can restore a prayer request")
	}

	// Restore the prayer by resetting created_at and clearing archived_at
	// Status (active/answered) is preserved
	now := time.Now()
	query := `UPDATE prayer_requests 
		SET created_at = ?, updated_at = ?, archived_at = NULL
		WHERE id = ?`

	_, err = d.db.Exec(query, now, now, prayerID)
	return err
}

func scanPrayerComments(rows *sql.Rows) ([]*PrayerComment, error) {
	var comments []*PrayerComment
	for rows.Next() {
		comment := &PrayerComment{}
		if err := rows.Scan(&comment.ID, &comment.PrayerID, &comment.UserID, &comment.Content,
			&comment.CreatedAt, &comment.UpdatedAt, &comment.Username, &comment.FirstName, &comment.LastName, &comment.ProfilePictureURL); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return comments, nil
}
