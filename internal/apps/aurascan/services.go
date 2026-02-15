package aurascan

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// =============================================================================
// AI types (internal)
// =============================================================================

type auraAnalysisResult struct {
	AuraColor      string  `json:"aura_color"`
	SecondaryColor *string `json:"secondary_color,omitempty"`
	EnergyLevel    int     `json:"energy_level"`
	MoodScore      int     `json:"mood_score"`
	Personality    string  `json:"personality"`
	DailyAdvice    string  `json:"daily_advice"`
}

type auraChatRequest struct {
	Model    string            `json:"model"`
	Messages []auraChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type auraChatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type auraChatContentPart struct {
	Type     string               `json:"type"`
	Text     string               `json:"text,omitempty"`
	ImageURL *auraChatImageURL    `json:"image_url,omitempty"`
}

type auraChatImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type auraChatResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Color traits mapping.
var colorTraits = map[string]struct {
	personality string
	strengths   []string
	challenges  []string
	dailyAdvice string
}{
	"red":    {"Passionate, energetic, and action-oriented.", []string{"Courage", "Leadership", "Determination"}, []string{"Impulsiveness", "Patience", "Anger Management"}, "Channel your energy into a physical activity today."},
	"orange": {"Creative, social, and adventurous.", []string{"Creativity", "Optimism", "Social Skills"}, []string{"Scattered Focus", "Restlessness", "Overcommitment"}, "Start a new creative project. Connect with an old friend."},
	"yellow": {"Optimistic, intellectual, and cheerful.", []string{"Analytical Thinking", "Positivity", "Communication"}, []string{"Critical Nature", "Overthinking", "Perfectionism"}, "Share your ideas with others. Take time to relax your mind."},
	"green":  {"Balanced, growth-oriented, and nurturing.", []string{"Compassion", "Reliability", "Growth Mindset"}, []string{"Jealousy", "Possessiveness", "Insecurity"}, "Spend time in nature. Nurture a relationship or a plant."},
	"blue":   {"Calm, intuitive, and trustworthy.", []string{"Communication", "Intuition", "Loyalty"}, []string{"Fear of Expression", "Melancholy", "Stubbornness"}, "Speak your truth today. Trust your gut feelings."},
	"indigo": {"Intuitive, wise, and deeply spiritual.", []string{"Vision", "Wisdom", "Integrity"}, []string{"Isolation", "Judgment", "Rigidity"}, "Meditate or reflect on your long-term goals."},
	"violet": {"Visionary, artistic, and magical.", []string{"Imagination", "Humanitarianism", "Leadership"}, []string{"Unrealistic Expectations", "Arrogance", "Detachment"}, "Engage in art or music. Visualize your ideal future."},
	"white":  {"Pure, balanced, and spiritually connected.", []string{"Purity", "Healing", "High Vibration"}, []string{"Vulnerability", "Naivety", "Disconnection from Reality"}, "Focus on cleansing your space. Protect your energy."},
	"gold":   {"Confident, abundant, and empowered.", []string{"Confidence", "Generosity", "Willpower"}, []string{"Ego", "Greed", "Overbearing nature"}, "Share your abundance with others. Practice humility."},
	"pink":   {"Loving, gentle, and compassionate.", []string{"Love", "Empathy", "Nurturing"}, []string{"Neediness", "Martyrdom", "Lack of Boundaries"}, "Practice self-love. Set healthy boundaries with kindness."},
}

var auraColors = []string{"red", "orange", "yellow", "green", "blue", "indigo", "violet", "white", "gold", "pink"}
var secondaryColors = []string{"silver", "gold", "white", "black", "grey"}

const auraDailyFreeLimit = 2

const auraSystemPrompt = `You are GlowType, an expert aura and color energy analyst. Analyze the person in this image carefully.
Determine their dominant aura color from: red, orange, yellow, green, blue, indigo, violet, white, gold, pink.
Return your analysis as a JSON object with these exact fields:
{"aura_color":"...", "secondary_color":"...", "energy_level":1-100, "mood_score":1-10, "personality":"...", "daily_advice":"..."}
Return ONLY the JSON object, no extra text.`

// =============================================================================
// AuraService
// =============================================================================

type AuraService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAuraService(db *gorm.DB, cfg *config.Config) *AuraService {
	return &AuraService{db: db, cfg: cfg}
}

func (s *AuraService) IsSubscribed(appID string, userID uuid.UUID) bool {
	var sub models.Subscription
	err := s.db.
		Where("user_id = ? AND app_id = ? AND status = ? AND current_period_end > ?", userID, appID, "active", time.Now()).
		Order("current_period_end DESC").
		First(&sub).Error
	return err == nil
}

func (s *AuraService) CanScan(appID string, userID uuid.UUID, isSubscribed bool) (bool, int, error) {
	if isSubscribed {
		return true, -1, nil
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var scansToday int64
	if err := s.db.Model(&AuraReading{}).Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startOfDay, endOfDay).
		Count(&scansToday).Error; err != nil {
		return false, 0, err
	}

	remaining := auraDailyFreeLimit - int(scansToday)
	if remaining < 0 {
		remaining = 0
	}

	return scansToday < auraDailyFreeLimit, remaining, nil
}

func (s *AuraService) Create(appID string, userID uuid.UUID, req CreateAuraRequest) (*AuraReading, error) {
	imageURL := strings.TrimSpace(req.ImageURL)
	imageData := strings.TrimSpace(req.ImageData)
	if imageURL == "" && imageData != "" {
		imageURL = "base64_upload"
	}
	if imageURL == "" {
		return nil, errors.New("image_url or image_data is required")
	}

	// Attempt AI analysis
	analysis, err := s.analyzeAura(imageData, imageURL)
	if err != nil {
		log.Printf("[WARN] AI analysis failed for user %s: %v, falling back to deterministic", userID, err)
		fallback := deterministicAuraResult(userID, imageURL)
		analysis = &fallback
	}

	traits, ok := colorTraits[analysis.AuraColor]
	if !ok {
		traits = colorTraits["violet"]
		analysis.AuraColor = "violet"
	}

	personality := analysis.Personality
	if personality == "" {
		personality = traits.personality
	}
	dailyAdvice := analysis.DailyAdvice
	if dailyAdvice == "" {
		dailyAdvice = traits.dailyAdvice
	}

	reading := &AuraReading{
		AppID:          appID,
		UserID:         userID,
		ImageURL:       imageURL,
		AuraColor:      analysis.AuraColor,
		SecondaryColor: analysis.SecondaryColor,
		EnergyLevel:    clamp(analysis.EnergyLevel, 1, 100),
		MoodScore:      clamp(analysis.MoodScore, 1, 10),
		Personality:    personality,
		Strengths:      traits.strengths,
		Challenges:     traits.challenges,
		DailyAdvice:    dailyAdvice,
		AnalyzedAt:     time.Now(),
	}

	if err := s.db.Create(reading).Error; err != nil {
		return nil, err
	}

	return reading, nil
}

func (s *AuraService) analyzeAura(imageData, imageURL string) (*auraAnalysisResult, error) {
	// Try GLM vision first, then DeepSeek fallback
	if s.cfg.GLMAPIKey != "" {
		result, err := s.analyzeWithProvider(s.cfg.GLMAPIURL, s.cfg.GLMAPIKey, s.cfg.GLMVisionModel, imageData, imageURL, true)
		if err == nil {
			return result, nil
		}
		log.Printf("[WARN] GLM analysis failed: %v", err)
	}

	if s.cfg.DeepSeekAPIKey != "" {
		result, err := s.analyzeWithProvider(s.cfg.DeepSeekAPIURL, s.cfg.DeepSeekAPIKey, s.cfg.DeepSeekModel, imageData, imageURL, false)
		if err == nil {
			return result, nil
		}
		log.Printf("[WARN] DeepSeek analysis failed: %v", err)
	}

	return nil, errors.New("no AI provider available")
}

func (s *AuraService) analyzeWithProvider(apiURL, apiKey, model, imageData, imageURL string, supportsVision bool) (*auraAnalysisResult, error) {
	var messages []auraChatMessage

	if supportsVision && (imageData != "" || imageURL != "") {
		var imgURL string
		if imageData != "" {
			imgURL = fmt.Sprintf("data:image/jpeg;base64,%s", imageData)
		} else {
			imgURL = imageURL
		}
		messages = []auraChatMessage{
			{Role: "system", Content: auraSystemPrompt},
			{Role: "user", Content: []auraChatContentPart{
				{Type: "text", Text: "Please analyze this photo and tell me about my aura and energy."},
				{Type: "image_url", ImageURL: &auraChatImageURL{URL: imgURL, Detail: "auto"}},
			}},
		}
	} else {
		messages = []auraChatMessage{
			{Role: "system", Content: auraSystemPrompt},
			{Role: "user", Content: "I've uploaded a photo for aura analysis. Please generate a thoughtful aura reading. Return the JSON analysis."},
		}
	}

	reqBody := auraChatRequest{Model: model, Messages: messages, Temperature: 0.7}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	timeout := s.cfg.AITimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("AI API error: status %d", resp.StatusCode)
	}

	var completion auraChatResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return nil, err
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("no response from AI")
	}

	var content string
	switch v := completion.Choices[0].Message.Content.(type) {
	case string:
		content = v
	default:
		contentBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to extract content from AI response")
		}
		content = string(contentBytes)
	}

	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var parsed auraAnalysisResult
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		start := strings.Index(content, "{")
		end := strings.LastIndex(content, "}")
		if start >= 0 && end > start {
			if err2 := json.Unmarshal([]byte(content[start:end+1]), &parsed); err2 != nil {
				return nil, fmt.Errorf("failed to parse aura result: %w", err2)
			}
		} else {
			return nil, fmt.Errorf("failed to parse aura result: %w", err)
		}
	}

	parsed.AuraColor = normalizeAuraColor(parsed.AuraColor)
	if parsed.AuraColor == "" {
		parsed.AuraColor = "blue"
	}
	if parsed.SecondaryColor != nil {
		n := strings.ToLower(strings.TrimSpace(*parsed.SecondaryColor))
		if n == "" || n == "null" || n == "none" {
			parsed.SecondaryColor = nil
		} else {
			parsed.SecondaryColor = &n
		}
	}
	parsed.EnergyLevel = clamp(parsed.EnergyLevel, 1, 100)
	parsed.MoodScore = clamp(parsed.MoodScore, 1, 10)

	return &parsed, nil
}

