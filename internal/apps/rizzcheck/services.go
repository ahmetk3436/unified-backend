package rizzcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	freeDailyLimit = 5
)

// RizzService handles AI response generation and streak tracking.
type RizzService struct {
	db     *gorm.DB
	cfg    *config.Config
	client *http.Client
}

// NewRizzService creates a new RizzService.
func NewRizzService(db *gorm.DB, cfg *config.Config) *RizzService {
	timeout := cfg.AITimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &RizzService{
		db:     db,
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

// GenerateResponses generates 3 AI-powered witty responses.
func (s *RizzService) GenerateResponses(userID uuid.UUID, appID, inputText, tone, category string) (*RizzResponse, error) {
	// Validate inputs
	if !ValidTones[tone] {
		return nil, fmt.Errorf("invalid tone: %s", tone)
	}
	if !ValidCategories[category] {
		return nil, fmt.Errorf("invalid category: %s", category)
	}
	if len(inputText) == 0 {
		return nil, fmt.Errorf("input text is required")
	}
	if len(inputText) > 1000 {
		return nil, fmt.Errorf("input text too long (max 1000 characters)")
	}

	// Check free usage limit
	streak, err := s.getOrCreateStreak(userID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to check usage: %w", err)
	}

	// Reset daily counter if new day
	s.resetDailyIfNeeded(streak)

	// Note: Premium check would happen in the handler layer via subscription context
	// Here we just enforce the free limit — handlers should check subscription before calling
	if streak.FreeUsesToday >= freeDailyLimit {
		return nil, fmt.Errorf("daily free limit reached (%d/%d). Upgrade to Premium for unlimited rizz!", streak.FreeUsesToday, freeDailyLimit)
	}

	// Generate via LLM
	r1, r2, r3, err := s.callLLM(inputText, tone, category)
	if err != nil {
		slog.Warn("LLM generation failed, using templates", "error", err)
		r1, r2, r3 = s.templateFallback(inputText, tone)
	}

	// Save response
	response := &RizzResponse{
		AppID:     appID,
		UserID:    userID,
		InputText: inputText,
		Tone:      tone,
		Category:  category,
		Response1: r1,
		Response2: r2,
		Response3: r3,
	}

	if err := s.db.Create(response).Error; err != nil {
		return nil, fmt.Errorf("failed to save response: %w", err)
	}

	// Update streak
	s.updateStreak(streak)

	return response, nil
}

// GetStreak returns user's streak and stats.
func (s *RizzService) GetStreak(userID uuid.UUID, appID string) (*RizzStreak, error) {
	streak, err := s.getOrCreateStreak(userID, appID)
	if err != nil {
		return nil, err
	}
	s.resetDailyIfNeeded(streak)
	return streak, nil
}

// GetHistory returns paginated response history.
func (s *RizzService) GetHistory(userID uuid.UUID, appID string, page, limit int) ([]RizzResponse, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var total int64
	s.db.Model(&RizzResponse{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&total)

	var responses []RizzResponse
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&responses).Error

	return responses, total, err
}

// SelectResponse marks which response the user selected.
func (s *RizzService) SelectResponse(userID uuid.UUID, appID string, responseID uuid.UUID, selectedIdx int) error {
	if selectedIdx < 1 || selectedIdx > 3 {
		return fmt.Errorf("selected_idx must be 1, 2, or 3")
	}

	result := s.db.Model(&RizzResponse{}).
		Scopes(tenant.ForTenant(appID)).
		Where("id = ? AND user_id = ?", responseID, userID).
		Update("selected_idx", selectedIdx)

	if result.RowsAffected == 0 {
		return fmt.Errorf("response not found")
	}
	return result.Error
}

// --- Internal helpers ---

func (s *RizzService) getOrCreateStreak(userID uuid.UUID, appID string) (*RizzStreak, error) {
	var streak RizzStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if err == nil {
		return &streak, nil
	}

	streak = RizzStreak{
		AppID:  appID,
		UserID: userID,
	}
	if err := s.db.Create(&streak).Error; err != nil {
		return nil, err
	}
	return &streak, nil
}

func (s *RizzService) resetDailyIfNeeded(streak *RizzStreak) {
	today := time.Now().Truncate(24 * time.Hour)
	if streak.LastUseDate == nil || !streak.LastUseDate.Truncate(24*time.Hour).Equal(today) {
		streak.FreeUsesToday = 0
	}
}

func (s *RizzService) updateStreak(streak *RizzStreak) {
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)

	streak.TotalRizzes++
	streak.FreeUsesToday++

	if streak.LastUseDate != nil {
		lastDate := streak.LastUseDate.Truncate(24 * time.Hour)
		if lastDate.Equal(yesterday) {
			streak.CurrentStreak++
		} else if !lastDate.Equal(today) {
			streak.CurrentStreak = 1
		}
	} else {
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}

	streak.LastUseDate = &today
	s.db.Save(streak)
}

// --- LLM integration ---

type llmRequest struct {
	Model       string       `json:"model"`
	Messages    []llmMessage `json:"messages"`
	Temperature float64      `json:"temperature"`
	MaxTokens   int          `json:"max_tokens"`
}

type llmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type generatedResponses struct {
	Response1 string `json:"response_1"`
	Response2 string `json:"response_2"`
	Response3 string `json:"response_3"`
}

func (s *RizzService) callLLM(inputText, tone, category string) (string, string, string, error) {
	// Try GLM-5 first, then DeepSeek fallback
	r1, r2, r3, err := s.callProvider(s.cfg.GLMAPIURL, s.cfg.GLMAPIKey, s.cfg.GLMModel, inputText, tone, category)
	if err == nil {
		return r1, r2, r3, nil
	}
	slog.Warn("GLM-5 failed, trying DeepSeek", "error", err)

	if s.cfg.DeepSeekAPIKey != "" {
		r1, r2, r3, err = s.callProvider(s.cfg.DeepSeekAPIURL, s.cfg.DeepSeekAPIKey, s.cfg.DeepSeekModel, inputText, tone, category)
		if err == nil {
			return r1, r2, r3, nil
		}
		slog.Warn("DeepSeek also failed", "error", err)
	}

	return "", "", "", fmt.Errorf("all LLM providers failed: %w", err)
}

func (s *RizzService) callProvider(apiURL, apiKey, model, inputText, tone, category string) (string, string, string, error) {
	if apiKey == "" {
		return "", "", "", fmt.Errorf("API key not configured")
	}

	systemPrompt := fmt.Sprintf(`You are RizzCheck, an expert conversational AI that generates witty, clever, and charming text message responses.

Your style: %s tone for a %s context.

Rules:
1. Generate exactly 3 different response options
2. Each response should be natural, clever, and match the requested tone
3. Keep responses concise (1-3 sentences max)
4. Never be offensive, crude, or inappropriate
5. Make responses feel authentic, not robotic
6. Return ONLY valid JSON, no markdown or explanation`, tone, category)

	userPrompt := fmt.Sprintf(`Someone sent this message: "%s"

Generate 3 witty %s responses for a %s conversation.

Return JSON:
{"response_1": "...", "response_2": "...", "response_3": "..."}`, inputText, tone, category)

	reqBody, err := json.Marshal(llmRequest{
		Model: model,
		Messages: []llmMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.8,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", "", "", err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var llmResp llmResponse
	if err := json.Unmarshal(body, &llmResp); err != nil {
		return "", "", "", err
	}

	if len(llmResp.Choices) == 0 {
		return "", "", "", fmt.Errorf("empty response from API")
	}

	content := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var gen generatedResponses
	if err := json.Unmarshal([]byte(content), &gen); err != nil {
		return "", "", "", fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if gen.Response1 == "" || gen.Response2 == "" || gen.Response3 == "" {
		return "", "", "", fmt.Errorf("incomplete responses from LLM")
	}

	return gen.Response1, gen.Response2, gen.Response3, nil
}

// templateFallback generates basic responses when all LLM providers fail.
func (s *RizzService) templateFallback(inputText, tone string) (string, string, string) {
	templates := map[string][3]string{
		"flirty": {
			"Is it just me, or did this conversation just get way more interesting? 😏",
			"I could say something clever, but honestly, you already have my attention.",
			"Bold move. I respect that. And I raise you this response.",
		},
		"professional": {
			"Thank you for reaching out. I'd be happy to discuss this further at your convenience.",
			"That's an excellent point. Let me share some thoughts on this.",
			"I appreciate the message. Looking forward to our continued conversation.",
		},
		"funny": {
			"I laughed, I cried, I screenshot this for later. 10/10 message.",
			"If messages were a sport, you'd be in the major leagues right now.",
			"I'm not saying this is the best message I've ever received... but it's top 3.",
		},
		"chill": {
			"Yeah that tracks. Good vibes only over here.",
			"Lowkey appreciate that. No cap.",
			"That's a whole vibe. I'm here for it.",
		},
		"savage": {
			"Interesting take. But have you considered being wrong about this?",
			"I'd agree with you, but then we'd both be wrong.",
			"Points for effort. The execution though... needs work.",
		},
		"romantic": {
			"Every message from you makes my day a little brighter. Just saying.",
			"If I could bottle this conversation, I would. It's that good.",
			"You have this way of making simple words feel like poetry.",
		},
		"confident": {
			"I knew you'd reach out. Good instincts on your part.",
			"Consider this my RSVP to whatever you're planning. I'm in.",
			"Now we're talking. Let's make this happen.",
		},
		"mysterious": {
			"Some things are better left unsaid... but I'll make an exception.",
			"Intriguing. I'll have to think about that one.",
			"The best conversations always start like this. Let's see where it goes.",
		},
	}

	if t, ok := templates[tone]; ok {
		// Shuffle order for variety
		idx := rand.Perm(3)
		return t[idx[0]], t[idx[1]], t[idx[2]]
	}

	return "That's a great message!", "I appreciate you sharing that.", "Let's keep this conversation going!"
}
