package service

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"otbor_avito_november_2025/internal/store"
)

var (
	ErrTeamExists  = errors.New("team_name already exists")
	ErrPRExists    = errors.New("PR id already exists")
	ErrPRMerged    = errors.New("cannot reassign on merged PR")
	ErrNotAssigned = errors.New("reviewer is not assigned to this PR")
	ErrNoCandidate = errors.New("no active replacement candidate in team")
	ErrNotFound    = errors.New("resource not found")
)

type TeamMember struct {
	UserID   string
	Username string
	IsActive bool
}

type PullRequestWithReviewers struct {
	PullRequest       *store.PullRequest
	AssignedReviewers []store.User
}

type Service struct {
	store *store.PostgresStore
}

func NewService(store *store.PostgresStore) *Service {
	rand.Seed(time.Now().UnixNano())
	return &Service{store: store}
}

func (s *Service) CreateOrUpdateTeam(ctx context.Context, teamName string, members []TeamMember) (*store.Team, error) {
	existingTeam, err := s.store.GetTeam(ctx, teamName)
	if err == nil && existingTeam != nil {
		return nil, ErrTeamExists
	}

	team := &store.Team{Name: teamName}
	if err := s.store.CreateTeam(ctx, team); err != nil {
		return nil, err
	}

	for _, member := range members {
		user := &store.User{
			UserID:   member.UserID,
			Username: member.Username,
			IsActive: member.IsActive,
			TeamName: teamName,
		}
		if err := s.store.CreateOrUpdateUser(ctx, user); err != nil {
			return nil, err
		}
	}

	return team, nil
}

func (s *Service) GetTeam(ctx context.Context, teamName string) (*store.Team, []store.User, error) {
	team, err := s.store.GetTeam(ctx, teamName)
	if err != nil {
		return nil, nil, err
	}
	if team == nil {
		return nil, nil, ErrNotFound
	}

	members, err := s.store.GetTeamMembers(ctx, teamName)
	if err != nil {
		return nil, nil, err
	}

	return team, members, nil
}

func (s *Service) SetUserActive(ctx context.Context, userID string, isActive bool) (*store.User, error) {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	user.IsActive = isActive
	if err := s.store.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) CreatePR(ctx context.Context, prID, prName, authorID string) (*PullRequestWithReviewers, error) {
	existingPR, err := s.store.GetPR(ctx, prID)
	if err != nil {
		return nil, err
	}
	if existingPR != nil {
		return nil, ErrPRExists
	}

	author, err := s.store.GetUser(ctx, authorID)
	if err != nil {
		return nil, err
	}
	if author == nil {
		return nil, ErrNotFound
	}

	activeMembers, err := s.store.GetActiveTeamMembers(ctx, author.TeamName, &authorID)
	if err != nil {
		return nil, err
	}

	var reviewers []store.User
	if len(activeMembers) > 0 {
		count := min(2, len(activeMembers))
		shuffled := make([]store.User, len(activeMembers))
		copy(shuffled, activeMembers)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		reviewers = shuffled[:count]
	}

	pr := &store.PullRequest{
		PullRequestID:   prID,
		PullRequestName: prName,
		AuthorID:        authorID,
		Status:          store.PRStatusOpen,
		CreatedAt:       time.Now(),
	}

	if err := s.store.CreatePR(ctx, pr); err != nil {
		return nil, err
	}

	for _, reviewer := range reviewers {
		if err := s.store.AssignReviewer(ctx, prID, reviewer.UserID); err != nil {
			return nil, err
		}
	}

	return &PullRequestWithReviewers{
		PullRequest:       pr,
		AssignedReviewers: reviewers,
	}, nil
}

