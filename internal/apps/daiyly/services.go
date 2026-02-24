package daiyly

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidMoodEmoji     = errors.New("invalid mood emoji")
	ErrInvalidMoodScore     = errors.New("mood score must be between 1 and 100")
	ErrInvalidCardColor     = errors.New("invalid card color")
	ErrJournalNotFound      = errors.New("journal entry not found")
	ErrNotOwner             = errors.New("you do not own this journal entry")
	ErrContentInappropriate = errors.New("content contains inappropriate language")
	ErrAnalysisNotFound     = errors.New("analysis not found")
)

// ContentFilterService provides content moderation functionality.
type ContentFilterService struct {
	blockedWords []string
}

func NewContentFilterService(blockedWords []string) *ContentFilterService {
	return &ContentFilterService{blockedWords: blockedWords}
}

func (f *ContentFilterService) FilterContent(content string) (bool, string) {
	if f == nil {
		return false, ""
	}
	contentLower := strings.ToLower(content)
	for _, word := range f.blockedWords {
		if strings.Contains(contentLower, strings.ToLower(word)) {
			return true, "contains blocked word"
		}
	}
	return false, ""
}

type JournalService struct {
	db            *gorm.DB
	contentFilter *ContentFilterService
	openaiKey     string
	openaiModel   string
	aiTimeout     time.Duration
}

func NewJournalService(db *gorm.DB, openaiKey, openaiModel string, aiTimeout time.Duration) *JournalService {
	model := openaiModel
	if model == "" {
		model = "gpt-4o-mini"
	}
	timeout := aiTimeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &JournalService{db: db, openaiKey: openaiKey, openaiModel: model, aiTimeout: timeout}
}

func (s *JournalService) CreateEntry(appID string, userID uuid.UUID, req CreateJournalRequest) (*JournalEntry, error) {
	if !isValidMoodEmoji(req.MoodEmoji) {
		return nil, ErrInvalidMoodEmoji
	}

	if req.MoodScore < 1 || req.MoodScore > 100 {
		return nil, ErrInvalidMoodScore
	}

	// Content filtering check
	if s.contentFilter != nil && req.Content != "" {
		flagged, _ := s.contentFilter.FilterContent(req.Content)
		if flagged {
			return nil, ErrContentInappropriate
		}
	}

	if req.CardColor == "" {
		req.CardColor = "#dbeafe"
	}
	if !isValidCardColor(req.CardColor) {
		return nil, ErrInvalidCardColor
	}

	entry := JournalEntry{
		ID:        uuid.New(),
		AppID:     appID,
		UserID:    userID,
		MoodEmoji: req.MoodEmoji,
		MoodScore: req.MoodScore,
		Content:   req.Content,
		PhotoURL:  req.PhotoURL,
		CardColor: req.CardColor,
		EntryDate: time.Now().UTC(),
		IsPrivate: req.IsPrivate,
	}

	if err := s.db.Create(&entry).Error; err != nil {
		return nil, err
	}

	// Streak update is best-effort; entry was already saved
	if err := s.UpdateStreak(appID, userID); err != nil {
		// Non-critical: the journal entry was created successfully
		_ = err
	}

	// Fire-and-forget AI analysis
	if s.openaiKey != "" && entry.Content != "" {
		go s.analyzeEntryAsync(appID, userID, entry.ID)
	}

	return &entry, nil
}

func (s *JournalService) GetEntries(appID string, userID uuid.UUID, limit, offset int) ([]JournalEntry, int64, error) {
	var entries []JournalEntry
	var total int64

	if err := s.db.Model(&JournalEntry{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).
		Order("entry_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&entries).Error

	return entries, total, err
}

func (s *JournalService) SearchEntries(appID string, userID uuid.UUID, query string, limit, offset int) (*SearchJournalResponse, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil, errors.New("search query must be at least 2 characters")
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var entries []JournalEntry
	var total int64

	searchPattern := "%" + query + "%"

	countQuery := s.db.Model(&JournalEntry{}).Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND (content ILIKE ? OR mood_emoji = ?)",
			userID, searchPattern, query)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, errors.New("failed to count search results")
	}

	fetchQuery := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND (content ILIKE ? OR mood_emoji = ?)",
			userID, searchPattern, query).
		Order("entry_date DESC").
		Limit(limit).
		Offset(offset)

	if err := fetchQuery.Find(&entries).Error; err != nil {
		return nil, errors.New("failed to fetch search results")
	}

	return &SearchJournalResponse{
		Entries: entries,
		Total:   total,
		Query:   query,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

func (s *JournalService) GetEntry(appID string, userID uuid.UUID, entryID uuid.UUID) (*JournalEntry, error) {
	var entry JournalEntry
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&entry, "id = ?", entryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJournalNotFound
		}
		return nil, err
	}

	if entry.UserID != userID {
		return nil, ErrNotOwner
	}

	return &entry, nil
}

