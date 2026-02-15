package eracheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- Era Service ---

type EraScore struct {
	Era        string  `json:"era"`
	Score      float64 `json:"score"`
	Percentage int     `json:"percentage"`
}

type EraService struct {
	db *gorm.DB
}

func NewEraService(db *gorm.DB) *EraService {
	return &EraService{db: db}
}

func (s *EraService) GetQuizQuestions(appID string) ([]EraQuiz, error) {
	var questions []EraQuiz
	err := s.db.Scopes(tenant.ForTenant(appID)).Order("created_at ASC").Find(&questions).Error
	return questions, err
}

func (s *EraService) SubmitQuizAnswers(appID string, userID uuid.UUID, answers map[string]int) (*EraResult, error) {
	if len(answers) == 0 {
		return nil, errors.New("no answers provided")
	}

	eraScores := make(map[string]float64)
	totalWeightApplied := 0.0

	for questionID, optionIndex := range answers {
		qID, err := uuid.Parse(questionID)
		if err != nil {
			continue
		}

		var question EraQuiz
		if err := s.db.First(&question, "id = ?", qID).Error; err != nil {
			continue
		}

		categoryWeight := GetCategoryWeight(question.Category)
		totalWeightApplied += categoryWeight

		var options []QuizOption
		if err := json.Unmarshal(question.Options, &options); err != nil {
			continue
		}

		if optionIndex >= 0 && optionIndex < len(options) {
			if era := options[optionIndex].Era; era != "" {
				eraScores[era] += categoryWeight
			}
		}
	}

	if totalWeightApplied == 0 {
		totalWeightApplied = 1.0
	}

	var eraScoreList []EraScore
	for era, score := range eraScores {
		pct := int((score / totalWeightApplied) * 100)
		if pct > 100 {
			pct = 100
		}
		if pct < 1 && score > 0 {
			pct = 1
		}
		eraScoreList = append(eraScoreList, EraScore{Era: era, Score: score, Percentage: pct})
	}

	sort.Slice(eraScoreList, func(i, j int) bool {
		return eraScoreList[i].Score > eraScoreList[j].Score
	})

	for len(eraScoreList) < 3 {
		eraScoreList = append(eraScoreList, EraScore{})
	}

	topEras := normalizePercentages(eraScoreList[:3])

	bestEra := topEras[0].Era
	if bestEra == "" {
		bestEra = "2022_clean_girl"
	}
	profile, ok := GetEraProfile(bestEra)
	if !ok {
		profile = EraProfiles["2022_clean_girl"]
		bestEra = "2022_clean_girl"
	}

	scoresJSON, _ := json.Marshal(map[string]interface{}{
		"weighted_scores": eraScores,
		"top_eras":        formatTopEras(topEras),
	})

	result := &EraResult{
		AppID:          appID,
		UserID:         userID,
		Era:            profile.Key,
		EraTitle:       profile.Title,
		EraEmoji:       profile.Emoji,
		EraDescription: profile.Description,
		EraColor:       profile.Color,
		MusicTaste:     profile.MusicTaste,
		StyleTraits:    profile.StyleTraits,
		Scores:         scoresJSON,
	}

	if err := s.db.Create(result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func GetTopErasForResult(scoresJSON []byte) []EraScore {
	var data map[string]interface{}
	if err := json.Unmarshal(scoresJSON, &data); err != nil {
		return nil
	}
	topStr, ok := data["top_eras"].(string)
	if !ok || topStr == "" {
		return nil
	}
	return parseTopEras(topStr)
}

func normalizePercentages(eras []EraScore) []EraScore {
	if len(eras) == 0 {
		return eras
	}
	total := 0
	for _, e := range eras {
		total += e.Percentage
	}
	if total == 100 || total == 0 {
		return eras
	}
	diff := 100 - total
	eras[0].Percentage += diff
	if eras[0].Percentage < 0 {
		eras[0].Percentage = 0
	}
	return eras
}

func formatTopEras(eras []EraScore) string {
	parts := make([]string, len(eras))
	for i, e := range eras {
		parts[i] = fmt.Sprintf("%s:%d", e.Era, e.Percentage)
	}
	return strings.Join(parts, ",")
}

func parseTopEras(s string) []EraScore {
	var result []EraScore
	for _, part := range strings.Split(s, ",") {
		pieces := strings.SplitN(part, ":", 2)
		if len(pieces) == 2 {
			var pct int
			fmt.Sscanf(pieces[1], "%d", &pct)
			result = append(result, EraScore{Era: pieces[0], Percentage: pct})
		}
	}
	return result
}

func (s *EraService) GetUserResults(appID string, userID uuid.UUID) ([]EraResult, error) {
	var results []EraResult
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Order("created_at DESC").Find(&results).Error
	return results, err
}

func (s *EraService) GetResultByID(id uuid.UUID) (*EraResult, error) {
	var result EraResult
	if err := s.db.First(&result, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("record not found")
		}
		return nil, err
	}
	return &result, nil
}

func (s *EraService) IncrementShareCount(id uuid.UUID) error {
	return s.db.Model(&EraResult{}).Where("id = ?", id).
		UpdateColumn("share_count", gorm.Expr("share_count + 1")).Error
}

func (s *EraService) GetEraStats(appID string, userID uuid.UUID) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalShares int64
	s.db.Model(&EraResult{}).Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(share_count),0)").Scan(&totalShares)
	stats["total_shares"] = totalShares

	var streak EraStreak
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error; err != nil {
		stats["current_streak"] = 0
		stats["longest_streak"] = 0
	} else {
		stats["current_streak"] = streak.CurrentStreak
		stats["longest_streak"] = streak.LongestStreak
	}

	var quizCount int64
	s.db.Model(&EraResult{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&quizCount)
	stats["quizzes_taken"] = quizCount

	if quizCount > 0 {
		var favoriteEra string
		s.db.Model(&EraResult{}).Scopes(tenant.ForTenant(appID)).
			Select("era").Where("user_id = ?", userID).
			Group("era").Order("COUNT(*) DESC").Limit(1).Scan(&favoriteEra)
		stats["favorite_era"] = favoriteEra
		if profile, ok := GetEraProfile(favoriteEra); ok {
			stats["favorite_era_profile"] = profile
		}
	}

	return stats, nil
}

