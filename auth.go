package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

const (
	sessionCookieName = "bible_session"
	sessionDuration   = 24 * time.Hour * 30 // 30 days
)

// AuthConfig holds OAuth configuration
type AuthConfig struct {
	GoogleClientID     string `yaml:"googleClientID"`
	GoogleClientSecret string `yaml:"googleClientSecret"`
	GitHubClientID     string `yaml:"githubClientID"`
	GitHubClientSecret string `yaml:"githubClientSecret"`
	BaseURL            string `yaml:"baseURL"` // e.g., http://localhost:8080
	SessionSecret      string `yaml:"sessionSecret"`
}

// AuthHandler manages authentication
type AuthHandler struct {
	db          *Database
	config      *AuthConfig
	googleOAuth *oauth2.Config
	githubOAuth *oauth2.Config
	oauthStates map[string]time.Time // Simple state storage (use Redis in production)
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(db *Database, config *AuthConfig) *AuthHandler {
	handler := &AuthHandler{
		db:          db,
		config:      config,
		oauthStates: make(map[string]time.Time),
	}

	// Configure Google OAuth
	if config.GoogleClientID != "" && config.GoogleClientSecret != "" {
		handler.googleOAuth = &oauth2.Config{
			ClientID:     config.GoogleClientID,
			ClientSecret: config.GoogleClientSecret,
			RedirectURL:  config.BaseURL + "/auth/google/callback",
			Scopes: []string{
				"https://www.googleapis.com/auth/userinfo.email",
				"https://www.googleapis.com/auth/userinfo.profile",
			},
			Endpoint: google.Endpoint,
		}
	}

	// Configure GitHub OAuth
	if config.GitHubClientID != "" && config.GitHubClientSecret != "" {
		handler.githubOAuth = &oauth2.Config{
			ClientID:     config.GitHubClientID,
			ClientSecret: config.GitHubClientSecret,
			RedirectURL:  config.BaseURL + "/auth/github/callback",
			Scopes:       []string{"user:email"},
			Endpoint:     github.Endpoint,
		}
	}

	// Start cleanup goroutine for expired states
	go handler.cleanupStates()

	return handler
}

// generateSecureToken generates a secure random token
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// createSession creates a new session for a user
func (h *AuthHandler) createSession(w http.ResponseWriter, userID int64) error {
	sessionID, err := generateSecureToken()
	if err != nil {
		return err
	}

	_, err = h.db.CreateSession(userID, sessionID, sessionDuration)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// getCurrentUser retrieves the current user from the session
func (h *AuthHandler) getCurrentUser(r *http.Request) (*User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}

	session, err := h.db.GetSession(cookie.Value)
	if err != nil {
		return nil, err
	}

	return h.db.GetUserByID(session.UserID)
}

// handleRegister handles user registration
func (h *AuthHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/register?error="+url.QueryEscape("Method not allowed"), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/register?error="+url.QueryEscape("Invalid form data"), http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	firstName := r.FormValue("first_name")
	lastName := r.FormValue("last_name")
	password := r.FormValue("password")

	if email == "" || firstName == "" || lastName == "" || password == "" {
		http.Redirect(w, r, "/register?error="+url.QueryEscape("All fields are required"), http.StatusSeeOther)
		return
	}

	// Create user
	user, err := h.db.CreateUser(email, firstName, lastName, password)
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		http.Redirect(w, r, "/register?error="+url.QueryEscape("Failed to create user. Email or username may already exist."), http.StatusSeeOther)
		return
	}

	// Create session
	if err := h.createSession(w, user.ID); err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Redirect(w, r, "/register?error="+url.QueryEscape("Failed to create session"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogin handles user login
func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Method not allowed"), http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Invalid form data"), http.StatusSeeOther)
		return
	}

	identifier := r.FormValue("identifier") // Can be email or username
	password := r.FormValue("password")

	if identifier == "" || password == "" {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("All fields are required"), http.StatusSeeOther)
		return
	}

	// Try to get user by email first, then username
	var user *User
	var err error

	user, err = h.db.GetUserByEmail(identifier)
	if err == sql.ErrNoRows {
		user, err = h.db.GetUserByUsername(identifier)
	}

	if err != nil {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Invalid credentials"), http.StatusSeeOther)
		return
	}

	// Verify password
	if !h.db.VerifyPassword(user, password) {
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Invalid credentials"), http.StatusSeeOther)
		return
	}

	// Create session
	if err := h.createSession(w, user.ID); err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Redirect(w, r, "/login?error="+url.QueryEscape("Failed to create session"), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout handles user logout
func (h *AuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		_ = h.db.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleGoogleLogin initiates Google OAuth flow
func (h *AuthHandler) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if h.googleOAuth == nil {
		http.Error(w, "Google OAuth not configured", http.StatusInternalServerError)
		return
	}

	state, err := generateSecureToken()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	h.oauthStates[state] = time.Now().Add(10 * time.Minute)
	url := h.googleOAuth.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// handleGoogleCallback handles Google OAuth callback
func (h *AuthHandler) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.googleOAuth == nil {
		http.Error(w, "Google OAuth not configured", http.StatusInternalServerError)
		return
	}

	state := r.URL.Query().Get("state")
	if _, exists := h.oauthStates[state]; !exists {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	delete(h.oauthStates, state)

	code := r.URL.Query().Get("code")
	token, err := h.googleOAuth.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("Failed to exchange token: %v", err)
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	// Get user info
	client := h.googleOAuth.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Printf("Failed to get user info: %v", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	var userInfo struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	if err := json.Unmarshal(body, &userInfo); err != nil {
		http.Error(w, "Failed to parse user info", http.StatusInternalServerError)
		return
	}

	// Split name into first and last
	firstName, lastName := splitName(userInfo.Name)

	// Get or create user
	user, err := h.db.GetUserByOAuth("google", userInfo.ID)
	if err == sql.ErrNoRows {
		// Create new user
		user, err = h.db.CreateOAuthUser(userInfo.Email, firstName, lastName, "google", userInfo.ID)
		if err != nil {
			log.Printf("Failed to create OAuth user: %v", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Create session
	if err := h.createSession(w, user.ID); err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleGitHubLogin initiates GitHub OAuth flow
func (h *AuthHandler) handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if h.githubOAuth == nil {
		http.Error(w, "GitHub OAuth not configured", http.StatusInternalServerError)
		return
	}

	state, err := generateSecureToken()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	h.oauthStates[state] = time.Now().Add(10 * time.Minute)
	url := h.githubOAuth.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// handleGitHubCallback handles GitHub OAuth callback
func (h *AuthHandler) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if h.githubOAuth == nil {
		http.Error(w, "GitHub OAuth not configured", http.StatusInternalServerError)
		return
	}

	state := r.URL.Query().Get("state")
	if _, exists := h.oauthStates[state]; !exists {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}
	delete(h.oauthStates, state)

	code := r.URL.Query().Get("code")
	token, err := h.githubOAuth.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("Failed to exchange token: %v", err)
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	// Get user info
	client := h.githubOAuth.Client(context.Background(), token)
	resp, err := client.Get("https://api.github.com/user")
	if err != nil {
		log.Printf("Failed to get user info: %v", err)
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	var userInfo struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}

	if err := json.Unmarshal(body, &userInfo); err != nil {
		http.Error(w, "Failed to parse user info", http.StatusInternalServerError)
		return
	}

	// If email is not public, try to get it from emails endpoint
	if userInfo.Email == "" {
		emailResp, err := client.Get("https://api.github.com/user/emails")
		if err == nil {
			defer func() {
				if err := emailResp.Body.Close(); err != nil {
					log.Printf("Error closing email response body: %v", err)
				}
			}()
			var emails []struct {
				Email   string `json:"email"`
				Primary bool   `json:"primary"`
			}
			emailBody, _ := io.ReadAll(emailResp.Body)
			if json.Unmarshal(emailBody, &emails) == nil {
				for _, e := range emails {
					if e.Primary {
						userInfo.Email = e.Email
						break
					}
				}
				if userInfo.Email == "" && len(emails) > 0 {
					userInfo.Email = emails[0].Email
				}
			}
		}
	}

	// Default email if still not found
	if userInfo.Email == "" {
		userInfo.Email = fmt.Sprintf("%s@github.user", userInfo.Login)
	}

	// Split name into first and last (if name is available, otherwise use login)
	firstName, lastName := splitName(userInfo.Name)
	if firstName == "" {
		firstName = userInfo.Login
	}

	// Get or create user
	oauthID := fmt.Sprintf("%d", userInfo.ID)
	user, err := h.db.GetUserByOAuth("github", oauthID)
	if err == sql.ErrNoRows {
		// Create new user
		user, err = h.db.CreateOAuthUser(userInfo.Email, firstName, lastName, "github", oauthID)
		if err != nil {
			log.Printf("Failed to create OAuth user: %v", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Create session
	if err := h.createSession(w, user.ID); err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// cleanupStates periodically removes expired OAuth states
func (h *AuthHandler) cleanupStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		for state, expiry := range h.oauthStates {
			if now.After(expiry) {
				delete(h.oauthStates, state)
			}
		}

		// Also cleanup expired database sessions
		_ = h.db.CleanupExpiredSessions()
	}
}

// requireAuth is a middleware that requires authentication
// DEPRECATED: Not currently used, can be removed or uncommented if needed
/*
func (h *AuthHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := h.getCurrentUser(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
*/

// getUserInfo returns JSON user info for the current user
func (h *AuthHandler) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		}); err != nil {
			log.Printf("Error encoding JSON response: %v", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated":       true,
		"id":                  user.ID,
		"username":            user.Username,
		"email":               user.Email,
		"provider":            user.OAuthProvider,
		"first_name":          user.FirstName,
		"last_name":           user.LastName,
		"profile_picture_url": user.ProfilePictureURL,
		"location_city":       user.LocationCity,
		"location_state":      user.LocationState,
		"default_translation": user.DefaultTranslation,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// handleSaveReadingPosition saves the user's current reading position
func (h *AuthHandler) handleSaveReadingPosition(w http.ResponseWriter, r *http.Request) {
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
		Translation string `json:"translation"`
		Book        string `json:"book"`
		Chapter     int    `json:"chapter"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Translation == "" || req.Book == "" || req.Chapter < 1 {
		http.Error(w, "Invalid reading position", http.StatusBadRequest)
		return
	}

	if err := h.db.SaveReadingPosition(user.ID, req.Translation, req.Book, req.Chapter); err != nil {
		log.Printf("Failed to save reading position: %v", err)
		http.Error(w, "Failed to save reading position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// handleGetReadingPosition retrieves the user's saved reading position
func (h *AuthHandler) handleGetReadingPosition(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		}); err != nil {
			log.Printf("Error encoding JSON response: %v", err)
		}
		return
	}

	pos, err := h.db.GetReadingPosition(user.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No saved position
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"authenticated": true,
				"hasPosition":   false,
			}); err != nil {
				log.Printf("Error encoding JSON response: %v", err)
			}
			return
		}
		log.Printf("Failed to get reading position: %v", err)
		http.Error(w, "Failed to get reading position", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"hasPosition":   true,
		"translation":   pos.Translation,
		"book":          pos.Book,
		"chapter":       pos.Chapter,
	}); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// splitName splits a full name into first and last name
func splitName(fullName string) (string, string) {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return "", ""
	}

	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}

	// First part is first name, rest is last name
	firstName := parts[0]
	lastName := strings.Join(parts[1:], " ")
	return firstName, lastName
}