func (s *JournalService) UpdateEntry(appID string, userID uuid.UUID, entryID uuid.UUID, req UpdateJournalRequest) (*JournalEntry, error) {
	entry, err := s.GetEntry(appID, userID, entryID)
	if err != nil {
		return nil, err
	}

	if req.Content != nil && *req.Content != "" && s.contentFilter != nil {
		flagged, _ := s.contentFilter.FilterContent(*req.Content)
		if flagged {
			return nil, ErrContentInappropriate
		}
	}

	if req.MoodEmoji != nil {
		if !isValidMoodEmoji(*req.MoodEmoji) {
			return nil, ErrInvalidMoodEmoji
		}
		entry.MoodEmoji = *req.MoodEmoji
	}

	if req.MoodScore != nil {
		if *req.MoodScore < 1 || *req.MoodScore > 100 {
			return nil, ErrInvalidMoodScore
		}
		entry.MoodScore = *req.MoodScore
	}

	if req.Content != nil {
		entry.Content = *req.Content
	}

	if req.PhotoURL != nil {
		entry.PhotoURL = *req.PhotoURL
	}

	if req.CardColor != nil {
		if !isValidCardColor(*req.CardColor) {
			return nil, ErrInvalidCardColor
		}
		entry.CardColor = *req.CardColor
	}

	if req.IsPrivate != nil {
		entry.IsPrivate = *req.IsPrivate
	}

	if err := s.db.Save(entry).Error; err != nil {
		return nil, err
	}

	return entry, nil
}

func (s *JournalService) DeleteEntry(appID string, userID uuid.UUID, entryID uuid.UUID) error {
	entry, err := s.GetEntry(appID, userID, entryID)
	if err != nil {
		return err
	}

	return s.db.Delete(entry).Error
}

func (s *JournalService) GetStreak(appID string, userID uuid.UUID) (*JournalStreak, error) {
	var streak JournalStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = JournalStreak{
			ID:            uuid.New(),
			AppID:         appID,
			UserID:        userID,
			CurrentStreak: 0,
			LongestStreak: 0,
			TotalEntries:  0,
		}
		if createErr := s.db.Create(&streak).Error; createErr != nil {
			return nil, createErr
		}
		return &streak, nil
	}
	if err != nil {
		return nil, err
	}
	return &streak, nil
}