func deterministicAuraResult(userID uuid.UUID, imageURL string) auraAnalysisResult {
	seedInput := strings.ToLower(strings.TrimSpace(imageURL)) + ":" + userID.String()
	hash := sha256.Sum256([]byte(seedInput))

	color := auraColors[int(hash[0])%len(auraColors)]
	energy := 45 + int(hash[1])%51
	mood := 5 + int(hash[2])%6

	var secondary *string
	if int(hash[3])%4 == 0 {
		candidate := secondaryColors[int(hash[4])%len(secondaryColors)]
		if candidate != color {
			secondary = &candidate
		}
	}

	return auraAnalysisResult{
		AuraColor:      color,
		SecondaryColor: secondary,
		EnergyLevel:    energy,
		MoodScore:      mood,
		Personality:    fmt.Sprintf("Your %s aura suggests a person of depth and character with natural leadership qualities.", color),
		DailyAdvice:    "Focus on positive energy today and let your true colors shine through.",
	}
}

func normalizeAuraColor(color string) string {
	normalized := strings.ToLower(strings.TrimSpace(color))
	for _, c := range auraColors {
		if normalized == c {
			return normalized
		}
	}
	return ""
}

func clamp(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func (s *AuraService) GetByID(appID string, userID, id uuid.UUID) (*AuraReading, error) {
	var reading AuraReading
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND id = ?", userID, id).First(&reading).Error
	if err != nil {
		return nil, err
	}
	return &reading, nil
}

func (s *AuraService) List(appID string, userID uuid.UUID, page, pageSize int) ([]AuraReading, int64, error) {
	var readings []AuraReading
	var total int64

	offset := (page - 1) * pageSize

	if err := s.db.Model(&AuraReading{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).
		Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&readings).Error
	if err != nil {
		return nil, 0, err
	}

	return readings, total, nil
}

func (s *AuraService) Delete(appID string, userID, id uuid.UUID) error {
	result := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND id = ?", userID, id).Delete(&AuraReading{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("record not found")
	}
	return nil
}

