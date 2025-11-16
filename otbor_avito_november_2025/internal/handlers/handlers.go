package handlers

import (
	"otbor_avito_november_2025/internal/api"
	"otbor_avito_november_2025/internal/service"
	"otbor_avito_november_2025/internal/store"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *service.Service
}

func NewHandler(service *service.Service) *Handler {
	return &Handler{service: service}
}

var _ api.ServerInterface = (*Handler)(nil)

func (h *Handler) PostPullRequestCreate(ctx echo.Context) error {
	var req api.PostPullRequestCreateJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, createError("INVALID_REQUEST", "Invalid request body"))
	}

	pr, err := h.service.CreatePR(ctx.Request().Context(), req.PullRequestId, req.PullRequestName, req.AuthorId)
	if err != nil {
		return handleServiceError(ctx, err)
	}

	return ctx.JSON(201, map[string]interface{}{
		"pr": convertPullRequestToAPI(pr),
	})
}

func (h *Handler) PostPullRequestMerge(ctx echo.Context) error {
	var req api.PostPullRequestMergeJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, createError("INVALID_REQUEST", "Invalid request body"))
	}

	pr, err := h.service.MergePR(ctx.Request().Context(), req.PullRequestId)
	if err != nil {
		return handleServiceError(ctx, err)
	}

	return ctx.JSON(200, map[string]interface{}{
		"pr": convertPullRequestToAPI(pr),
	})
}

func (h *Handler) PostPullRequestReassign(ctx echo.Context) error {
	var req api.PostPullRequestReassignJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, createError("INVALID_REQUEST", "Invalid request body"))
	}

	pr, replacedBy, err := h.service.ReassignReviewer(ctx.Request().Context(), req.PullRequestId, req.OldUserId)
	if err != nil {
		return handleServiceError(ctx, err)
	}

	return ctx.JSON(200, map[string]interface{}{
		"pr":          convertPullRequestToAPI(pr),
		"replaced_by": replacedBy,
	})
}

func (h *Handler) PostTeamAdd(ctx echo.Context) error {
	var req api.Team
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, createError("INVALID_REQUEST", "Invalid request body"))
	}

	members := make([]service.TeamMember, len(req.Members))
	for i, m := range req.Members {
		members[i] = service.TeamMember{
			UserID:   m.UserId,
			Username: m.Username,
			IsActive: m.IsActive,
		}
	}

	team, err := h.service.CreateOrUpdateTeam(ctx.Request().Context(), req.TeamName, members)
	if err != nil {
		if err == service.ErrTeamExists {
			return ctx.JSON(400, createError("TEAM_EXISTS", err.Error()))
		}
		return ctx.JSON(500, createError("INTERNAL_ERROR", err.Error()))
	}

	_, teamMembers, err := h.service.GetTeam(ctx.Request().Context(), req.TeamName)
	if err != nil {
		return ctx.JSON(500, createError("INTERNAL_ERROR", err.Error()))
	}

	apiMembers := make([]api.TeamMember, len(teamMembers))
	for i, m := range teamMembers {
		apiMembers[i] = api.TeamMember{
			UserId:   m.UserID,
			Username: m.Username,
			IsActive: m.IsActive,
		}
	}

	response := api.Team{
		TeamName: team.Name,
		Members:  apiMembers,
	}

	return ctx.JSON(201, map[string]interface{}{
		"team": response,
	})
}

func (h *Handler) GetTeamGet(ctx echo.Context, params api.GetTeamGetParams) error {
	team, members, err := h.service.GetTeam(ctx.Request().Context(), params.TeamName)
	if err != nil {
		return ctx.JSON(404, createError("NOT_FOUND", err.Error()))
	}

	apiMembers := make([]api.TeamMember, len(members))
	for i, m := range members {
		apiMembers[i] = api.TeamMember{
			UserId:   m.UserID,
			Username: m.Username,
			IsActive: m.IsActive,
		}
	}

	response := api.Team{
		TeamName: team.Name,
		Members:  apiMembers,
	}

	return ctx.JSON(200, response)
}

func (h *Handler) GetUsersGetReview(ctx echo.Context, params api.GetUsersGetReviewParams) error {
	prs, err := h.service.GetUserAssignedPRs(ctx.Request().Context(), params.UserId)
	if err != nil {
		return ctx.JSON(404, createError("NOT_FOUND", err.Error()))
	}

	shortPRs := make([]api.PullRequestShort, len(prs))
	for i, pr := range prs {
		shortPRs[i] = api.PullRequestShort{
			PullRequestId:   pr.PullRequest.PullRequestID,
			PullRequestName: pr.PullRequest.PullRequestName,
			AuthorId:        pr.PullRequest.AuthorID,
			Status:          api.PullRequestShortStatus(pr.PullRequest.Status),
		}
	}

	return ctx.JSON(200, map[string]interface{}{
		"user_id":       params.UserId,
		"pull_requests": shortPRs,
	})
}

func (h *Handler) PostUsersSetIsActive(ctx echo.Context) error {
	var req api.PostUsersSetIsActiveJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(400, createError("INVALID_REQUEST", "Invalid request body"))
	}

	user, err := h.service.SetUserActive(ctx.Request().Context(), req.UserId, req.IsActive)
	if err != nil {
		return ctx.JSON(404, createError("NOT_FOUND", err.Error()))
	}

	response := api.User{
		UserId:   user.UserID,
		Username: user.Username,
		TeamName: user.TeamName,
		IsActive: user.IsActive,
	}

	return ctx.JSON(200, map[string]interface{}{
		"user": response,
	})
}

func createError(code, message string) api.ErrorResponse {
	return api.ErrorResponse{
		Error: struct {
			Code    api.ErrorResponseErrorCode `json:"code"`
			Message string                     `json:"message"`
		}{
			Code:    api.ErrorResponseErrorCode(code),
			Message: message,
		},
	}
}

func handleServiceError(ctx echo.Context, err error) error {
	switch err {
	case service.ErrPRExists:
		return ctx.JSON(409, createError("PR_EXISTS", err.Error()))
	case service.ErrPRMerged:
		return ctx.JSON(409, createError("PR_MERGED", err.Error()))
	case service.ErrNotAssigned:
		return ctx.JSON(409, createError("NOT_ASSIGNED", err.Error()))
	case service.ErrNoCandidate:
		return ctx.JSON(409, createError("NO_CANDIDATE", err.Error()))
	case service.ErrNotFound:
		return ctx.JSON(404, createError("NOT_FOUND", err.Error()))
	default:
		return ctx.JSON(500, createError("INTERNAL_ERROR", err.Error()))
	}
}

func convertPullRequestToAPI(pr *service.PullRequestWithReviewers) api.PullRequest {
	assignedReviewers := getUserIDs(pr.AssignedReviewers)

	return api.PullRequest{
		PullRequestId:     pr.PullRequest.PullRequestID,
		PullRequestName:   pr.PullRequest.PullRequestName,
		AuthorId:          pr.PullRequest.AuthorID,
		Status:            api.PullRequestStatus(pr.PullRequest.Status),
		AssignedReviewers: assignedReviewers,
		CreatedAt:         &pr.PullRequest.CreatedAt,
		MergedAt:          pr.PullRequest.MergedAt,
	}
}

func getUserIDs(users []store.User) []string {
	ids := make([]string, len(users))
	for i, user := range users {
		ids[i] = user.UserID
	}
	return ids
}