// --- Streak Service ---

type StreakService struct {
	db *gorm.DB
}

func NewStreakService(db *gorm.DB) *StreakService {
	return &StreakService{db: db}
}

func (s *StreakService) GetOrCreateStreak(appID string, userID uuid.UUID) (*EraStreak, error) {
	var streak EraStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if err == nil {
		return &streak, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	streak = EraStreak{
		AppID:          appID,
		UserID:         userID,
		LastActiveDate: time.Now().Truncate(24 * time.Hour),
	}
	if err := s.db.Create(&streak).Error; err != nil {
		return nil, err
	}
	return &streak, nil
}

func (s *StreakService) UpdateStreak(appID string, userID uuid.UUID) error {
	streak, err := s.GetOrCreateStreak(appID, userID)
	if err != nil {
		return err
	}

	today := time.Now().Truncate(24 * time.Hour)
	lastActive := streak.LastActiveDate.Truncate(24 * time.Hour)

	if today.Equal(lastActive) {
		return nil
	}

	if lastActive.Equal(today.Add(-24 * time.Hour)) {
		streak.CurrentStreak++
	} else {
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}
	streak.LastActiveDate = today
	return s.db.Save(&streak).Error
}

type StreakBadge struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Emoji       string `json:"emoji"`
	Required    int    `json:"required"`
	Unlocked    bool   `json:"unlocked"`
}

func (s *StreakService) GetStreakBadges(appID string, userID uuid.UUID) ([]StreakBadge, error) {
	streak, err := s.GetOrCreateStreak(appID, userID)
	if err != nil {
		return nil, err
	}
	return []StreakBadge{
		{Name: "Starter", Description: "Complete 1 day", Emoji: "⭐", Required: 1, Unlocked: streak.CurrentStreak >= 1},
		{Name: "Committed", Description: "3-day streak", Emoji: "🔥", Required: 3, Unlocked: streak.CurrentStreak >= 3},
		{Name: "Dedicated", Description: "7-day streak", Emoji: "💎", Required: 7, Unlocked: streak.CurrentStreak >= 7},
		{Name: "Obsessed", Description: "14-day streak", Emoji: "👑", Required: 14, Unlocked: streak.CurrentStreak >= 14},
		{Name: "Legend", Description: "30-day streak", Emoji: "🏆", Required: 30, Unlocked: streak.CurrentStreak >= 30},
		{Name: "Icon", Description: "50-day streak", Emoji: "💫", Required: 50, Unlocked: streak.CurrentStreak >= 50},
	}, nil
}

// --- Challenge Service ---

type ChallengeService struct {
	db                *gorm.DB
	aiAnalyzer        *AIAnalyzer
	moderationService *services.ModerationService
}

func NewChallengeService(db *gorm.DB, aiURL, aiKey string, moderationService *services.ModerationService) *ChallengeService {
	var analyzer *AIAnalyzer
	if aiKey != "" {
		analyzer = NewAIAnalyzer(aiURL, aiKey)
	}
	return &ChallengeService{db: db, aiAnalyzer: analyzer, moderationService: moderationService}
}