func (s *AuraService) GetStats(appID string, userID uuid.UUID) (*AuraStatsResponse, error) {
	var readings []AuraReading
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Find(&readings).Error; err != nil {
		return nil, err
	}

	if len(readings) == 0 {
		return &AuraStatsResponse{ColorDistribution: make(map[string]int)}, nil
	}

	colorDist := make(map[string]int)
	totalEnergy := 0
	totalMood := 0

	for _, r := range readings {
		colorDist[r.AuraColor]++
		totalEnergy += r.EnergyLevel
		totalMood += r.MoodScore
	}

	return &AuraStatsResponse{
		ColorDistribution: colorDist,
		TotalReadings:     int64(len(readings)),
		AverageEnergy:     float64(totalEnergy) / float64(len(readings)),
		AverageMood:       float64(totalMood) / float64(len(readings)),
	}, nil
}

// =============================================================================
// AuraMatchService
// =============================================================================

type AuraMatchService struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewAuraMatchService(db *gorm.DB, cfg *config.Config) *AuraMatchService {
	return &AuraMatchService{db: db, cfg: cfg}
}

// Complementary color pairs.
var complementaryColors = map[string]string{
	"red": "green", "green": "red", "blue": "orange", "orange": "blue",
	"yellow": "violet", "violet": "yellow", "indigo": "gold", "gold": "indigo",
	"pink": "white", "white": "pink",
}

