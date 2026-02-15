package wouldyou

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChallengeService handles WouldYouRather challenge logic.
type ChallengeService struct {
	db                *gorm.DB
	questionGenerator *QuestionGeneratorService
}

// NewChallengeService creates a new ChallengeService.
func NewChallengeService(db *gorm.DB, qg *QuestionGeneratorService) *ChallengeService {
	return &ChallengeService{db: db, questionGenerator: qg}
}

// GetDailyChallenge returns today's challenge, creating one with rotation if needed.
func (s *ChallengeService) GetDailyChallenge(appID string) (*Challenge, error) {
	today := time.Now().Truncate(24 * time.Hour)

	var challenge Challenge
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("is_daily = ? AND daily_date = ?", true, today).First(&challenge).Error
	if err == nil {
		return &challenge, nil
	}

	// Get recently used challenges (last 30 days) to avoid repeats
	var recentChallenges []Challenge
	thirtyDaysAgo := today.AddDate(0, 0, -30)
	s.db.Scopes(tenant.ForTenant(appID)).Where("is_daily = ? AND daily_date > ?", true, thirtyDaysAgo).Find(&recentChallenges)

	usedOptions := make(map[string]bool)
	for _, rc := range recentChallenges {
		usedOptions[rc.OptionA+"|"+rc.OptionB] = true
	}

	available := make([]int, 0)
	for i, c := range DailyChallenges {
		key := c.OptionA + "|" + c.OptionB
		if !usedOptions[key] {
			available = append(available, i)
		}
	}

	if len(available) == 0 {
		for i := range DailyChallenges {
			available = append(available, i)
		}
	}

	idx := available[rand.Intn(len(available))]
	c := DailyChallenges[idx]

	challenge = Challenge{
		AppID:     appID,
		OptionA:   c.OptionA,
		OptionB:   c.OptionB,
		Category:  c.Category,
		IsDaily:   true,
		DailyDate: today,
	}

	if err := s.db.Create(&challenge).Error; err != nil {
		return nil, err
	}

	return &challenge, nil
}

// GetChallengesByCategory returns challenges filtered by category.
func (s *ChallengeService) GetChallengesByCategory(appID string, category string, userID uuid.UUID, limit int) ([]map[string]interface{}, error) {
	s.ensureCategoryChallenges(appID, category)

	var challenges []Challenge
	query := s.db.Scopes(tenant.ForTenant(appID)).Where("category = ?", category).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	query.Find(&challenges)

	if len(challenges) < 5 && s.questionGenerator != nil && s.questionGenerator.IsAvailable() {
		log.Printf("Low challenge count for category %s (%d), generating more...", category, len(challenges))
		_, genErr := s.questionGenerator.GenerateBatch(appID, category, 10)
		if genErr != nil {
			log.Printf("Failed to generate challenges for category %s: %v", category, genErr)
		} else {
			challenges = nil
			query = s.db.Scopes(tenant.ForTenant(appID)).Where("category = ?", category).Order("created_at DESC")
			if limit > 0 {
				query = query.Limit(limit)
			}
			query.Find(&challenges)
		}
	}

	result := make([]map[string]interface{}, 0)
	for _, ch := range challenges {
		total := ch.VotesA + ch.VotesB
		percentA, percentB := 0, 0
		if total > 0 {
			percentA = (ch.VotesA * 100) / total
			percentB = (ch.VotesB * 100) / total
		}

		userChoice := ""
		if userID != uuid.Nil {
			var vote Vote
			if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND challenge_id = ?", userID, ch.ID).First(&vote).Error; err == nil {
				userChoice = vote.Choice
			}
		}

		result = append(result, map[string]interface{}{
			"challenge": ch, "user_choice": userChoice,
			"percent_a": percentA, "percent_b": percentB,
			"total_votes": total,
		})
	}

	return result, nil
}