func (s *Service) MergePR(ctx context.Context, prID string) (*PullRequestWithReviewers, error) {
	pr, err := s.store.GetPR(ctx, prID)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, ErrNotFound
	}

	if pr.Status == store.PRStatusMerged {
		reviewers, err := s.store.GetPRReviewers(ctx, prID)
		if err != nil {
			return nil, err
		}
		return &PullRequestWithReviewers{
			PullRequest:       pr,
			AssignedReviewers: reviewers,
		}, nil
	}

	now := time.Now()
	pr.Status = store.PRStatusMerged
	pr.MergedAt = &now

	if err := s.store.UpdatePR(ctx, pr); err != nil {
		return nil, err
	}

	reviewers, err := s.store.GetPRReviewers(ctx, prID)
	if err != nil {
		return nil, err
	}

	return &PullRequestWithReviewers{
		PullRequest:       pr,
		AssignedReviewers: reviewers,
	}, nil
}

func (s *Service) ReassignReviewer(ctx context.Context, prID, oldUserID string) (*PullRequestWithReviewers, string, error) {
	pr, err := s.store.GetPR(ctx, prID)
	if err != nil {
		return nil, "", err
	}
	if pr == nil {
		return nil, "", ErrNotFound
	}

	if pr.Status == store.PRStatusMerged {
		return nil, "", ErrPRMerged
	}

	currentReviewers, err := s.store.GetPRReviewers(ctx, prID)
	if err != nil {
		return nil, "", err
	}

	isAssigned := false
	for _, reviewer := range currentReviewers {
		if reviewer.UserID == oldUserID {
			isAssigned = true
			break
		}
	}
	if !isAssigned {
		return nil, "", ErrNotAssigned
	}

	oldReviewer, err := s.store.GetUser(ctx, oldUserID)
	if err != nil {
		return nil, "", err
	}
	if oldReviewer == nil {
		return nil, "", ErrNotFound
	}

	activeMembers, err := s.store.GetActiveTeamMembers(ctx, oldReviewer.TeamName, &pr.AuthorID)
	if err != nil {
		return nil, "", err
	}

	var availableMembers []store.User
	currentReviewerMap := make(map[string]bool)
	for _, reviewer := range currentReviewers {
		currentReviewerMap[reviewer.UserID] = true
	}

	for _, member := range activeMembers {
		if !currentReviewerMap[member.UserID] && member.UserID != oldUserID {
			availableMembers = append(availableMembers, member)
		}
	}

	if len(availableMembers) == 0 {
		return nil, "", ErrNoCandidate
	}

	newReviewer := availableMembers[rand.Intn(len(availableMembers))]

	if err := s.store.RemoveReviewer(ctx, prID, oldUserID); err != nil {
		return nil, "", err
	}
	if err := s.store.AssignReviewer(ctx, prID, newReviewer.UserID); err != nil {
		return nil, "", err
	}

	updatedReviewers, err := s.store.GetPRReviewers(ctx, prID)
	if err != nil {
		return nil, "", err
	}

	result := &PullRequestWithReviewers{
		PullRequest:       pr,
		AssignedReviewers: updatedReviewers,
	}

	return result, newReviewer.UserID, nil
}

func (s *Service) GetUserAssignedPRs(ctx context.Context, userID string) ([]*PullRequestWithReviewers, error) {
	prs, err := s.store.GetUserAssignedPRs(ctx, userID)
	if err != nil {
		return nil, err
	}

	var result []*PullRequestWithReviewers
	for _, pr := range prs {
		reviewers, err := s.store.GetPRReviewers(ctx, pr.PullRequestID)
		if err != nil {
			return nil, err
		}
		result = append(result, &PullRequestWithReviewers{
			PullRequest:       &pr,
			AssignedReviewers: reviewers,
		})
	}

	return result, nil
}

func (s *Service) GetPR(ctx context.Context, prID string) (*PullRequestWithReviewers, error) {
	pr, err := s.store.GetPR(ctx, prID)
	if err != nil {
		return nil, err
	}
	if pr == nil {
		return nil, ErrNotFound
	}

	reviewers, err := s.store.GetPRReviewers(ctx, prID)
	if err != nil {
		return nil, err
	}

	return &PullRequestWithReviewers{
		PullRequest:       pr,
		AssignedReviewers: reviewers,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