func (s *JournalService) UpdateStreak(appID string, userID uuid.UUID) error {
	streak, err := s.GetStreak(appID, userID)
	if err != nil {
		return err
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	lastEntry := streak.LastEntryDate.UTC().Truncate(24 * time.Hour)

	if today.Equal(lastEntry) {
		return nil
	}

	yesterday := today.AddDate(0, 0, -1)
	if lastEntry.Equal(yesterday) {
		streak.CurrentStreak++
	} else {
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}

	streak.TotalEntries++
	streak.LastEntryDate = time.Now().UTC()

	return s.db.Save(streak).Error
}

func (s *JournalService) GetWeeklyInsights(appID string, userID uuid.UUID) (*WeeklyInsights, error) {
	sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -7)

	var entries []JournalEntry
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND entry_date >= ?", userID, sevenDaysAgo).
		Order("entry_date ASC").
		Find(&entries).Error
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return &WeeklyInsights{
			AverageMoodScore: 0,
			MoodTrend:        "stable",
			TopMood:          "",
			TotalEntries:     0,
			DailyScores:      []DailyScore{},
			MoodDistribution: map[string]int{},
			WritingStats:     WritingStats{AvgWordCount: 0, TotalWords: 0},
			TimePattern:      map[string]int{"morning": 0, "afternoon": 0, "evening": 0, "night": 0},
			StreakData:       StreakData{Current: 0, Longest: 0, Total: 0},
		}, nil
	}

	totalScore := 0
	emojiCount := make(map[string]int)
	dailyScores := []DailyScore{}
	totalWords := 0
	timePattern := map[string]int{
		"morning":   0,
		"afternoon": 0,
		"evening":   0,
		"night":     0,
	}

	for _, e := range entries {
		totalScore += e.MoodScore
		emojiCount[e.MoodEmoji]++
		dailyScores = append(dailyScores, DailyScore{
			Date:  e.EntryDate.Format("2006-01-02"),
			Score: e.MoodScore,
		})

		wordCount := len(strings.Fields(e.Content))
		totalWords += wordCount

		hour := e.EntryDate.Hour()
		switch {
		case hour >= 5 && hour < 12:
			timePattern["morning"]++
		case hour >= 12 && hour < 17:
			timePattern["afternoon"]++
		case hour >= 17 && hour < 21:
			timePattern["evening"]++
		default:
			timePattern["night"]++
		}
	}

	avgScore := totalScore / len(entries)
	avgWordCount := totalWords / len(entries)

	topMood := ""
	maxCount := 0
	for emoji, count := range emojiCount {
		if count > maxCount {
			maxCount = count
			topMood = emoji
		}
	}

	trend := "stable"
	if len(entries) >= 2 {
		mid := len(entries) / 2
		firstHalfTotal := 0
		for i := 0; i < mid; i++ {
			firstHalfTotal += entries[i].MoodScore
		}
		secondHalfTotal := 0
		for i := mid; i < len(entries); i++ {
			secondHalfTotal += entries[i].MoodScore
		}
		firstHalfAvg := firstHalfTotal / mid
		secondHalfAvg := secondHalfTotal / (len(entries) - mid)
		diff := secondHalfAvg - firstHalfAvg
		if diff > 5 {
			trend = "improving"
		} else if diff < -5 {
			trend = "declining"
		}
	}

	// Fetch streak data
	var streak JournalStreak
	streakResult := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak)
	streakData := StreakData{Current: 0, Longest: 0, Total: 0}
	if streakResult.Error == nil {
		streakData.Current = streak.CurrentStreak
		streakData.Longest = streak.LongestStreak
		streakData.Total = streak.TotalEntries
	}

	return &WeeklyInsights{
		AverageMoodScore: avgScore,
		MoodTrend:        trend,
		TopMood:          topMood,
		TotalEntries:     len(entries),
		DailyScores:      dailyScores,
		MoodDistribution: emojiCount,
		WritingStats: WritingStats{
			AvgWordCount: avgWordCount,
			TotalWords:   totalWords,
		},
		TimePattern: timePattern,
		StreakData:  streakData,
	}, nil
}

// calculateTopMoods returns the 3 most frequently used moods.
func calculateTopMoods(distribution map[string]int) []string {
	type moodCount struct {
		mood  string
		count int
	}

	var moodCounts []moodCount
	for mood, count := range distribution {
		moodCounts = append(moodCounts, moodCount{mood, count})
	}

	sort.Slice(moodCounts, func(i, j int) bool {
		return moodCounts[i].count > moodCounts[j].count
	})

	result := make([]string, 0, 3)
	for i := 0; i < 3 && i < len(moodCounts); i++ {
		result = append(result, moodCounts[i].mood)
	}

	return result
}

func isValidMoodEmoji(emoji string) bool {
	for _, valid := range MoodEmojis {
		if emoji == valid {
			return true
		}
	}
	return false
}

func isValidCardColor(color string) bool {
	for _, valid := range CardColors {
		if color == valid {
			return true
		}
	}
	return false
}

