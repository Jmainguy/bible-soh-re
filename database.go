package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
	ID            int64          `json:"id"`
	Email         string         `json:"email"`
	Username      string         `json:"username"`
	PasswordHash  sql.NullString `json:"-"`              // Never expose password hash
	OAuthProvider string         `json:"oauth_provider"` // "local", "google", or "github"
	OAuthID       sql.NullString `json:"-"`              // Don't expose OAuth ID
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Session represents a user session
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Database handles all database operations
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database connection and initializes tables
func NewDatabase(dbPath string) (*Database, error) {
	// Open with proper SQLite parameters for read-write access
	// _journal_mode=WAL enables Write-Ahead Logging for better concurrency
	// _busy_timeout=5000 waits up to 5 seconds if database is locked
	// _parse_time=true automatically parses DATETIME values into time.Time
	// _loc=UTC ensures times are in UTC timezone
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_parse_time=true&_loc=UTC")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for better concurrency
	db.SetMaxOpenConns(1) // SQLite works best with single writer
	db.SetMaxIdleConns(1)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &Database{db: db}

	// Initialize tables
	if err := database.initTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return database, nil
}

// initTables creates the necessary database tables
func (d *Database) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT,
			oauth_provider TEXT DEFAULT 'local',
			oauth_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
		`CREATE INDEX IF NOT EXISTS idx_users_oauth ON users(oauth_provider, oauth_id)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE TABLE IF NOT EXISTS reading_positions (
			user_id INTEGER PRIMARY KEY,
			translation TEXT NOT NULL,
			book TEXT NOT NULL,
			chapter INTEGER NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS study_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			logo_url TEXT,
			created_by INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_study_groups_created_by ON study_groups(created_by)`,
		`CREATE TABLE IF NOT EXISTS study_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL,
			week_number INTEGER NOT NULL,
			start_date DATE NOT NULL,
			end_date DATE NOT NULL,
			book TEXT NOT NULL,
			start_chapter INTEGER NOT NULL,
			end_chapter INTEGER NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (group_id) REFERENCES study_groups(id) ON DELETE CASCADE,
			UNIQUE(group_id, week_number)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_study_plans_group_id ON study_plans(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_study_plans_dates ON study_plans(start_date, end_date)`,
		`CREATE TABLE IF NOT EXISTS group_members (
			group_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (group_id, user_id),
			FOREIGN KEY (group_id) REFERENCES study_groups(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_group_members_user_id ON group_members(user_id)`,
		`CREATE TABLE IF NOT EXISTS group_invites (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id INTEGER NOT NULL,
			invited_by INTEGER NOT NULL,
			email TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (group_id) REFERENCES study_groups(id) ON DELETE CASCADE,
			FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_group_invites_token ON group_invites(token)`,
		`CREATE INDEX IF NOT EXISTS idx_group_invites_email ON group_invites(email)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			group_id INTEGER,
			book TEXT NOT NULL,
			chapter INTEGER NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (group_id) REFERENCES study_groups(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_user_book_chapter ON notes(user_id, book, chapter)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_group_book_chapter ON notes(group_id, book, chapter)`,
		`CREATE TABLE IF NOT EXISTS note_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			note_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_note_comments_note_id ON note_comments(note_id)`,
	}

	for _, query := range queries {
		if _, err := d.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// CreateUser creates a new user with username/password
func (d *Database) CreateUser(email, username, password string) (*User, error) {
	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := d.db.Exec(
		`INSERT INTO users (email, username, password_hash, oauth_provider) VALUES (?, ?, ?, 'local')`,
		email, username, string(hashedPassword),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	return d.GetUserByID(id)
}

// CreateOAuthUser creates a new user from OAuth
func (d *Database) CreateOAuthUser(email, username, provider, oauthID string) (*User, error) {
	result, err := d.db.Exec(
		`INSERT INTO users (email, username, oauth_provider, oauth_id) VALUES (?, ?, ?, ?)`,
		email, username, provider, oauthID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}

	return d.GetUserByID(id)
}

// GetUserByID retrieves a user by ID
func (d *Database) GetUserByID(id int64) (*User, error) {
	user := &User{}
	err := d.db.QueryRow(
		`SELECT id, email, username, password_hash, oauth_provider, oauth_id, created_at, updated_at 
		 FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.OAuthProvider,
		&user.OAuthID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByEmail retrieves a user by email
func (d *Database) GetUserByEmail(email string) (*User, error) {
	user := &User{}
	err := d.db.QueryRow(
		`SELECT id, email, username, password_hash, oauth_provider, oauth_id, created_at, updated_at 
		 FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.OAuthProvider,
		&user.OAuthID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByUsername retrieves a user by username
func (d *Database) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := d.db.QueryRow(
		`SELECT id, email, username, password_hash, oauth_provider, oauth_id, created_at, updated_at 
		 FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.OAuthProvider,
		&user.OAuthID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByOAuth retrieves a user by OAuth provider and ID
func (d *Database) GetUserByOAuth(provider, oauthID string) (*User, error) {
	user := &User{}
	err := d.db.QueryRow(
		`SELECT id, email, username, password_hash, oauth_provider, oauth_id, created_at, updated_at 
		 FROM users WHERE oauth_provider = ? AND oauth_id = ?`,
		provider, oauthID,
	).Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.OAuthProvider,
		&user.OAuthID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// VerifyPassword checks if the provided password matches the user's password hash
func (d *Database) VerifyPassword(user *User, password string) bool {
	if !user.PasswordHash.Valid {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(password))
	return err == nil
}

// CreateSession creates a new session for a user
func (d *Database) CreateSession(userID int64, sessionID string, duration time.Duration) (*Session, error) {
	expiresAt := time.Now().Add(duration)

	_, err := d.db.Exec(
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		sessionID, userID, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &Session{
		ID:        sessionID,
		UserID:    userID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}, nil
}

// GetSession retrieves a session by ID
func (d *Database) GetSession(sessionID string) (*Session, error) {
	session := &Session{}
	err := d.db.QueryRow(
		`SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?`,
		sessionID,
	).Scan(&session.ID, &session.UserID, &session.ExpiresAt, &session.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		// Delete expired session
		_ = d.DeleteSession(sessionID)
		return nil, sql.ErrNoRows
	}

	return session, nil
}

// DeleteSession deletes a session
func (d *Database) DeleteSession(sessionID string) error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE id = ?`, sessionID)
	return err
}

// DeleteUserSessions deletes all sessions for a user
func (d *Database) DeleteUserSessions(userID int64) error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// CleanupExpiredSessions removes all expired sessions
func (d *Database) CleanupExpiredSessions() error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now())
	return err
}

// ReadingPosition represents a user's reading position
type ReadingPosition struct {
	UserID      int64
	Translation string
	Book        string
	Chapter     int
	UpdatedAt   time.Time
}

// SaveReadingPosition saves or updates a user's reading position
func (d *Database) SaveReadingPosition(userID int64, translation, book string, chapter int) error {
	_, err := d.db.Exec(
		`INSERT INTO reading_positions (user_id, translation, book, chapter, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
			translation = excluded.translation,
			book = excluded.book,
			chapter = excluded.chapter,
			updated_at = excluded.updated_at`,
		userID, translation, book, chapter, time.Now(),
	)
	return err
}

// GetReadingPosition retrieves a user's reading position
func (d *Database) GetReadingPosition(userID int64) (*ReadingPosition, error) {
	pos := &ReadingPosition{}
	err := d.db.QueryRow(
		`SELECT user_id, translation, book, chapter, updated_at
		 FROM reading_positions WHERE user_id = ?`,
		userID,
	).Scan(&pos.UserID, &pos.Translation, &pos.Book, &pos.Chapter, &pos.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return pos, nil
}
