package store

import (
	"errors"
	"math/rand"
	"sort"
	"sync"
	"time"
)

const (
	StatusOpen   = "OPEN"
	StatusMerged = "MERGED"
)

var (
	ErrTeamExists             = errors.New("team already exists")
	ErrTeamNotFound           = errors.New("team not found")
	ErrUserNotFound           = errors.New("user not found")
	ErrPullRequestExists      = errors.New("pull request already exists")
	ErrPullRequestNotFound    = errors.New("pull request not found")
	ErrPullRequestMerged      = errors.New("pull request merged")
	ErrReviewerNotAssigned    = errors.New("reviewer not assigned")
	ErrNoReplacementCandidate = errors.New("no replacement candidate")
)

type TeamMemberInput struct {
	UserID   string
	Username string
	IsActive bool
}

type Team struct {
	Name    string
	Members []TeamMember
}

type TeamMember struct {
	UserID   string
	Username string
	IsActive bool
}

type User struct {
	ID       string
	Username string
	TeamName string
	IsActive bool
}

type PullRequest struct {
	ID                string
	Name              string
	AuthorID          string
	Status            string
	AssignedReviewers []string
	CreatedAt         time.Time
	MergedAt          *time.Time
}

type Store struct {
	mu    sync.RWMutex
	teams map[string]*teamRecord
	users map[string]*User
	prs   map[string]*PullRequest
	rnd   *rand.Rand
}

type teamRecord struct {
	Name    string
	Members map[string]struct{}
}

func New() *Store {
	return &Store{
		teams: make(map[string]*teamRecord),
		users: make(map[string]*User),
		prs:   make(map[string]*PullRequest),
		rnd:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Store) CreateTeam(name string, members []TeamMemberInput) (*Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.teams[name]; exists {
		return nil, ErrTeamExists
	}

	record := &teamRecord{
		Name:    name,
		Members: make(map[string]struct{}),
	}
	s.teams[name] = record

	for _, member := range members {
		if member.UserID == "" {
			continue
		}
		u := s.upsertUserLocked(member.UserID, member.Username, name, member.IsActive)
		record.Members[u.ID] = struct{}{}
	}

	return s.buildTeamLocked(record), nil
}

func (s *Store) GetTeam(name string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.teams[name]
	if !ok {
		return nil, ErrTeamNotFound
	}

	return s.buildTeamLocked(record), nil
}

func (s *Store) upsertUserLocked(id, username, teamName string, isActive bool) *User {
	user, ok := s.users[id]
	if !ok {
		user = &User{ID: id}
		s.users[id] = user
	}

	if user.TeamName != "" && user.TeamName != teamName {
		if oldTeam, ok := s.teams[user.TeamName]; ok {
			delete(oldTeam.Members, user.ID)
		}
	}

	user.Username = username
	user.TeamName = teamName
	user.IsActive = isActive

	return user
}

func (s *Store) SetUserActive(userID string, isActive bool) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	user.IsActive = isActive
	return cloneUser(user), nil
}

func (s *Store) GetUser(userID string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	return cloneUser(user), nil
}

type CreatePullRequestInput struct {
	ID       string
	Name     string
	AuthorID string
}

func (s *Store) CreatePullRequest(input CreatePullRequestInput) (*PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.prs[input.ID]; exists {
		return nil, ErrPullRequestExists
	}

	author, ok := s.users[input.AuthorID]
	if !ok {
		return nil, ErrUserNotFound
	}

	if author.TeamName == "" {
		return nil, ErrTeamNotFound
	}

	team, ok := s.teams[author.TeamName]
	if !ok {
		return nil, ErrTeamNotFound
	}

	reviewers := s.pickReviewersLocked(team, author.ID)

	now := time.Now().UTC()
	pr := &PullRequest{
		ID:                input.ID,
		Name:              input.Name,
		AuthorID:          author.ID,
		Status:            StatusOpen,
		AssignedReviewers: reviewers,
		CreatedAt:         now,
	}

	s.prs[pr.ID] = pr
	return clonePullRequest(pr), nil
}

func (s *Store) pickReviewersLocked(team *teamRecord, authorID string) []string {
	candidates := make([]string, 0, len(team.Members))
	for memberID := range team.Members {
		if memberID == authorID {
			continue
		}
		user := s.users[memberID]
		if user == nil || !user.IsActive {
			continue
		}
		candidates = append(candidates, memberID)
	}

	if len(candidates) == 0 {
		return nil
	}

	s.rnd.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	limit := 2
	if len(candidates) < limit {
		limit = len(candidates)
	}

	reviewers := make([]string, 0, limit)
	reviewers = append(reviewers, candidates[:limit]...)
	return reviewers
}