func (s *ChallengeService) ensureCategoryChallenges(appID, category string) {
	var count int64
	s.db.Model(&Challenge{}).Scopes(tenant.ForTenant(appID)).Where("category = ? AND is_daily = ?", category, false).Count(&count)
	if count > 0 {
		return
	}

	for _, c := range DailyChallenges {
		if c.Category == category {
			ch := Challenge{
				AppID:    appID,
				OptionA:  c.OptionA,
				OptionB:  c.OptionB,
				Category: c.Category,
				IsDaily:  false,
			}
			s.db.Create(&ch)
		}
	}
}

// GetRandomChallenge returns a random non-daily challenge the user hasn't voted on.
func (s *ChallengeService) GetRandomChallenge(appID string, userID uuid.UUID) (*Challenge, error) {
	for _, cat := range []string{"life", "deep", "superpower", "funny", "love", "tech"} {
		s.ensureCategoryChallenges(appID, cat)
	}

	var challenge Challenge
	subQuery := s.db.Model(&Vote{}).Scopes(tenant.ForTenant(appID)).Select("challenge_id").Where("user_id = ?", userID)

	err := s.db.Scopes(tenant.ForTenant(appID)).Where("is_daily = ? AND id NOT IN (?)", false, subQuery).
		Order("RANDOM()").
		First(&challenge).Error

	if err == nil {
		return &challenge, nil
	}

	var totalCount int64
	s.db.Model(&Challenge{}).Scopes(tenant.ForTenant(appID)).Where("is_daily = ?", false).Count(&totalCount)

	if totalCount == 0 {
		if s.questionGenerator != nil && s.questionGenerator.IsAvailable() {
			_, genErr := s.questionGenerator.GenerateBatch(appID, "general", 10)
			if genErr != nil {
				return nil, errors.New("no challenges available and generation failed")
			}
			err = s.db.Scopes(tenant.ForTenant(appID)).Where("is_daily = ? AND id NOT IN (?)", false, subQuery).
				Order("RANDOM()").
				First(&challenge).Error
			if err == nil {
				return &challenge, nil
			}
		}
		return nil, errors.New("no challenges available")
	}

	err = s.db.Scopes(tenant.ForTenant(appID)).Where("is_daily = ?", false).Order("RANDOM()").First(&challenge).Error
	if err != nil {
		return nil, errors.New("no challenges available")
	}

	return &challenge, nil
}

// Vote records a user's or guest's vote.
func (s *ChallengeService) Vote(appID string, userID uuid.UUID, guestID string, challengeID uuid.UUID, choice string) (*Vote, error) {
	if choice != "A" && choice != "B" {
		return nil, errors.New("invalid choice, must be A or B")
	}

	if userID == uuid.Nil && guestID != "" {
		count := s.GetGuestVoteCount(appID, guestID, time.Now())
		if count >= 3 {
			return nil, errors.New("Daily free limit reached. Sign up for unlimited votes!")
		}

		var existing Vote
		if err := s.db.Scopes(tenant.ForTenant(appID)).Where("guest_id = ? AND challenge_id = ?", guestID, challengeID).First(&existing).Error; err == nil {
			return nil, errors.New("already voted on this challenge")
		}
	} else if userID != uuid.Nil {
		var existing Vote
		if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND challenge_id = ?", userID, challengeID).First(&existing).Error; err == nil {
			return nil, errors.New("already voted on this challenge")
		}
	} else {
		return nil, errors.New("authentication required")
	}

	vote := &Vote{
		AppID:       appID,
		UserID:      userID,
		GuestID:     guestID,
		ChallengeID: challengeID,
		Choice:      choice,
	}

	if err := s.db.Create(vote).Error; err != nil {
		return nil, err
	}

	if choice == "A" {
		s.db.Model(&Challenge{}).Where("id = ?", challengeID).Update("votes_a", gorm.Expr("votes_a + 1"))
	} else {
		s.db.Model(&Challenge{}).Where("id = ?", challengeID).Update("votes_b", gorm.Expr("votes_b + 1"))
	}

	if userID != uuid.Nil {
		s.updateStreak(appID, userID)
	}

	return vote, nil
}

// GetGuestVoteCount returns the number of votes a guest made on a given date.
func (s *ChallengeService) GetGuestVoteCount(appID, guestID string, date time.Time) int {
	startOfDay := date.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int64
	s.db.Model(&Vote{}).Scopes(tenant.ForTenant(appID)).
		Where("guest_id = ? AND created_at >= ? AND created_at < ?", guestID, startOfDay, endOfDay).
		Count(&count)

	return int(count)
}