// --- OpenAI Integration ---

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *JournalService) callOpenAI(systemPrompt, userPrompt string) (string, error) {
	reqBody := openAIChatRequest{
		Model: s.openaiModel,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.openaiKey)

	client := &http.Client{Timeout: s.aiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content, nil
}

// --- AI Service Methods ---

func (s *JournalService) analyzeEntryAsync(appID string, userID, entryID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[daiyly] analyzeEntryAsync panic: %v", r)
		}
	}()

	var entry JournalEntry
	if err := s.db.First(&entry, "id = ?", entryID).Error; err != nil {
		return
	}

	analysis := EntryAnalysis{
		ID:      uuid.New(),
		AppID:   appID,
		UserID:  userID,
		EntryID: entryID,
		Status:  "pending",
	}
	if err := s.db.Create(&analysis).Error; err != nil {
		return // unique index violation means already analyzing
	}

	systemPrompt := `You are a compassionate journal analyst. Analyze the following journal entry and respond with JSON only (no markdown, no code fences):
{"themes":["theme1","theme2"],"sentiment_label":"positive","sentiment_score":0.5,"cognitive_patterns":[],"insight":"A brief 2-3 sentence empathetic insight."}

Rules:
- themes: 2-4 detected themes (e.g. "work stress", "family", "gratitude", "health")
- sentiment_label: one of "positive", "negative", "neutral", "mixed"
- sentiment_score: float from -1.0 (very negative) to 1.0 (very positive)
- cognitive_patterns: empty array if none detected, otherwise patterns like "catastrophizing", "all-or-nothing thinking", "overgeneralization"
- insight: warm, empathetic, non-judgmental paragraph`

	userPrompt := fmt.Sprintf("Mood: %s (score: %d/100)\n\n%s", entry.MoodEmoji, entry.MoodScore, entry.Content)

	content, err := s.callOpenAI(systemPrompt, userPrompt)
	if err != nil {
		s.db.Model(&analysis).Update("status", "failed")
		return
	}

	var parsed struct {
		Themes            []string `json:"themes"`
		SentimentLabel    string   `json:"sentiment_label"`
		SentimentScore    float64  `json:"sentiment_score"`
		CognitivePatterns []string `json:"cognitive_patterns"`
		Insight           string   `json:"insight"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		s.db.Model(&analysis).Update("status", "failed")
		return
	}

	themesJSON, _ := json.Marshal(parsed.Themes)
	patternsJSON, _ := json.Marshal(parsed.CognitivePatterns)

	s.db.Model(&analysis).Updates(map[string]interface{}{
		"themes":             string(themesJSON),
		"sentiment_label":    parsed.SentimentLabel,
		"sentiment_score":    parsed.SentimentScore,
		"cognitive_patterns": string(patternsJSON),
		"insight":            parsed.Insight,
		"status":             "completed",
	})
}

func (s *JournalService) GetEntryAnalysis(appID string, userID, entryID uuid.UUID) (*EntryAnalysisResponse, error) {
	var analysis EntryAnalysis
	err := s.db.Where("app_id = ? AND user_id = ? AND entry_id = ?", appID, userID, entryID).First(&analysis).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnalysisNotFound
		}
		return nil, err
	}

	var themes []string
	var patterns []string
	json.Unmarshal([]byte(analysis.Themes), &themes)
	json.Unmarshal([]byte(analysis.CognitivePatterns), &patterns)
	if themes == nil {
		themes = []string{}
	}
	if patterns == nil {
		patterns = []string{}
	}

	return &EntryAnalysisResponse{
		Themes:            themes,
		SentimentLabel:    analysis.SentimentLabel,
		SentimentScore:    analysis.SentimentScore,
		CognitivePatterns: patterns,
		Insight:           analysis.Insight,
		Status:            analysis.Status,
	}, nil
}

func (s *JournalService) TriggerAnalysis(appID string, userID, entryID uuid.UUID) error {
	// Delete existing analysis if any, then re-analyze
	s.db.Where("entry_id = ?", entryID).Delete(&EntryAnalysis{})
	go s.analyzeEntryAsync(appID, userID, entryID)
	return nil
}

func (s *JournalService) GetPersonalizedPrompts(appID string, userID uuid.UUID) (*PromptsResponse, error) {
	genericPrompts := []JournalPrompt{
		{Text: "What are you grateful for today?", Category: "gratitude"},
		{Text: "Describe a challenge you overcame recently.", Category: "reflection"},
		{Text: "What is one small goal for tomorrow?", Category: "goal"},
		{Text: "How are you really feeling right now?", Category: "emotional"},
	}

	if s.openaiKey == "" {
		return &PromptsResponse{Prompts: genericPrompts}, nil
	}

	sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -7)
	var entries []JournalEntry
	s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND entry_date >= ?", userID, sevenDaysAgo).
		Order("entry_date DESC").Limit(10).Find(&entries)

	if len(entries) == 0 {
		return &PromptsResponse{Prompts: genericPrompts}, nil
	}

	var summary strings.Builder
	for _, e := range entries {
		preview := e.Content
		if len(preview) > 150 {
			preview = preview[:150] + "..."
		}
		summary.WriteString(fmt.Sprintf("- %s (mood: %s, score: %d): %s\n", e.EntryDate.Format("Jan 2"), e.MoodEmoji, e.MoodScore, preview))
	}

	systemPrompt := `You are a journaling coach. Based on the user's recent journal entries, generate 3-5 personalized journaling prompts. Respond with JSON only (no markdown, no code fences):
{"prompts":[{"text":"prompt text under 100 chars","category":"gratitude"},{"text":"...","category":"reflection"}]}

Categories: gratitude, reflection, goal, emotional
Rules:
- Make prompts diverse across categories
- Reference observed mood patterns subtly
- Keep each prompt under 100 characters
- Be warm, encouraging, and non-judgmental`

	content, err := s.callOpenAI(systemPrompt, summary.String())
	if err != nil {
		return &PromptsResponse{Prompts: genericPrompts}, nil
	}

	var parsed struct {
		Prompts []JournalPrompt `json:"prompts"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil || len(parsed.Prompts) == 0 {
		return &PromptsResponse{Prompts: genericPrompts}, nil
	}

	return &PromptsResponse{Prompts: parsed.Prompts}, nil
}

