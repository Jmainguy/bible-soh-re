package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Note represents a study note (personal or group)
type Note struct {
	ID                int64         `json:"id"`
	UserID            int64         `json:"user_id"`
	GroupID           sql.NullInt64 `json:"group_id"` // Null for personal notes
	Book              string        `json:"book"`
	Chapter           int           `json:"chapter"`
	Content           string        `json:"content"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	Username          string        `json:"username"`            // For display purposes
	FirstName         string        `json:"first_name"`          // User's first name
	LastName          string        `json:"last_name"`           // User's last name
	ProfilePictureURL string        `json:"profile_picture_url"` // User's profile picture
}

// NoteComment represents a comment on a note
type NoteComment struct {
	ID                int64          `json:"id"`
	NoteID            int64          `json:"note_id"`
	UserID            int64          `json:"user_id"`
	ParentID          sql.NullInt64  `json:"parent_id"` // Null for top-level comments
	Content           string         `json:"content"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Username          string         `json:"username"`            // For display purposes
	FirstName         string         `json:"first_name"`          // User's first name
	LastName          string         `json:"last_name"`           // User's last name
	ProfilePictureURL string         `json:"profile_picture_url"` // User's profile picture
	Replies           []*NoteComment `json:"replies,omitempty"`   // Nested replies for threading
}

// CreateNote creates a new note
func (d *Database) CreateNote(userID int64, groupID sql.NullInt64, book string, chapter int, content string) (*Note, error) {
	result, err := d.db.Exec(
		`INSERT INTO notes (user_id, group_id, book, chapter, content) VALUES (?, ?, ?, ?, ?)`,
		userID, groupID, book, chapter, content,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create note: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get note ID: %w", err)
	}

	return d.GetNote(id)
}

// GetNote retrieves a note by ID
func (d *Database) GetNote(id int64) (*Note, error) {
	note := &Note{}
	err := d.db.QueryRow(
		`SELECT n.id, n.user_id, n.group_id, n.book, n.chapter, n.content, 
		        n.created_at, n.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM notes n
		 INNER JOIN users u ON n.user_id = u.id
		 WHERE n.id = ?`,
		id,
	).Scan(&note.ID, &note.UserID, &note.GroupID, &note.Book, &note.Chapter,
		&note.Content, &note.CreatedAt, &note.UpdatedAt, &note.Username, &note.FirstName, &note.LastName, &note.ProfilePictureURL)
	if err != nil {
		return nil, err
	}
	return note, nil
}

