package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ToxicSozo/GoDraw/internal/store"
)

type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type teamMemberPayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

type teamPayload struct {
	TeamName string              `json:"team_name"`
	Members  []teamMemberPayload `json:"members"`
}

type teamAddRequest teamPayload

type teamAddResponse struct {
	Team teamPayload `json:"team"`
}

type setIsActiveRequest struct {
	UserID   string `json:"user_id"`
	IsActive *bool  `json:"is_active"`
}

type userPayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	TeamName string `json:"team_name"`
	IsActive bool   `json:"is_active"`
}

type setIsActiveResponse struct {
	User userPayload `json:"user"`
}

type createPullRequestRequest struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
}

type pullRequestResponse struct {
	PullRequestID     string   `json:"pull_request_id"`
	PullRequestName   string   `json:"pull_request_name"`
	AuthorID          string   `json:"author_id"`
	Status            string   `json:"status"`
	AssignedReviewers []string `json:"assigned_reviewers"`
	CreatedAt         *string  `json:"createdAt,omitempty"`
	MergedAt          *string  `json:"mergedAt,omitempty"`
}

type createPullRequestResponse struct {
	PR pullRequestResponse `json:"pr"`
}

type mergePullRequestRequest struct {
	PullRequestID string `json:"pull_request_id"`
}

type mergePullRequestResponse struct {
	PR pullRequestResponse `json:"pr"`
}

type reassignRequest struct {
	PullRequestID string `json:"pull_request_id"`
	OldUserID     string `json:"old_user_id"`
}

type reassignResponse struct {
	PR         pullRequestResponse `json:"pr"`
	ReplacedBy string              `json:"replaced_by"`
}

type userReviewsResponse struct {
	UserID       string             `json:"user_id"`
	PullRequests []pullRequestShort `json:"pull_requests"`
}

type pullRequestShort struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
	Status          string `json:"status"`
}