func (s *JournalService) GetWeeklyReport(appID string, userID uuid.UUID, forceRefresh bool) (*WeeklyReportResponse, error) {
	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	weekStart := now.AddDate(0, 0, -(weekday - 1)).Truncate(24 * time.Hour)

	// Check cache
	if !forceRefresh {
		var cached WeeklyReport
		err := s.db.Where("app_id = ? AND user_id = ? AND week_start = ?", appID, userID, weekStart).First(&cached).Error
		if err == nil {
			var themes []string
			json.Unmarshal([]byte(cached.KeyThemes), &themes)
			if themes == nil {
				themes = []string{}
			}

			stats, _ := s.GetWeeklyInsights(appID, userID)
			if stats == nil {
				stats = &WeeklyInsights{}
			}

			return &WeeklyReportResponse{
				Narrative:       cached.Narrative,
				KeyThemes:       themes,
				MoodExplanation: cached.MoodExplanation,
				Suggestion:      cached.Suggestion,
				WeekStart:       weekStart.Format("2006-01-02"),
				Stats:           *stats,
			}, nil
		}
	} else {
		s.db.Where("app_id = ? AND user_id = ? AND week_start = ?", appID, userID, weekStart).Delete(&WeeklyReport{})
	}

	stats, err := s.GetWeeklyInsights(appID, userID)
	if err != nil {
		return nil, err
	}

	if stats.TotalEntries == 0 {
		return &WeeklyReportResponse{
			Narrative:       "You haven't written any entries this week yet. Start journaling to get your personalized weekly summary!",
			KeyThemes:       []string{},
			MoodExplanation: "",
			Suggestion:      "Try writing just one sentence about how you feel today.",
			WeekStart:       weekStart.Format("2006-01-02"),
			Stats:           *stats,
		}, nil
	}

	if s.openaiKey == "" {
		return &WeeklyReportResponse{
			Narrative:       fmt.Sprintf("This week you wrote %d entries with an average mood score of %d.", stats.TotalEntries, stats.AverageMoodScore),
			KeyThemes:       []string{},
			MoodExplanation: fmt.Sprintf("Your mood trend is %s.", stats.MoodTrend),
			Suggestion:      "Keep journaling daily to build deeper insights.",
			WeekStart:       weekStart.Format("2006-01-02"),
			Stats:           *stats,
		}, nil
	}

	sevenDaysAgo := time.Now().UTC().AddDate(0, 0, -7)
	var entries []JournalEntry
	s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND entry_date >= ?", userID, sevenDaysAgo).
		Order("entry_date ASC").Find(&entries)

	var summary strings.Builder
	for _, e := range entries {
		preview := e.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		summary.WriteString(fmt.Sprintf("- %s %s (score: %d): %s\n", e.EntryDate.Format("Mon Jan 2"), e.MoodEmoji, e.MoodScore, preview))
	}

	systemPrompt := `You are a warm, insightful journaling companion. Write a weekly summary for this user. Respond with JSON only (no markdown, no code fences):
{"narrative":"3-4 sentence warm overview of their week","key_themes":["2-4 themes"],"mood_explanation":"1-2 sentences explaining their mood pattern","suggestion":"1 specific actionable suggestion for next week"}

Rules:
- Be empathetic, warm, and encouraging
- Reference specific patterns from their entries
- The suggestion should be concrete and achievable
- Never be preachy or judgmental`

	statsContext := fmt.Sprintf("Stats: %d entries, avg mood %d/100, trend: %s, top mood: %s\n\nEntries:\n%s",
		stats.TotalEntries, stats.AverageMoodScore, stats.MoodTrend, stats.TopMood, summary.String())

	content, err := s.callOpenAI(systemPrompt, statsContext)
	if err != nil {
		return &WeeklyReportResponse{
			Narrative:       fmt.Sprintf("This week you wrote %d entries with an average mood score of %d.", stats.TotalEntries, stats.AverageMoodScore),
			KeyThemes:       []string{},
			MoodExplanation: fmt.Sprintf("Your mood trend is %s.", stats.MoodTrend),
			Suggestion:      "Keep journaling daily to build deeper insights.",
			WeekStart:       weekStart.Format("2006-01-02"),
			Stats:           *stats,
		}, nil
	}

	var parsed struct {
		Narrative       string   `json:"narrative"`
		KeyThemes       []string `json:"key_themes"`
		MoodExplanation string   `json:"mood_explanation"`
		Suggestion      string   `json:"suggestion"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return &WeeklyReportResponse{
			Narrative:       fmt.Sprintf("This week you wrote %d entries with an average mood score of %d.", stats.TotalEntries, stats.AverageMoodScore),
			KeyThemes:       []string{},
			MoodExplanation: fmt.Sprintf("Your mood trend is %s.", stats.MoodTrend),
			Suggestion:      "Keep journaling daily to build deeper insights.",
			WeekStart:       weekStart.Format("2006-01-02"),
			Stats:           *stats,
		}, nil
	}

	themesJSON, _ := json.Marshal(parsed.KeyThemes)
	report := WeeklyReport{
		ID:              uuid.New(),
		AppID:           appID,
		UserID:          userID,
		WeekStart:       weekStart,
		Narrative:       parsed.Narrative,
		KeyThemes:       string(themesJSON),
		MoodExplanation: parsed.MoodExplanation,
		Suggestion:      parsed.Suggestion,
	}
	s.db.Create(&report) // best-effort cache

	return &WeeklyReportResponse{
		Narrative:       parsed.Narrative,
		KeyThemes:       parsed.KeyThemes,
		MoodExplanation: parsed.MoodExplanation,
		Suggestion:      parsed.Suggestion,
		WeekStart:       weekStart.Format("2006-01-02"),
		Stats:           *stats,
	}, nil
}

func (s *JournalService) GetFlashbacks(appID string, userID uuid.UUID) (*FlashbacksResponse, error) {
	now := time.Now().UTC()
	var flashbacks []FlashbackEntry

	periods := []struct {
		label   string
		daysAgo int
	}{
		{"1 week ago", 7},
		{"1 month ago", 30},
		{"1 year ago", 365},
	}

	for _, p := range periods {
		targetDate := now.AddDate(0, 0, -p.daysAgo)
		startOfDay := targetDate.Truncate(24 * time.Hour)
		endOfDay := startOfDay.Add(24 * time.Hour)

		var entry JournalEntry
		err := s.db.Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND entry_date >= ? AND entry_date < ?", userID, startOfDay, endOfDay).
			Order("entry_date DESC").
			First(&entry).Error

		if err == nil {
			flashbacks = append(flashbacks, FlashbackEntry{
				Entry:   entry,
				Period:  p.label,
				DaysAgo: p.daysAgo,
			})
		}
	}

	return &FlashbacksResponse{Entries: flashbacks}, nil
}
