package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// StudyGroup represents a study group
type StudyGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	LogoURL   string    `json:"logo_url,omitempty"`
	CreatedBy int64     `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// StudyPlan represents a weekly reading schedule for a group
type StudyPlan struct {
	ID           int64  `json:"id"`
	GroupID      int64  `json:"group_id"`
	WeekNumber   int    `json:"week_number"`
	StartDate    string `json:"start_date"` // YYYY-MM-DD format
	EndDate      string `json:"end_date"`   // YYYY-MM-DD format
	Book         string `json:"book"`
	StartChapter int    `json:"start_chapter"`
	EndChapter   int    `json:"end_chapter"`
	Description  string `json:"description,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// GroupMember represents a group membership
type GroupMember struct {
	GroupID  int64     `json:"group_id"`
	UserID   int64     `json:"user_id"`
	Role     string    `json:"role"` // "admin" or "member"
	JoinedAt time.Time `json:"joined_at"`
	Username string    `json:"username"` // Added for display
}

// GroupInvite represents a pending group invitation
type GroupInvite struct {
	ID                int64     `json:"id"`
	GroupID           int64     `json:"group_id"`
	InvitedBy         int64     `json:"invited_by"`
	Email             string    `json:"email"`
	Token             string    `json:"token"`
	ExpiresAt         time.Time `json:"expires_at"`
	CreatedAt         time.Time `json:"created_at"`
	GroupName         string    `json:"group_name"`          // Added for display
	InvitedByUsername string    `json:"invited_by_username"` // Added for display
}

// CreateStudyGroup creates a new study group
func (d *Database) CreateStudyGroup(name, logoURL string, createdBy int64) (*StudyGroup, error) {
	result, err := d.db.Exec(
		`INSERT INTO study_groups (name, logo_url, created_by) VALUES (?, ?, ?)`,
		name, logoURL, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create study group: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get group ID: %w", err)
	}

	// Add creator as admin
	_, err = d.db.Exec(
		`INSERT INTO group_members (group_id, user_id, role) VALUES (?, ?, 'admin')`,
		id, createdBy,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add creator as admin: %w", err)
	}

	return d.GetStudyGroup(id)
}

// GetStudyGroup retrieves a study group by ID
func (d *Database) GetStudyGroup(id int64) (*StudyGroup, error) {
	group := &StudyGroup{}
	err := d.db.QueryRow(
		`SELECT id, name, COALESCE(logo_url, ''), created_by, created_at FROM study_groups WHERE id = ?`,
		id,
	).Scan(&group.ID, &group.Name, &group.LogoURL, &group.CreatedBy, &group.CreatedAt)
	if err != nil {
		return nil, err
	}
	return group, nil
}