var synergyMessages = map[string]string{
	"same":          "You share a deep soul connection! Your energies resonate on the same frequency.",
	"complementary": "Your energies perfectly balance each other.",
	"neutral":       "Your auras have a harmonious blend, creating a stable connection.",
	"challenging":   "Your energies create exciting tension - you push each other to grow.",
}

var tensionMessages = map[string]string{
	"same":          "Too much similarity can lead to stagnation.",
	"complementary": "Your differences may sometimes cause misunderstandings.",
	"neutral":       "Neither of you may feel deeply challenged.",
	"challenging":   "Conflicting energies require patience.",
}

var adviceMessages = map[string]string{
	"same":          "Celebrate your similarities while exploring new territories together.",
	"complementary": "Embrace your differences as gifts.",
	"neutral":       "Build intentional rituals to deepen your connection.",
	"challenging":   "Practice patience and active listening.",
}

func (s *AuraMatchService) Create(appID string, userID uuid.UUID, req CreateMatchRequest) (*AuraMatchResponse, error) {
	friendID, err := uuid.Parse(req.FriendID)
	if err != nil {
		return nil, errors.New("invalid friend ID")
	}
	if userID == friendID {
		return nil, errors.New("cannot match with yourself")
	}

	var userAura AuraReading
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Order("created_at DESC").First(&userAura).Error; err != nil {
		return nil, errors.New("you need an aura reading first")
	}

	var friendAura AuraReading
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", friendID).Order("created_at DESC").First(&friendAura).Error; err != nil {
		return nil, errors.New("friend doesn't have an aura reading yet")
	}

	score, synergy, tension, advice := s.calculateCompatibility(userAura.AuraColor, friendAura.AuraColor)

	match := &AuraMatch{
		AppID:              appID,
		UserID:             userID,
		FriendID:           friendID,
		UserAuraID:         userAura.ID,
		FriendAuraID:       friendAura.ID,
		CompatibilityScore: score,
		Synergy:            synergy,
		Tension:            tension,
		Advice:             advice,
	}

	if err := s.db.Create(match).Error; err != nil {
		return nil, err
	}

	return &AuraMatchResponse{
		ID: match.ID, UserID: match.UserID, FriendID: match.FriendID,
		UserAuraID: match.UserAuraID, FriendAuraID: match.FriendAuraID,
		CompatibilityScore: match.CompatibilityScore,
		Synergy: match.Synergy, Tension: match.Tension, Advice: match.Advice,
		UserAuraColor: userAura.AuraColor, FriendAuraColor: friendAura.AuraColor,
		CreatedAt: match.CreatedAt,
	}, nil
}

