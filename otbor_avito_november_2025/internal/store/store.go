package store

import (
	"context"
	"database/sql"
	"time"
)

type Team struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type User struct {
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	IsActive  bool      `json:"is_active"`
	TeamName  string    `json:"team_name"`
	CreatedAt time.Time `json:"created_at"`
}

type PullRequestStatus string

const (
	PRStatusOpen   PullRequestStatus = "OPEN"
	PRStatusMerged PullRequestStatus = "MERGED"
)

type PullRequest struct {
	PullRequestID   string            `json:"pull_request_id"`
	PullRequestName string            `json:"pull_request_name"`
	AuthorID        string            `json:"author_id"`
	Status          PullRequestStatus `json:"status"`
	CreatedAt       time.Time         `json:"created_at"`
	MergedAt        *time.Time        `json:"merged_at"`
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) CreateTeam(ctx context.Context, team *Team) error {
	query := `INSERT INTO teams (name, created_at) VALUES ($1, $2)`
	_, err := s.db.ExecContext(ctx, query, team.Name, time.Now())
	return err
}

func (s *PostgresStore) GetTeam(ctx context.Context, name string) (*Team, error) {
	query := `SELECT name, created_at FROM teams WHERE name = $1`
	row := s.db.QueryRowContext(ctx, query, name)

	var team Team
	err := row.Scan(&team.Name, &team.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &team, nil
}

func (s *PostgresStore) GetTeamMembers(ctx context.Context, teamName string) ([]User, error) {
	query := `SELECT user_id, username, is_active, team_name, created_at FROM users WHERE team_name = $1`
	rows, err := s.db.QueryContext(ctx, query, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.UserID, &user.Username, &user.IsActive, &user.TeamName, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (s *PostgresStore) CreateOrUpdateUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (user_id, username, is_active, team_name, created_at) 
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) 
		DO UPDATE SET username = $2, is_active = $3, team_name = $4
	`
	_, err := s.db.ExecContext(ctx, query,
		user.UserID, user.Username, user.IsActive, user.TeamName, time.Now())
	return err
}

func (s *PostgresStore) GetUser(ctx context.Context, userID string) (*User, error) {
	query := `SELECT user_id, username, is_active, team_name, created_at FROM users WHERE user_id = $1`
	row := s.db.QueryRowContext(ctx, query, userID)

	var user User
	err := row.Scan(&user.UserID, &user.Username, &user.IsActive, &user.TeamName, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &user, err
}

func (s *PostgresStore) UpdateUser(ctx context.Context, user *User) error {
	query := `UPDATE users SET username = $1, is_active = $2, team_name = $3 WHERE user_id = $4`
	_, err := s.db.ExecContext(ctx, query, user.Username, user.IsActive, user.TeamName, user.UserID)
	return err
}

func (s *PostgresStore) GetActiveTeamMembers(ctx context.Context, teamName string, excludeUserID *string) ([]User, error) {
	query := `SELECT user_id, username, is_active, team_name, created_at FROM users WHERE team_name = $1 AND is_active = true`

	if excludeUserID != nil {
		query += " AND user_id != $2"
		rows, err := s.db.QueryContext(ctx, query, teamName, *excludeUserID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		return s.scanUsers(rows)
	}

	rows, err := s.db.QueryContext(ctx, query, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanUsers(rows)
}

func (s *PostgresStore) CreatePR(ctx context.Context, pr *PullRequest) error {
	query := `
		INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status, created_at) 
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := s.db.ExecContext(ctx, query,
		pr.PullRequestID, pr.PullRequestName, pr.AuthorID, pr.Status, time.Now())
	return err
}

func (s *PostgresStore) GetPR(ctx context.Context, prID string) (*PullRequest, error) {
	query := `SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at FROM pull_requests WHERE pull_request_id = $1`
	row := s.db.QueryRowContext(ctx, query, prID)

	var pr PullRequest
	var mergedAt sql.NullTime
	err := row.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &mergedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}
	return &pr, err
}

func (s *PostgresStore) UpdatePR(ctx context.Context, pr *PullRequest) error {
	query := `
		UPDATE pull_requests 
		SET pull_request_name = $1, status = $2, merged_at = $3 
		WHERE pull_request_id = $4
	`
	_, err := s.db.ExecContext(ctx, query,
		pr.PullRequestName, pr.Status, pr.MergedAt, pr.PullRequestID)
	return err
}

func (s *PostgresStore) AssignReviewer(ctx context.Context, prID, userID string) error {
	query := `INSERT INTO pr_reviewers (pull_request_id, user_id, assigned_at) VALUES ($1, $2, $3)`
	_, err := s.db.ExecContext(ctx, query, prID, userID, time.Now())
	return err
}

func (s *PostgresStore) GetPRReviewers(ctx context.Context, prID string) ([]User, error) {
	query := `
		SELECT u.user_id, u.username, u.is_active, u.team_name, u.created_at 
		FROM users u
		JOIN pr_reviewers pr ON u.user_id = pr.user_id
		WHERE pr.pull_request_id = $1
	`
	rows, err := s.db.QueryContext(ctx, query, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanUsers(rows)
}

func (s *PostgresStore) RemoveReviewer(ctx context.Context, prID, userID string) error {
	query := `DELETE FROM pr_reviewers WHERE pull_request_id = $1 AND user_id = $2`
	_, err := s.db.ExecContext(ctx, query, prID, userID)
	return err
}

func (s *PostgresStore) GetUserAssignedPRs(ctx context.Context, userID string) ([]PullRequest, error) {
	query := `
		SELECT p.pull_request_id, p.pull_request_name, p.author_id, p.status, p.created_at, p.merged_at
		FROM pull_requests p
		JOIN pr_reviewers pr ON p.pull_request_id = pr.pull_request_id
		WHERE pr.user_id = $1
	`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []PullRequest
	for rows.Next() {
		var pr PullRequest
		var mergedAt sql.NullTime
		err := rows.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status, &pr.CreatedAt, &mergedAt)
		if err != nil {
			return nil, err
		}
		if mergedAt.Valid {
			pr.MergedAt = &mergedAt.Time
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

func (s *PostgresStore) scanUsers(rows *sql.Rows) ([]User, error) {
	var users []User
	for rows.Next() {
		var user User
		err := rows.Scan(&user.UserID, &user.Username, &user.IsActive, &user.TeamName, &user.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}