func (s *Store) GetPullRequest(prID string) (*PullRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pr, ok := s.prs[prID]
	if !ok {
		return nil, ErrPullRequestNotFound
	}
	return clonePullRequest(pr), nil
}

func (s *Store) MergePullRequest(prID string) (*PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pr, ok := s.prs[prID]
	if !ok {
		return nil, ErrPullRequestNotFound
	}

	if pr.Status != StatusMerged {
		pr.Status = StatusMerged
		now := time.Now().UTC()
		if pr.MergedAt == nil {
			pr.MergedAt = &now
		}
	}

	return clonePullRequest(pr), nil
}

type ReassignResult struct {
	PR         *PullRequest
	ReplacedBy string
}

func (s *Store) ReassignReviewer(prID, oldReviewerID string) (*ReassignResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pr, ok := s.prs[prID]
	if !ok {
		return nil, ErrPullRequestNotFound
	}

	if pr.Status == StatusMerged {
		return nil, ErrPullRequestMerged
	}

	index := -1
	for i, reviewer := range pr.AssignedReviewers {
		if reviewer == oldReviewerID {
			index = i
			break
		}
	}
	if index == -1 {
		return nil, ErrReviewerNotAssigned
	}

	reviewer, ok := s.users[oldReviewerID]
	if !ok {
		return nil, ErrUserNotFound
	}
	if reviewer.TeamName == "" {
		return nil, ErrTeamNotFound
	}

	team, ok := s.teams[reviewer.TeamName]
	if !ok {
		return nil, ErrTeamNotFound
	}

	candidates := s.pickReplacementCandidatesLocked(team, pr.AuthorID, pr.AssignedReviewers, oldReviewerID)
	if len(candidates) == 0 {
		return nil, ErrNoReplacementCandidate
	}

	replacement := candidates[s.rnd.Intn(len(candidates))]
	pr.AssignedReviewers[index] = replacement

	return &ReassignResult{PR: clonePullRequest(pr), ReplacedBy: replacement}, nil
}

func (s *Store) pickReplacementCandidatesLocked(team *teamRecord, authorID string, assigned []string, skip string) []string {
	assignedSet := make(map[string]struct{}, len(assigned))
	for _, id := range assigned {
		if id != skip {
			assignedSet[id] = struct{}{}
		}
	}

	candidates := make([]string, 0, len(team.Members))
	for memberID := range team.Members {
		if memberID == skip || memberID == authorID {
			continue
		}
		if _, exists := assignedSet[memberID]; exists {
			continue
		}
		user := s.users[memberID]
		if user == nil || !user.IsActive {
			continue
		}
		candidates = append(candidates, memberID)
	}
	return candidates
}

func (s *Store) ListPullRequestsByReviewer(userID string) ([]*PullRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.users[userID]; !ok {
		return nil, ErrUserNotFound
	}

	result := make([]*PullRequest, 0)
	for _, pr := range s.prs {
		for _, reviewer := range pr.AssignedReviewers {
			if reviewer == userID {
				result = append(result, clonePullRequest(pr))
				break
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

func (s *Store) buildTeamLocked(record *teamRecord) *Team {
	members := make([]TeamMember, 0, len(record.Members))
	for memberID := range record.Members {
		user := s.users[memberID]
		if user == nil {
			continue
		}
		members = append(members, TeamMember{
			UserID:   user.ID,
			Username: user.Username,
			IsActive: user.IsActive,
		})
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].UserID < members[j].UserID
	})

	return &Team{
		Name:    record.Name,
		Members: members,
	}
}

func cloneUser(u *User) *User {
	if u == nil {
		return nil
	}
	clone := *u
	return &clone
}

func clonePullRequest(pr *PullRequest) *PullRequest {
	if pr == nil {
		return nil
	}
	clone := *pr
	if pr.AssignedReviewers != nil {
		clone.AssignedReviewers = make([]string, len(pr.AssignedReviewers))
		copy(clone.AssignedReviewers, pr.AssignedReviewers)
	}
	if pr.MergedAt != nil {
		ts := *pr.MergedAt
		clone.MergedAt = &ts
	}
	return &clone
}