func (s *AuraMatchService) calculateCompatibility(userColor, friendColor string) (int, string, string, string) {
	var score int
	var matchType string

	if userColor == friendColor {
		score = 85 + rand.Intn(16)
		matchType = "same"
	} else if complementaryColors[userColor] == friendColor {
		score = 70 + rand.Intn(21)
		matchType = "complementary"
	} else {
		score = 50 + rand.Intn(26)
		matchType = "neutral"
	}

	synergy := fmt.Sprintf("%s Your %s aura meets their %s energy.", synergyMessages[matchType], userColor, friendColor)
	tension := tensionMessages[matchType]
	advice := adviceMessages[matchType]

	return score, synergy, tension, advice
}

func (s *AuraMatchService) List(appID string, userID uuid.UUID) ([]AuraMatchResponse, error) {
	var matches []AuraMatch
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Order("created_at DESC").Find(&matches).Error; err != nil {
		return nil, err
	}

	responses := make([]AuraMatchResponse, len(matches))
	for i, m := range matches {
		var userAura, friendAura AuraReading
		s.db.First(&userAura, "id = ?", m.UserAuraID)
		s.db.First(&friendAura, "id = ?", m.FriendAuraID)

		responses[i] = AuraMatchResponse{
			ID: m.ID, UserID: m.UserID, FriendID: m.FriendID,
			UserAuraID: m.UserAuraID, FriendAuraID: m.FriendAuraID,
			CompatibilityScore: m.CompatibilityScore,
			Synergy: m.Synergy, Tension: m.Tension, Advice: m.Advice,
			UserAuraColor: userAura.AuraColor, FriendAuraColor: friendAura.AuraColor,
			CreatedAt: m.CreatedAt,
		}
	}

	return responses, nil
}

func (s *AuraMatchService) GetByFriend(appID string, userID, friendID uuid.UUID) (*AuraMatchResponse, error) {
	var match AuraMatch
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND friend_id = ?", userID, friendID).
		Order("created_at DESC").First(&match).Error; err != nil {
		return nil, err
	}

	var userAura, friendAura AuraReading
	s.db.First(&userAura, "id = ?", match.UserAuraID)
	s.db.First(&friendAura, "id = ?", match.FriendAuraID)

	return &AuraMatchResponse{
		ID: match.ID, UserID: match.UserID, FriendID: match.FriendID,
		UserAuraID: match.UserAuraID, FriendAuraID: match.FriendAuraID,
		CompatibilityScore: match.CompatibilityScore,
		Synergy: match.Synergy, Tension: match.Tension, Advice: match.Advice,
		UserAuraColor: userAura.AuraColor, FriendAuraColor: friendAura.AuraColor,
		CreatedAt: match.CreatedAt,
	}, nil
}

// =============================================================================
// StreakService
// =============================================================================

type StreakService struct {
	db *gorm.DB
}

func NewStreakService(db *gorm.DB) *StreakService {
	return &StreakService{db: db}
}

var streakUnlocks = map[int]string{
	3: "silver", 7: "gold", 14: "white", 21: "rainbow", 30: "cosmic", 50: "celestial",
}