// GetUserGroups retrieves all groups a user is a member of
func (d *Database) GetUserGroups(userID int64) ([]*StudyGroup, error) {
	rows, err := d.db.Query(
		`SELECT g.id, g.name, COALESCE(g.logo_url, ''), g.created_by, g.created_at
		 FROM study_groups g
		 INNER JOIN group_members gm ON g.id = gm.group_id
		 WHERE gm.user_id = ?
		 ORDER BY g.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var groups []*StudyGroup
	for rows.Next() {
		group := &StudyGroup{}
		if err := rows.Scan(&group.ID, &group.Name, &group.LogoURL, &group.CreatedBy, &group.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// GetGroupMembers retrieves all members of a group
func (d *Database) GetGroupMembers(groupID int64) ([]*GroupMember, error) {
	rows, err := d.db.Query(
		`SELECT u.username, gm.role, gm.joined_at
		 FROM users u
		 INNER JOIN group_members gm ON u.id = gm.user_id
		 WHERE gm.group_id = ?
		 ORDER BY gm.joined_at`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var members []*GroupMember

	for rows.Next() {
		member := &GroupMember{GroupID: groupID}
		if err := rows.Scan(&member.Username, &member.Role, &member.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

// IsGroupMember checks if a user is a member of a group
func (d *Database) IsGroupMember(groupID, userID int64) (bool, error) {
	var count int
	err := d.db.QueryRow(
		`SELECT COUNT(*) FROM group_members WHERE group_id = ? AND user_id = ?`,
		groupID, userID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsGroupAdmin checks if a user is an admin of a group
func (d *Database) IsGroupAdmin(groupID, userID int64) (bool, error) {
	var role string
	err := d.db.QueryRow(
		`SELECT role FROM group_members WHERE group_id = ? AND user_id = ?`,
		groupID, userID,
	).Scan(&role)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return role == "admin", nil
}

// CreateGroupInvite creates a new group invitation
func (d *Database) CreateGroupInvite(groupID, invitedBy int64, email, token string) (*GroupInvite, error) {
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	result, err := d.db.Exec(
		`INSERT INTO group_invites (group_id, invited_by, email, token, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		groupID, invitedBy, email, token, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create invite: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get invite ID: %w", err)
	}

	return d.GetGroupInvite(id)
}

// GetGroupInvite retrieves an invite by ID
func (d *Database) GetGroupInvite(id int64) (*GroupInvite, error) {
	invite := &GroupInvite{}
	err := d.db.QueryRow(
		`SELECT id, group_id, invited_by, email, token, expires_at, created_at
		 FROM group_invites WHERE id = ?`,
		id,
	).Scan(&invite.ID, &invite.GroupID, &invite.InvitedBy, &invite.Email,
		&invite.Token, &invite.ExpiresAt, &invite.CreatedAt)
	if err != nil {
		return nil, err
	}
	return invite, nil
}

// GetGroupInviteByToken retrieves an invite by its token
func (d *Database) GetGroupInviteByToken(token string) (*GroupInvite, error) {
	invite := &GroupInvite{}
	err := d.db.QueryRow(
		`SELECT id, group_id, invited_by, email, token, expires_at, created_at
		 FROM group_invites WHERE token = ? AND expires_at > ?`,
		token, time.Now(),
	).Scan(&invite.ID, &invite.GroupID, &invite.InvitedBy, &invite.Email,
		&invite.Token, &invite.ExpiresAt, &invite.CreatedAt)
	if err != nil {
		return nil, err
	}
	return invite, nil
}

// AcceptGroupInvite accepts an invitation and adds user to group
func (d *Database) AcceptGroupInvite(inviteID, userID int64) error {
	// Get invite
	invite, err := d.GetGroupInvite(inviteID)
	if err != nil {
		return err
	}

	// Check if expired
	if time.Now().After(invite.ExpiresAt) {
		return fmt.Errorf("invite has expired")
	}

	// Check if user is already a member
	isMember, err := d.IsGroupMember(invite.GroupID, userID)
	if err != nil {
		return fmt.Errorf("failed to check membership: %w", err)
	}
	if isMember {
		// User is already a member, just delete the invite
		_, err = d.db.Exec(`DELETE FROM group_invites WHERE id = ?`, inviteID)
		return err
	}

	// Add user to group
	_, err = d.db.Exec(
		`INSERT INTO group_members (group_id, user_id, role) VALUES (?, ?, 'member')`,
		invite.GroupID, userID,
	)
	if err != nil {
		return fmt.Errorf("failed to add user to group: %w", err)
	}

	// Delete the invite
	_, err = d.db.Exec(`DELETE FROM group_invites WHERE id = ?`, inviteID)
	return err
}

// DeclineGroupInvite deletes an invitation without adding user to group
func (d *Database) DeclineGroupInvite(inviteID int64) error {
	_, err := d.db.Exec(`DELETE FROM group_invites WHERE id = ?`, inviteID)
	return err
}

// GetPendingInvites retrieves all pending invites for a user's email
func (d *Database) GetPendingInvites(email string) ([]*GroupInvite, error) {
	rows, err := d.db.Query(
		`SELECT gi.id, gi.group_id, gi.invited_by, gi.email, gi.token, gi.expires_at, gi.created_at,
		        g.name as group_name, u.username as invited_by_username
		 FROM group_invites gi
		 INNER JOIN study_groups g ON gi.group_id = g.id
		 INNER JOIN users u ON gi.invited_by = u.id
		 WHERE gi.email = ? AND gi.expires_at > ?
		 ORDER BY gi.created_at DESC`,
		email, time.Now(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var invites []*GroupInvite
	for rows.Next() {
		invite := &GroupInvite{}
		if err := rows.Scan(&invite.ID, &invite.GroupID, &invite.InvitedBy, &invite.Email,
			&invite.Token, &invite.ExpiresAt, &invite.CreatedAt,
			&invite.GroupName, &invite.InvitedByUsername); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, nil
}

// RemoveGroupMember removes a user from a group
func (d *Database) RemoveGroupMember(groupID, userID int64) error {
	_, err := d.db.Exec(
		`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`,
		groupID, userID,
	)
	return err
}

// UpdateGroupName updates a group's name
func (d *Database) UpdateGroupName(groupID int64, name string) error {
	_, err := d.db.Exec(
		`UPDATE study_groups SET name = ? WHERE id = ?`,
		name, groupID,
	)
	return err
}

// UpdateGroupLogo updates a group's logo URL
func (d *Database) UpdateGroupLogo(groupID int64, logoURL string) error {
	_, err := d.db.Exec(
		`UPDATE study_groups SET logo_url = ? WHERE id = ?`,
		logoURL, groupID,
	)
	return err
}

// CreateStudyPlan creates a new study plan for a group
func (d *Database) CreateStudyPlan(groupID int64, weekNumber int, startDate, endDate, book string, startChapter, endChapter int, description string) (*StudyPlan, error) {
	result, err := d.db.Exec(
		`INSERT INTO study_plans (group_id, week_number, start_date, end_date, book, start_chapter, end_chapter, description)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		groupID, weekNumber, startDate, endDate, book, startChapter, endChapter, description,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create study plan: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get study plan ID: %w", err)
	}

	return d.GetStudyPlan(id)
}

// GetStudyPlan retrieves a study plan by ID
func (d *Database) GetStudyPlan(id int64) (*StudyPlan, error) {
	plan := &StudyPlan{}
	err := d.db.QueryRow(
		`SELECT id, group_id, week_number, start_date, end_date, book, start_chapter, end_chapter, COALESCE(description, ''), created_at
		 FROM study_plans WHERE id = ?`,
		id,
	).Scan(&plan.ID, &plan.GroupID, &plan.WeekNumber, &plan.StartDate, &plan.EndDate,
		&plan.Book, &plan.StartChapter, &plan.EndChapter, &plan.Description, &plan.CreatedAt)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// GetGroupStudyPlans retrieves all study plans for a group
func (d *Database) GetGroupStudyPlans(groupID int64) ([]*StudyPlan, error) {
	rows, err := d.db.Query(
		`SELECT id, group_id, week_number, start_date, end_date, book, start_chapter, end_chapter, COALESCE(description, ''), created_at
		 FROM study_plans WHERE group_id = ? ORDER BY week_number`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var plans []*StudyPlan
	for rows.Next() {
		plan := &StudyPlan{}
		if err := rows.Scan(&plan.ID, &plan.GroupID, &plan.WeekNumber, &plan.StartDate, &plan.EndDate,
			&plan.Book, &plan.StartChapter, &plan.EndChapter, &plan.Description, &plan.CreatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// UpdateStudyPlan updates a study plan
func (d *Database) UpdateStudyPlan(id int64, weekNumber int, startDate, endDate, book string, startChapter, endChapter int, description string) error {
	_, err := d.db.Exec(
		`UPDATE study_plans SET week_number = ?, start_date = ?, end_date = ?, book = ?, start_chapter = ?, end_chapter = ?, description = ?
		 WHERE id = ?`,
		weekNumber, startDate, endDate, book, startChapter, endChapter, description, id,
	)
	return err
}

// DeleteStudyPlan deletes a study plan
func (d *Database) DeleteStudyPlan(id int64) error {
	_, err := d.db.Exec(`DELETE FROM study_plans WHERE id = ?`, id)
	return err
}

// DeleteStudyGroup deletes a group and all associated data
func (d *Database) DeleteStudyGroup(groupID int64) error {
	// Note: Foreign keys with CASCADE will handle cleanup
	_, err := d.db.Exec(`DELETE FROM study_groups WHERE id = ?`, groupID)
	return err
}