// GetUserVote returns user's vote on a challenge.
func (s *ChallengeService) GetUserVote(appID string, userID, challengeID uuid.UUID) (*Vote, error) {
	var vote Vote
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND challenge_id = ?", userID, challengeID).First(&vote).Error; err != nil {
		return nil, err
	}
	return &vote, nil
}

// GetGuestVote returns guest's vote on a challenge.
func (s *ChallengeService) GetGuestVote(appID, guestID string, challengeID uuid.UUID) (*Vote, error) {
	var vote Vote
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("guest_id = ? AND challenge_id = ?", guestID, challengeID).First(&vote).Error; err != nil {
		return nil, err
	}
	return &vote, nil
}

func (s *ChallengeService) updateStreak(appID string, userID uuid.UUID) {
	today := time.Now().Truncate(24 * time.Hour)

	var streak ChallengeStreak
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error; err != nil {
		streak = ChallengeStreak{
			AppID:         appID,
			UserID:        userID,
			CurrentStreak: 1,
			LongestStreak: 1,
			TotalVotes:    1,
			LastVoteDate:  today,
		}
		s.db.Create(&streak)
		return
	}

	yesterday := today.AddDate(0, 0, -1)
	streak.TotalVotes++

	if streak.LastVoteDate.Equal(yesterday) {
		streak.CurrentStreak++
	} else if !streak.LastVoteDate.Equal(today) {
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}
	streak.LastVoteDate = today
	s.db.Save(&streak)
}

// GetStats returns user's voting stats.
func (s *ChallengeService) GetStats(appID string, userID uuid.UUID) (map[string]interface{}, error) {
	var streak ChallengeStreak
	s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak)

	return map[string]interface{}{
		"current_streak": streak.CurrentStreak,
		"longest_streak": streak.LongestStreak,
		"total_votes":    streak.TotalVotes,
	}, nil
}

// GetChallengeHistory returns past challenges with user's votes.
func (s *ChallengeService) GetChallengeHistory(appID string, userID uuid.UUID, limit int) ([]map[string]interface{}, error) {
	var challenges []Challenge
	s.db.Scopes(tenant.ForTenant(appID)).Where("is_daily = ?", true).Order("daily_date DESC").Limit(limit).Find(&challenges)

	result := make([]map[string]interface{}, 0)
	for _, c := range challenges {
		var vote Vote
		s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND challenge_id = ?", userID, c.ID).First(&vote)

		total := c.VotesA + c.VotesB
		percentA, percentB := 0, 0
		if total > 0 {
			percentA = (c.VotesA * 100) / total
			percentB = (c.VotesB * 100) / total
		}

		result = append(result, map[string]interface{}{
			"challenge": c, "user_choice": vote.Choice,
			"percent_a": percentA, "percent_b": percentB,
			"total_votes": total,
		})
	}

	return result, nil
}

// GetCategories returns all available categories.
func (s *ChallengeService) GetCategories(appID string) ([]string, error) {
	var categories []string

	err := s.db.Model(&Challenge{}).Scopes(tenant.ForTenant(appID)).
		Distinct("category").
		Pluck("category", &categories).Error

	if err != nil {
		return nil, err
	}

	if len(categories) == 0 {
		categories = []string{"life", "deep", "superpower", "funny", "love", "tech"}
	}

	return categories, nil
}

// --- QuestionGeneratorService ---

// QuestionGeneratorService handles AI-powered question generation via GLM-5.
type QuestionGeneratorService struct {
	db     *gorm.DB
	apiURL string
	apiKey string
	model  string
	client *http.Client
}

type glmRequest struct {
	Model       string       `json:"model"`
	Messages    []glmMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int          `json:"max_tokens"`
}

type glmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type glmResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Content []struct {
		Text string `json:"text"`
	} `json:"content,omitempty"`
}

type generatedQuestion struct {
	OptionA  string `json:"option_a"`
	OptionB  string `json:"option_b"`
	Category string `json:"category"`
}