// GetPersonalNotes retrieves all personal notes for a user at a specific book/chapter
func (d *Database) GetPersonalNotes(userID int64, book string, chapter int) ([]*Note, error) {
	rows, err := d.db.Query(
		`SELECT n.id, n.user_id, n.group_id, n.book, n.chapter, n.content,
		        n.created_at, n.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM notes n
		 INNER JOIN users u ON n.user_id = u.id
		 WHERE n.user_id = ? AND n.group_id IS NULL AND n.book = ? AND n.chapter = ?
		 ORDER BY n.created_at DESC`,
		userID, book, chapter,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	return scanNotes(rows)
}

// GetGroupNotes retrieves all group notes for a specific book/chapter
func (d *Database) GetGroupNotes(groupID int64, book string, chapter int) ([]*Note, error) {
	rows, err := d.db.Query(
		`SELECT n.id, n.user_id, n.group_id, n.book, n.chapter, n.content,
		        n.created_at, n.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM notes n
		 INNER JOIN users u ON n.user_id = u.id
		 WHERE n.group_id = ? AND n.book = ? AND n.chapter = ?
		 ORDER

 BY n.created_at DESC`,
		groupID, book, chapter,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	return scanNotes(rows)
}

// scanNotes is a helper to scan multiple notes from rows
func scanNotes(rows *sql.Rows) ([]*Note, error) {
	var notes []*Note
	for rows.Next() {
		note := &Note{}
		if err := rows.Scan(&note.ID, &note.UserID, &note.GroupID, &note.Book,
			&note.Chapter, &note.Content, &note.CreatedAt, &note.UpdatedAt,
			&note.Username, &note.FirstName, &note.LastName, &note.ProfilePictureURL); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, nil
}

// UpdateNote updates a note's content (only by the owner)
func (d *Database) UpdateNote(noteID, userID int64, content string) error {
	result, err := d.db.Exec(
		`UPDATE notes SET content = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		content, time.Now(), noteID, userID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("note not found or user not authorized")
	}
	return nil
}

// DeleteNote deletes a note (only by the owner)
func (d *Database) DeleteNote(noteID, userID int64) error {
	result, err := d.db.Exec(
		`DELETE FROM notes WHERE id = ? AND user_id = ?`,
		noteID, userID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("note not found or user not authorized")
	}
	return nil
}

// CreateNoteComment creates a comment on a note
func (d *Database) CreateNoteComment(noteID, userID int64, parentID sql.NullInt64, content string) (*NoteComment, error) {
	result, err := d.db.Exec(
		`INSERT INTO note_comments (note_id, user_id, parent_id, content) VALUES (?, ?, ?, ?)`,
		noteID, userID, parentID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get comment ID: %w", err)
	}

	return d.GetNoteComment(id)
}

// GetNoteComment retrieves a comment by ID
func (d *Database) GetNoteComment(id int64) (*NoteComment, error) {
	comment := &NoteComment{}
	err := d.db.QueryRow(
		`SELECT c.id, c.note_id, c.user_id, c.parent_id, c.content, c.created_at, c.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM note_comments c
		 INNER JOIN users u ON c.user_id = u.id
		 WHERE c.id = ?`,
		id,
	).Scan(&comment.ID, &comment.NoteID, &comment.UserID, &comment.ParentID, &comment.Content,
		&comment.CreatedAt, &comment.UpdatedAt, &comment.Username, &comment.FirstName, &comment.LastName, &comment.ProfilePictureURL)
	if err != nil {
		return nil, err
	}
	return comment, nil
}

// GetNoteComments retrieves all comments for a note with threading
func (d *Database) GetNoteComments(noteID int64) ([]*NoteComment, error) {
	rows, err := d.db.Query(
		`SELECT c.id, c.note_id, c.user_id, c.parent_id, c.content, c.created_at, c.updated_at, u.username, u.first_name, u.last_name, u.profile_picture_url
		 FROM note_comments c
		 INNER JOIN users u ON c.user_id = u.id
		 WHERE c.note_id = ?
		 ORDER BY c.created_at ASC`,
		noteID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var comments []*NoteComment
	for rows.Next() {
		comment := &NoteComment{}
		if err := rows.Scan(&comment.ID, &comment.NoteID, &comment.UserID, &comment.ParentID, &comment.Content,
			&comment.CreatedAt, &comment.UpdatedAt, &comment.Username, &comment.FirstName, &comment.LastName, &comment.ProfilePictureURL); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}

	// Build threaded structure
	return buildNoteCommentTree(comments), nil
}

// buildNoteCommentTree organizes flat comments into a threaded structure
func buildNoteCommentTree(comments []*NoteComment) []*NoteComment {
	commentMap := make(map[int64]*NoteComment)
	rootComments := []*NoteComment{} // Initialize as empty slice, not nil

	// First pass: index all comments
	for _, comment := range comments {
		commentMap[comment.ID] = comment
		comment.Replies = []*NoteComment{} // Initialize empty replies
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

// UpdateNoteComment updates a comment (only by the owner)
func (d *Database) UpdateNoteComment(commentID, userID int64, content string) error {
	result, err := d.db.Exec(
		`UPDATE note_comments SET content = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		content, time.Now(), commentID, userID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("comment not found or user not authorized")
	}
	return nil
}

// DeleteNoteComment deletes a comment (only by the owner)
func (d *Database) DeleteNoteComment(commentID, userID int64) error {
	result, err := d.db.Exec(
		`DELETE FROM note_comments WHERE id = ? AND user_id = ?`,
		commentID, userID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("comment not found or user not authorized")
	}
	return nil
}
