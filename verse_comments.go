package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// VerseComment represents a threaded comment on a specific verse
type VerseComment struct {
	ID                int64           `json:"id"`
	GroupID           sql.NullInt64   `json:"group_id"` // Null for personal comments
	UserID            int64           `json:"user_id"`
	Book              string          `json:"book"`
	Chapter           int             `json:"chapter"`
	Verse             int             `json:"verse"`
	ParentID          sql.NullInt64   `json:"parent_id"` // Null for top-level comments
	Content           string          `json:"content"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Username          string          `json:"username"`
	FirstName         string          `json:"first_name"`
	LastName          string          `json:"last_name"`
	ProfilePictureURL string          `json:"profile_picture_url"`
	Replies           []*VerseComment `json:"replies,omitempty"` // Nested replies for threading
}

// CreateVerseComment creates a new verse comment
func (d *Database) CreateVerseComment(groupID sql.NullInt64, userID int64, book string, chapter, verse int, parentID sql.NullInt64, content string) (*VerseComment, error) {
	result, err := d.db.Exec(
		`INSERT INTO verse_comments (group_id, user_id, book, chapter, verse, parent_id, content) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		groupID, userID, book, chapter, verse, parentID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create verse comment: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return d.GetVerseComment(id)
}

// GetVerseComment retrieves a single verse comment by ID
func (d *Database) GetVerseComment(id int64) (*VerseComment, error) {
	comment := &VerseComment{}
	err := d.db.QueryRow(
		`SELECT vc.id, vc.group_id, vc.user_id, vc.book, vc.chapter, vc.verse, vc.parent_id,
		        vc.content, vc.created_at, vc.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM verse_comments vc
		 INNER JOIN users u ON vc.user_id = u.id
		 WHERE vc.id = ?`,
		id,
	).Scan(&comment.ID, &comment.GroupID, &comment.UserID, &comment.Book, &comment.Chapter, &comment.Verse, &comment.ParentID,
		&comment.Content, &comment.CreatedAt, &comment.UpdatedAt, &comment.Username, &comment.FirstName, &comment.LastName, &comment.ProfilePictureURL)

	if err != nil {
		return nil, err
	}
	return comment, nil
}

// GetVerseComments retrieves comments for a specific verse, optionally filtered by group
func (d *Database) GetVerseComments(book string, chapter, verse int, groupID sql.NullInt64) ([]*VerseComment, error) {
	var query string
	var args []interface{}

	if groupID.Valid {
		// Get comments for specific group
		query = `SELECT vc.id, vc.group_id, vc.user_id, vc.book, vc.chapter, vc.verse, vc.parent_id,
		                vc.content, vc.created_at, vc.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		         FROM verse_comments vc
		         INNER JOIN users u ON vc.user_id = u.id
		         WHERE vc.book = ? AND vc.chapter = ? AND vc.verse = ? AND vc.group_id = ?
		         ORDER BY vc.created_at ASC`
		args = []interface{}{book, chapter, verse, groupID}
	} else {
		// Get personal comments (no group)
		query = `SELECT vc.id, vc.group_id, vc.user_id, vc.book, vc.chapter, vc.verse, vc.parent_id,
		                vc.content, vc.created_at, vc.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		         FROM verse_comments vc
		         INNER JOIN users u ON vc.user_id = u.id
		         WHERE vc.book = ? AND vc.chapter = ? AND vc.verse = ? AND vc.group_id IS NULL
		         ORDER BY vc.created_at ASC`
		args = []interface{}{book, chapter, verse}
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query verse comments: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var comments []*VerseComment
	for rows.Next() {
		comment := &VerseComment{}
		err := rows.Scan(&comment.ID, &comment.GroupID, &comment.UserID, &comment.Book, &comment.Chapter, &comment.Verse, &comment.ParentID,
			&comment.Content, &comment.CreatedAt, &comment.UpdatedAt, &comment.Username, &comment.FirstName, &comment.LastName, &comment.ProfilePictureURL)
		if err != nil {
			return nil, fmt.Errorf("failed to scan verse comment: %w", err)
		}
		comments = append(comments, comment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating verse comments: %w", err)
	}

	// Build threaded structure
	return buildCommentTree(comments), nil
}

// buildCommentTree organizes flat comments into a threaded structure
func buildCommentTree(comments []*VerseComment) []*VerseComment {
	commentMap := make(map[int64]*VerseComment)
	rootComments := []*VerseComment{} // Initialize as empty slice, not nil

	// First pass: index all comments
	for _, comment := range comments {
		commentMap[comment.ID] = comment
		comment.Replies = []*VerseComment{} // Initialize empty replies
	}

	// Second pass: build tree structure
	for _, comment := range comments {
		if comment.ParentID.Valid {
			// This is a reply
			if parent, exists := commentMap[comment.ParentID.Int64]; exists {
				parent.Replies = append(parent.Replies, comment)
			}
		} else {
			// This is a root comment
			rootComments = append(rootComments, comment)
		}
	}

	return rootComments
}

// UpdateVerseComment updates an existing verse comment
func (d *Database) UpdateVerseComment(id, userID int64, content string) error {
	result, err := d.db.Exec(
		`UPDATE verse_comments SET content = ?, updated_at = CURRENT_TIMESTAMP 
		 WHERE id = ? AND user_id = ?`,
		content, id, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to update verse comment: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("verse comment not found or you don't have permission to edit it")
	}

	return nil
}

// DeleteVerseComment deletes a verse comment (and all its replies via CASCADE)
func (d *Database) DeleteVerseComment(id, userID int64) error {
	// Check if user owns the comment or is admin/owner of the group
	var comment VerseComment
	err := d.db.QueryRow(
		`SELECT id, user_id, group_id FROM verse_comments WHERE id = ?`,
		id,
	).Scan(&comment.ID, &comment.UserID, &comment.GroupID)

	if err != nil {
		return fmt.Errorf("verse comment not found")
	}

	// Check permission
	isOwner := comment.UserID == userID
	isGroupAdmin := false

	if comment.GroupID.Valid {
		var role string
		err = d.db.QueryRow(
			`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
			comment.GroupID, userID,
		).Scan(&role)
		if err == nil {
			isGroupAdmin = (role == "admin" || role == "owner")
		}
	}

	if !isOwner && !isGroupAdmin {
		return fmt.Errorf("you don't have permission to delete this comment")
	}

	// Delete the comment (CASCADE will delete replies)
	_, err = d.db.Exec(`DELETE FROM verse_comments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete verse comment: %w", err)
	}

	return nil
}

// GetVerseCommentCount returns the count of comments for a specific verse
func (d *Database) GetVerseCommentCount(book string, chapter, verse int, groupID sql.NullInt64) (int, error) {
	var count int
	var err error

	if groupID.Valid {
		err = d.db.QueryRow(
			`SELECT COUNT(*) FROM verse_comments 
			 WHERE book = ? AND chapter = ? AND verse = ? AND group_id = ?`,
			book, chapter, verse, groupID,
		).Scan(&count)
	} else {
		err = d.db.QueryRow(
			`SELECT COUNT(*) FROM verse_comments 
			 WHERE book = ? AND chapter = ? AND verse = ? AND group_id IS NULL`,
			book, chapter, verse,
		).Scan(&count)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to count verse comments: %w", err)
	}
	return count, nil
}