func (s *StreakService) GetOrCreate(appID string, userID uuid.UUID) (*AuraStreak, error) {
	var streak AuraStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if err == gorm.ErrRecordNotFound {
		streak = AuraStreak{
			AppID:          appID,
			UserID:         userID,
			UnlockedColors: []string{},
		}
		if err := s.db.Create(&streak).Error; err != nil {
			return nil, err
		}
		return &streak, nil
	}
	if err != nil {
		return nil, err
	}
	return &streak, nil
}

func (s *StreakService) Get(appID string, userID uuid.UUID) (*StreakResponse, error) {
	streak, err := s.GetOrCreate(appID, userID)
	if err != nil {
		return nil, err
	}

	resp := &StreakResponse{
		ID: streak.ID, UserID: streak.UserID,
		CurrentStreak: streak.CurrentStreak, LongestStreak: streak.LongestStreak,
		TotalScans: streak.TotalScans, LastScanDate: streak.LastScanDate,
		UnlockedColors: streak.UnlockedColors,
	}

	for days, color := range streakUnlocks {
		if streak.CurrentStreak < days {
			resp.NextUnlock = color
			resp.DaysUntilUnlock = days - streak.CurrentStreak
			break
		}
	}

	return resp, nil
}

func (s *StreakService) Update(appID string, userID uuid.UUID) (*StreakUpdateResponse, error) {
	streak, err := s.GetOrCreate(appID, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	lastScan := time.Date(streak.LastScanDate.Year(), streak.LastScanDate.Month(), streak.LastScanDate.Day(), 0, 0, 0, 0, streak.LastScanDate.Location())

	streakBroken := false
	message := ""
	var newUnlock string

	if !streak.LastScanDate.IsZero() && lastScan.Equal(today) {
		message = "You've already scanned today! Come back tomorrow."
		return &StreakUpdateResponse{
			Streak: StreakResponse{
				ID: streak.ID, UserID: streak.UserID,
				CurrentStreak: streak.CurrentStreak, LongestStreak: streak.LongestStreak,
				TotalScans: streak.TotalScans, LastScanDate: streak.LastScanDate,
				UnlockedColors: streak.UnlockedColors,
			},
			Message: message,
		}, nil
	}

	yesterday := today.AddDate(0, 0, -1)

	if streak.LastScanDate.IsZero() {
		streak.CurrentStreak = 1
		message = "Your aura journey begins! Day 1 streak started."
	} else if lastScan.Equal(yesterday) {
		streak.CurrentStreak++
		message = fmt.Sprintf("%d day streak! Keep going!", streak.CurrentStreak)
	} else {
		if streak.CurrentStreak > 0 {
			streakBroken = true
			message = fmt.Sprintf("New beginning! Your previous streak was %d days. Let's start fresh!", streak.CurrentStreak)
		} else {
			message = "Your aura journey begins!"
		}
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}

	streak.TotalScans++
	streak.LastScanDate = now

	for days, color := range streakUnlocks {
		if streak.CurrentStreak == days {
			newUnlock = color
			if !containsStr(streak.UnlockedColors, color) {
				streak.UnlockedColors = append(streak.UnlockedColors, color)
				message = message + " You unlocked the " + color + " aura!"
			}
			break
		}
	}

	if err := s.db.Save(streak).Error; err != nil {
		return nil, err
	}

	resp := &StreakUpdateResponse{
		Streak: StreakResponse{
			ID: streak.ID, UserID: streak.UserID,
			CurrentStreak: streak.CurrentStreak, LongestStreak: streak.LongestStreak,
			TotalScans: streak.TotalScans, LastScanDate: streak.LastScanDate,
			UnlockedColors: streak.UnlockedColors,
		},
		NewUnlock:    newUnlock,
		StreakBroken: streakBroken,
		Message:      message,
	}

	for days, color := range streakUnlocks {
		if streak.CurrentStreak < days {
			resp.Streak.NextUnlock = color
			resp.Streak.DaysUntilUnlock = days - streak.CurrentStreak
			break
		}
	}

	return resp, nil
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