func (s *ChallengeService) GetDailyChallenge(appID string, userID uuid.UUID) (*EraChallenge, error) {
	today := time.Now().Truncate(24 * time.Hour)

	var challenge EraChallenge
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND challenge_date = ?", userID, today).First(&challenge).Error
	if err == nil {
		return &challenge, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	prompt := challengePrompts[rand.Intn(len(challengePrompts))]
	challenge = EraChallenge{
		AppID:         appID,
		UserID:        userID,
		ChallengeDate: today,
		Prompt:        prompt,
	}
	if err := s.db.Create(&challenge).Error; err != nil {
		return nil, err
	}
	return &challenge, nil
}

func (s *ChallengeService) SubmitChallengeResponse(appID string, userID uuid.UUID, response string) (*EraChallenge, error) {
	if s.moderationService != nil {
		if isClean, reason := s.moderationService.FilterContent(response); !isClean {
			return nil, fmt.Errorf("content rejected: %s", s.moderationService.GetRejectionMessage(reason))
		}
	}

	today := time.Now().Truncate(24 * time.Hour)
	var challenge EraChallenge
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND challenge_date = ?", userID, today).First(&challenge).Error
	if err != nil {
		return nil, errors.New("no challenge found for today — get today's challenge first")
	}
	if challenge.Response != "" {
		return nil, errors.New("you have already responded to today's challenge")
	}

	era := s.detectEraFromResponse(response)
	if era == "unknown" {
		era = "2022_clean_girl"
	}

	challenge.Response = response
	challenge.Era = era
	if err := s.db.Save(&challenge).Error; err != nil {
		return nil, err
	}
	return &challenge, nil
}

func (s *ChallengeService) GetChallengeHistory(appID string, userID uuid.UUID, limit int) ([]EraChallenge, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var challenges []EraChallenge
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).Order("challenge_date DESC").Limit(limit).Find(&challenges).Error
	return challenges, err
}

func (s *ChallengeService) detectEraFromResponse(input string) string {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if s.aiAnalyzer != nil && s.aiAnalyzer.IsConfigured() {
		era, err := s.aiAnalyzer.AnalyzeEraFromText(normalized)
		if err == nil && era != "" {
			return era
		}
		slog.Warn("AI era analysis failed, using keywords", "error", err)
	}
	return detectEraFromKeywords(normalized)
}

func detectEraFromKeywords(input string) string {
	keywords := map[string][]string{
		"y2k": {"y2k", "2000s", "butterfly", "paris", "bedazzled", "glitter", "britney", "juicy"},
		"2016_tumblr": {"tumblr", "pastel", "galaxy", "grunge", "choker", "flannel", "indie", "band tee"},
		"2018_vsco": {"vsco", "scrunchie", "hydro", "flask", "puka", "shell", "beach", "chill"},
		"2020_cottagecore": {"cottage", "bread", "prairie", "floral", "nature", "picnic", "baking", "folk"},
		"dark_academia": {"academia", "library", "poetry", "classical", "vintage", "tweed", "blazer", "hozier"},
		"indie_sleaze": {"indie", "sleaze", "party", "messy", "leather", "punk", "strokes", "warehouse"},
		"2022_clean_girl": {"clean", "minimal", "slicked", "gold", "neutral", "polished", "blazer", "loafer"},
		"2024_mob_wife": {"mob", "wife", "fur", "leopard", "luxury", "bold", "sunglasses", "chanel"},
		"coastal_cowgirl": {"cowgirl", "boots", "beach", "turquoise", "western", "sunset", "kacey", "denim"},
		"2025_demure": {"demure", "mindful", "cutesy", "modest", "bow", "polite", "soft", "pink"},
	}
	for era, words := range keywords {
		for _, word := range words {
			if strings.Contains(input, word) {
				return era
			}
		}
	}
	return "unknown"
}

// --- AI Analyzer ---

type AIAnalyzer struct {
	apiURL     string
	apiKey     string
	httpClient *http.Client
	validEras  map[string]bool
}

type aiRequest struct {
	Model    string      `json:"model"`
	Messages []aiMessage `json:"messages"`
}
type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type aiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewAIAnalyzer(apiURL, apiKey string) *AIAnalyzer {
	validEras := make(map[string]bool, len(EraKeys))
	for _, k := range EraKeys {
		validEras[k] = true
	}
	return &AIAnalyzer{
		apiURL: apiURL, apiKey: apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		validEras:  validEras,
	}
}

func (a *AIAnalyzer) IsConfigured() bool { return a.apiKey != "" }

func (a *AIAnalyzer) AnalyzeEraFromText(input string) (string, error) {
	body, _ := json.Marshal(aiRequest{
		Model: "glm-5",
		Messages: []aiMessage{
			{Role: "system", Content: "You are an aesthetic era classifier. Given user text, classify into exactly one of: y2k, 2016_tumblr, 2018_vsco, 2020_cottagecore, dark_academia, indie_sleaze, 2022_clean_girl, 2024_mob_wife, coastal_cowgirl, 2025_demure. Respond with ONLY the era key."},
			{Role: "user", Content: input},
		},
	})

	req, _ := http.NewRequest("POST", a.apiURL, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API status %d", resp.StatusCode)
	}

	var aiResp aiResponse
	if err := json.Unmarshal(respBody, &aiResp); err != nil {
		return "", err
	}
	if len(aiResp.Choices) == 0 {
		return "", errors.New("no choices")
	}

	era := strings.TrimSpace(strings.ToLower(aiResp.Choices[0].Message.Content))
	if !a.validEras[era] {
		return "", fmt.Errorf("invalid era: %s", era)
	}
	return era, nil
}