// NewQuestionGeneratorService creates a new question generator service.
func NewQuestionGeneratorService(db *gorm.DB, apiURL, apiKey, model string) *QuestionGeneratorService {
	return &QuestionGeneratorService{
		db:     db,
		apiURL: apiURL,
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// IsAvailable checks if the GLM API is configured.
func (s *QuestionGeneratorService) IsAvailable() bool {
	return s.apiKey != ""
}

// GenerateBatch generates multiple questions for a given category.
func (s *QuestionGeneratorService) GenerateBatch(appID, category string, count int) ([]Challenge, error) {
	if category == "" {
		category = "general"
	}
	if count <= 0 {
		count = 10
	}
	if count > 50 {
		count = 50
	}
	if s.apiKey == "" {
		return nil, fmt.Errorf("GLM API key not configured")
	}

	systemPrompt := `You are a creative question generator for a "Would You Rather" game. Generate fun, engaging, and thought-provoking questions that make people think and laugh.

Rules:
1. Questions should be family-friendly but interesting
2. Both options should be equally difficult to choose between
3. Be creative and avoid cliches
4. Include a mix of funny, deep, and silly questions
5. Return ONLY valid JSON array, no markdown, no explanation`

	userPrompt := fmt.Sprintf(`Generate exactly %d "Would You Rather" questions for the category: "%s".

Return a JSON array with this exact format:
[
  {
    "option_a": "First option text here",
    "option_b": "Second option text here",
    "category": "%s"
  }
]

Make sure each question is unique and creative. Return ONLY the JSON array, nothing else.`, count, category, category)

	glmReq := glmRequest{
		Model: s.model,
		Messages: []glmMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.9,
		MaxTokens:   4096,
	}

	reqBody, err := json.Marshal(glmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GLM API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	var glmResp glmResponse
	if err := json.Unmarshal(body, &glmResp); err != nil {
		return nil, fmt.Errorf("failed to parse GLM response: %w", err)
	}

	var content string
	if len(glmResp.Choices) > 0 {
		content = glmResp.Choices[0].Message.Content
	} else if len(glmResp.Content) > 0 {
		content = glmResp.Content[0].Text
	} else {
		return nil, fmt.Errorf("no content in GLM response")
	}

	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var generatedQuestions []generatedQuestion
	if err := json.Unmarshal([]byte(content), &generatedQuestions); err != nil {
		return nil, fmt.Errorf("failed to parse generated questions: %w", err)
	}

	var challenges []Challenge
	for _, q := range generatedQuestions {
		if q.OptionA == "" || q.OptionB == "" {
			continue
		}

		if q.Category == "" {
			q.Category = category
		}

		challenge := Challenge{
			AppID:    appID,
			OptionA:  strings.TrimSpace(q.OptionA),
			OptionB:  strings.TrimSpace(q.OptionB),
			Category: strings.ToLower(strings.TrimSpace(q.Category)),
			IsDaily:  false,
		}

		challenges = append(challenges, challenge)
	}

	if len(challenges) == 0 {
		return nil, fmt.Errorf("no valid questions generated")
	}

	if err := s.db.Create(&challenges).Error; err != nil {
		return nil, fmt.Errorf("failed to save challenges to database: %w", err)
	}

	log.Printf("Successfully generated and saved %d questions for category: %s", len(challenges), category)
	return challenges, nil
}

// GenerateForAllCategories generates questions for all standard categories.
func (s *QuestionGeneratorService) GenerateForAllCategories(appID string, countPerCategory int) (map[string][]Challenge, error) {
	categories := []string{"funny", "deep", "food", "adventure", "impossible", "would", "tech", "lifestyle"}

	results := make(map[string][]Challenge)
	var errs []error

	for _, category := range categories {
		challenges, err := s.GenerateBatch(appID, category, countPerCategory)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", category, err))
			continue
		}
		results[category] = challenges
	}

	if len(errs) > 0 && len(results) == 0 {
		return nil, fmt.Errorf("failed to generate for all categories: %v", errs)
	}

	return results, nil
}