func New(store *store.Store) *Server {
	s := &Server{
		store: store,
		mux:   http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/team/add", s.handleTeamAdd)
	s.mux.HandleFunc("/team/get", s.handleTeamGet)
	s.mux.HandleFunc("/users/setIsActive", s.handleSetIsActive)
	s.mux.HandleFunc("/pullRequest/create", s.handleCreatePullRequest)
	s.mux.HandleFunc("/pullRequest/merge", s.handleMergePullRequest)
	s.mux.HandleFunc("/pullRequest/reassign", s.handleReassign)
	s.mux.HandleFunc("/users/getReview", s.handleUserReviews)
}

func (s *Server) handleTeamAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req teamAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON payload")
		return
	}

	if req.TeamName == "" {
		badRequest(w, "team_name is required")
		return
	}

	members := make([]store.TeamMemberInput, 0, len(req.Members))
	for _, m := range req.Members {
		if m.UserID == "" {
			continue
		}
		members = append(members, store.TeamMemberInput{
			UserID:   m.UserID,
			Username: m.Username,
			IsActive: m.IsActive,
		})
	}

	team, err := s.store.CreateTeam(req.TeamName, members)
	if err != nil {
		if errors.Is(err, store.ErrTeamExists) {
			writeError(w, http.StatusBadRequest, "TEAM_EXISTS", "team_name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	resp := teamAddResponse{Team: makeTeamPayload(team)}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleTeamGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		badRequest(w, "team_name is required")
		return
	}

	team, err := s.store.GetTeam(teamName)
	if err != nil {
		if errors.Is(err, store.ErrTeamNotFound) {
			writeNotFound(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, makeTeamPayload(team))
}

func (s *Server) handleSetIsActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req setIsActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON payload")
		return
	}

	if req.UserID == "" || req.IsActive == nil {
		badRequest(w, "user_id and is_active are required")
		return
	}

	user, err := s.store.SetUserActive(req.UserID, *req.IsActive)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeNotFound(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	resp := setIsActiveResponse{User: makeUserPayload(user)}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreatePullRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req createPullRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON payload")
		return
	}

	if req.PullRequestID == "" || req.PullRequestName == "" || req.AuthorID == "" {
		badRequest(w, "pull_request_id, pull_request_name, and author_id are required")
		return
	}

	pr, err := s.store.CreatePullRequest(store.CreatePullRequestInput{
		ID:       req.PullRequestID,
		Name:     req.PullRequestName,
		AuthorID: req.AuthorID,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrPullRequestExists):
			writeError(w, http.StatusConflict, "PR_EXISTS", "pull request id already exists")
		case errors.Is(err, store.ErrUserNotFound), errors.Is(err, store.ErrTeamNotFound):
			writeNotFound(w)
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		}
		return
	}

	resp := createPullRequestResponse{PR: makePullRequestResponse(pr)}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleMergePullRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req mergePullRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON payload")
		return
	}

	if req.PullRequestID == "" {
		badRequest(w, "pull_request_id is required")
		return
	}

	pr, err := s.store.MergePullRequest(req.PullRequestID)
	if err != nil {
		if errors.Is(err, store.ErrPullRequestNotFound) {
			writeNotFound(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	resp := mergePullRequestResponse{PR: makePullRequestResponse(pr)}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleReassign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req reassignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON payload")
		return
	}

	if req.PullRequestID == "" || req.OldUserID == "" {
		badRequest(w, "pull_request_id and old_user_id are required")
		return
	}

	result, err := s.store.ReassignReviewer(req.PullRequestID, req.OldUserID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrPullRequestNotFound), errors.Is(err, store.ErrUserNotFound), errors.Is(err, store.ErrTeamNotFound):
			writeNotFound(w)
		case errors.Is(err, store.ErrPullRequestMerged):
			writeError(w, http.StatusConflict, "PR_MERGED", "cannot reassign on merged PR")
		case errors.Is(err, store.ErrReviewerNotAssigned):
			writeError(w, http.StatusConflict, "NOT_ASSIGNED", "reviewer is not assigned to this PR")
		case errors.Is(err, store.ErrNoReplacementCandidate):
			writeError(w, http.StatusConflict, "NO_CANDIDATE", "no active replacement candidate in team")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		}
		return
	}

	resp := reassignResponse{PR: makePullRequestResponse(result.PR), ReplacedBy: result.ReplacedBy}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUserReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		badRequest(w, "user_id is required")
		return
	}

	prs, err := s.store.ListPullRequestsByReviewer(userID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeNotFound(w)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	resp := userReviewsResponse{
		UserID:       userID,
		PullRequests: make([]pullRequestShort, 0, len(prs)),
	}
	for _, pr := range prs {
		resp.PullRequests = append(resp.PullRequests, pullRequestShort{
			PullRequestID:   pr.ID,
			PullRequestName: pr.Name,
			AuthorID:        pr.AuthorID,
			Status:          pr.Status,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

func makeTeamPayload(team *store.Team) teamPayload {
	payload := teamPayload{
		TeamName: team.Name,
		Members:  make([]teamMemberPayload, 0, len(team.Members)),
	}
	for _, member := range team.Members {
		payload.Members = append(payload.Members, teamMemberPayload{
			UserID:   member.UserID,
			Username: member.Username,
			IsActive: member.IsActive,
		})
	}
	return payload
}

func makeUserPayload(user *store.User) userPayload {
	return userPayload{
		UserID:   user.ID,
		Username: user.Username,
		TeamName: user.TeamName,
		IsActive: user.IsActive,
	}
}

func makePullRequestResponse(pr *store.PullRequest) pullRequestResponse {
	resp := pullRequestResponse{
		PullRequestID:     pr.ID,
		PullRequestName:   pr.Name,
		AuthorID:          pr.AuthorID,
		Status:            pr.Status,
		AssignedReviewers: append([]string(nil), pr.AssignedReviewers...),
	}

	if !pr.CreatedAt.IsZero() {
		created := pr.CreatedAt.Format(time.RFC3339)
		resp.CreatedAt = &created
	}
	if pr.MergedAt != nil {
		merged := pr.MergedAt.Format(time.RFC3339)
		resp.MergedAt = &merged
	}

	return resp
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = message
	_ = json.NewEncoder(w).Encode(body)
}

func writeNotFound(w http.ResponseWriter) {
	writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
}

func badRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "BAD_REQUEST", message)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
