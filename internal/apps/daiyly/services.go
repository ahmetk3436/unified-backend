package daiyly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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
	ErrInvalidPhotoURL      = errors.New("photo_url must be an https:// URL of at most 2048 characters")
	ErrInvalidAudioURL      = errors.New("audio_url must be an https:// URL of at most 2048 characters")
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
	db                *gorm.DB
	contentFilter     *ContentFilterService
	aiAPIKey          string
	aiAPIURL          string
	aiModel           string
	aiTimeout         time.Duration
	openAIAPIKey      string
	openAIModel       string
	emotionSenseMLURL string
}

func NewJournalService(db *gorm.DB, aiAPIKey, aiAPIURL, aiModel string, aiTimeout time.Duration, openAIAPIKey, openAIModel, emotionSenseMLURL string) *JournalService {
	if aiAPIURL == "" {
		aiAPIURL = "https://api.z.ai/api/paas/v4/chat/completions"
	}
	if aiModel == "" {
		aiModel = "glm-5"
	}
	if aiTimeout == 0 {
		aiTimeout = 60 * time.Second
	}
	if openAIModel == "" {
		openAIModel = "gpt-4o-mini"
	}
	return &JournalService{
		db:                db,
		aiAPIKey:          aiAPIKey,
		aiAPIURL:          aiAPIURL,
		aiModel:           aiModel,
		aiTimeout:         aiTimeout,
		openAIAPIKey:      openAIAPIKey,
		openAIModel:       openAIModel,
		emotionSenseMLURL: emotionSenseMLURL,
	}
}

func (s *JournalService) CreateEntry(appID string, userID uuid.UUID, req CreateJournalRequest) (*JournalEntry, error) {
	if !isValidMoodEmoji(req.MoodEmoji) {
		return nil, ErrInvalidMoodEmoji
	}

	if req.MoodScore < 1 || req.MoodScore > 100 {
		return nil, ErrInvalidMoodScore
	}

	if len(req.Content) > 50000 {
		return nil, errors.New("content too long (max 50000 characters)")
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

	// Use client's local date if provided and valid (prevents timezone drift for late-night entries).
	// Format: "YYYY-MM-DD". Falls back to server UTC if missing or invalid.
	entryDate := time.Now().UTC()
	if req.EntryDate != "" {
		if parsed, err := time.Parse("2006-01-02", req.EntryDate); err == nil {
			// Sanity: reject dates more than 1 day in the future or more than 7 days in the past
			now := time.Now().UTC()
			if parsed.After(now.AddDate(0, 0, -7)) && parsed.Before(now.AddDate(0, 0, 2)) {
				entryDate = parsed
			}
		}
	}

	if !isValidStorageURL(req.PhotoURL) {
		return nil, ErrInvalidPhotoURL
	}

	if !isValidStorageURL(req.AudioURL) {
		return nil, ErrInvalidAudioURL
	}

	if len(req.Transcript) > 50000 {
		return nil, errors.New("transcript too long (max 50000 characters)")
	}

	entry := JournalEntry{
		ID:         uuid.New(),
		AppID:      appID,
		UserID:     userID,
		MoodEmoji:  req.MoodEmoji,
		MoodScore:  req.MoodScore,
		Content:    req.Content,
		PhotoURL:   req.PhotoURL,
		AudioURL:   req.AudioURL,
		Transcript: req.Transcript,
		CardColor:  req.CardColor,
		EntryDate:  entryDate,
		IsPrivate:  req.IsPrivate,
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
	if s.aiAPIKey != "" && entry.Content != "" {
		go s.analyzeEntryAsync(appID, userID, entry.ID)
	}

	// Fire async emotion analysis (non-blocking)
	s.analyzeEmotionAsync(appID, entry.ID.String(), entry.Content)

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
	if len(query) > 100 {
		query = query[:100]
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var entries []JournalEntry
	var total int64

	// Escape LIKE wildcards so user-supplied % and _ are treated as literals.
	// The backslash escape character must be doubled first to avoid double-escaping.
	escapedQuery := strings.ReplaceAll(query, `\`, `\\`)
	escapedQuery = strings.ReplaceAll(escapedQuery, `%`, `\%`)
	escapedQuery = strings.ReplaceAll(escapedQuery, `_`, `\_`)
	searchPattern := "%" + escapedQuery + "%"

	countQuery := s.db.Model(&JournalEntry{}).Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND (content ILIKE ? ESCAPE E'\\\\' OR mood_emoji = ?)",
			userID, searchPattern, query)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, errors.New("failed to count search results")
	}

	fetchQuery := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND (content ILIKE ? ESCAPE E'\\\\' OR mood_emoji = ?)",
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

	if req.Content != nil && len(*req.Content) > 50000 {
		return nil, errors.New("content too long (max 50000 characters)")
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
		if !isValidStorageURL(*req.PhotoURL) {
			return nil, ErrInvalidPhotoURL
		}
		entry.PhotoURL = *req.PhotoURL
	}

	if req.AudioURL != nil {
		if !isValidStorageURL(*req.AudioURL) {
			return nil, ErrInvalidAudioURL
		}
		entry.AudioURL = *req.AudioURL
	}

	if req.Transcript != nil {
		if len(*req.Transcript) > 50000 {
			return nil, errors.New("transcript too long (max 50000 characters)")
		}
		entry.Transcript = *req.Transcript
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

	// Fire async emotion analysis when content was updated (non-blocking)
	if req.Content != nil && *req.Content != "" {
		s.analyzeEmotionAsync(appID, entry.ID.String(), entry.Content)
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

	// Already journaled today — nothing to do.
	if today.Equal(lastEntry) {
		return nil
	}

	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	if lastEntry.Equal(yesterday) {
		// Consecutive day — extend streak normally.
		// If grace was active it is now consumed; clear it.
		streak.CurrentStreak++
		streak.GracePeriodActive = false
	} else if lastEntry.Equal(twoDaysAgo) && streak.CurrentStreak >= 7 {
		// User missed exactly one day and has a streak of 7+.
		// Grant grace period if they haven't used one in the last 7 days.
		graceAllowed := streak.GracePeriodUsedAt == nil ||
			time.Since(*streak.GracePeriodUsedAt) >= 7*24*time.Hour

		if graceAllowed {
			// Keep streak alive; mark grace active so the client can show a warning.
			streak.GracePeriodActive = true
			now := time.Now().UTC()
			streak.GracePeriodUsedAt = &now
			// Do NOT increment CurrentStreak — the user hasn't journaled yet today.
			// Increment TotalEntries below still happens because they ARE journaling now.
		} else {
			// Grace already used within 7 days — reset streak.
			streak.CurrentStreak = 1
			streak.GracePeriodActive = false
		}
	} else {
		// Gap is 3+ days or streak < 3: reset.
		streak.CurrentStreak = 1
		streak.GracePeriodActive = false
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

// isValidStorageURL accepts empty values (field is optional) and requires https:// scheme
// and a maximum length of 2048 chars. This prevents: (1) excessively long URLs being
// stored in the DB, (2) http:// URLs that would cause mixed-content on the client.
// Used for both photo_url and audio_url fields.
func isValidStorageURL(url string) bool {
	if url == "" {
		return true
	}
	if len(url) > 2048 {
		return false
	}
	return strings.HasPrefix(url, "https://")
}

// isValidPhotoURL is an alias kept for backward compatibility.
// New callers should use isValidStorageURL.
func isValidPhotoURL(url string) bool { return isValidStorageURL(url) }

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
		Model: s.aiModel,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", s.aiAPIURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.aiAPIKey)

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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&chatResp); err != nil {
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

// callOpenAIChat calls the native OpenAI chat completions endpoint using s.openAIAPIKey and s.openAIModel.
// Falls back to callOpenAI (GLM) when openAIAPIKey is not configured.
func (s *JournalService) callOpenAIChat(systemPrompt, userPrompt string) (string, error) {
	if s.openAIAPIKey == "" {
		// Fallback: use the GLM-compatible endpoint.
		return s.callOpenAI(systemPrompt, userPrompt)
	}

	reqBody := openAIChatRequest{
		Model: s.openAIModel,
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
	req.Header.Set("Authorization", "Bearer "+s.openAIAPIKey)

	client := &http.Client{Timeout: s.aiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai chat returned status %d", resp.StatusCode)
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown code fences if present.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content, nil
}

// --- EmotionSenseML ---

// analyzeEmotionAsync calls EmotionSenseML in a goroutine after entry creation or content update.
// It updates the entry with detected_emotion, emotion_scores, and emotion_analyzed_at.
// Never blocks the main request path. All failures are silently swallowed.
func (s *JournalService) analyzeEmotionAsync(appID, entryID, content string) {
	if s.emotionSenseMLURL == "" || len(content) < 10 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		encoded := url.QueryEscape(content)
		req, err := http.NewRequestWithContext(ctx, "POST",
			s.emotionSenseMLURL+"/api/v1/analyze/text?content="+encoded, nil)
		if err != nil {
			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return
		}

		var result struct {
			DominantEmotion string `json:"dominantEmotion"`
			Emotions        []struct {
				Type  string  `json:"type"`
				Score float64 `json:"score"`
			} `json:"emotions"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return
		}

		if result.DominantEmotion == "" {
			return
		}

		// Normalize known label typos from the voice model that may bleed into text responses.
		emotionMap := map[string]string{
			"suprised": "surprise",
			"fearful":  "fear",
		}
		emotion := result.DominantEmotion
		if normalized, ok := emotionMap[emotion]; ok {
			emotion = normalized
		}

		scoresJSON, err := json.Marshal(result.Emotions)
		if err != nil {
			return
		}
		scoresStr := string(scoresJSON)
		now := time.Now()

		s.db.Model(&JournalEntry{}).
			Where("id = ? AND app_id = ?", entryID, appID).
			Updates(map[string]interface{}{
				"detected_emotion":    emotion,
				"emotion_scores":      scoresStr,
				"emotion_analyzed_at": now,
			})
	}()
}

// --- AI Service Methods ---

func (s *JournalService) analyzeEntryAsync(appID string, userID, entryID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("[daiyly] analyzeEntryAsync panic", "panic", r)
		}
	}()

	var entry JournalEntry
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&entry, "id = ? AND user_id = ?", entryID, userID).Error; err != nil {
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
	// Delete existing analysis only if it belongs to this user in this app.
	// Scoping by user_id + app_id prevents a crafted entryID from deleting
	// another user's analysis even if the caller somehow bypassed ownership checks.
	s.db.Where("entry_id = ? AND user_id = ? AND app_id = ?", entryID, userID, appID).Delete(&EntryAnalysis{})
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

	if s.aiAPIKey == "" {
		return &PromptsResponse{Prompts: genericPrompts}, nil
	}

	// Check daily cache first
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var cached DailyPromptCache
	err := s.db.Where("app_id = ? AND user_id = ? AND prompt_date = ?", appID, userID, today).First(&cached).Error
	if err == nil && cached.PromptsJSON != "" {
		var prompts []JournalPrompt
		if err := json.Unmarshal([]byte(cached.PromptsJSON), &prompts); err == nil && len(prompts) > 0 {
			return &PromptsResponse{Prompts: prompts}, nil
		}
	}

	// Cache miss — generate via AI
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

	// Save to daily cache (upsert — delete old + create new)
	promptsJSON, _ := json.Marshal(parsed.Prompts)
	s.db.Where("app_id = ? AND user_id = ? AND prompt_date = ?", appID, userID, today).Delete(&DailyPromptCache{})
	s.db.Create(&DailyPromptCache{
		ID:          uuid.New(),
		AppID:       appID,
		UserID:      userID,
		PromptDate:  today,
		PromptsJSON: string(promptsJSON),
	})

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

	if s.aiAPIKey == "" {
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

func (s *JournalService) GetNotificationConfig(appID string, userID uuid.UUID) (*NotificationConfigResponse, error) {
	// --- 1. Calculate optimal time from user's journaling patterns ---
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)
	var entries []JournalEntry
	s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND entry_date >= ?", userID, thirtyDaysAgo).
		Find(&entries)

	suggestedHour := 20 // default 8 PM
	suggestedMinute := 0

	if len(entries) >= 3 {
		// Count entries per hour
		hourCounts := make(map[int]int)
		for _, e := range entries {
			h := e.EntryDate.Hour()
			hourCounts[h]++
		}
		// Find peak hour
		peakHour := 20
		peakCount := 0
		for h, c := range hourCounts {
			if c > peakCount {
				peakCount = c
				peakHour = h
			}
		}
		// Suggest 1 hour before peak (remind before they usually write)
		suggestedHour = peakHour - 1
		if suggestedHour < 0 {
			suggestedHour = 23
		}
		suggestedMinute = 0
	}

	// --- 2. Check daily cache for messages ---
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var cached NotificationConfigCache
	err := s.db.Where("app_id = ? AND user_id = ? AND config_date = ?", appID, userID, today).First(&cached).Error
	if err == nil && cached.MessagesJSON != "" {
		var msgs struct {
			Daily  []NotificationMessage `json:"daily"`
			Streak []NotificationMessage `json:"streak"`
		}
		if err := json.Unmarshal([]byte(cached.MessagesJSON), &msgs); err == nil && len(msgs.Daily) > 0 {
			return &NotificationConfigResponse{
				SuggestedHour:   suggestedHour,
				SuggestedMinute: suggestedMinute,
				DailyMessages:   msgs.Daily,
				StreakMessages:   msgs.Streak,
			}, nil
		}
	}

	// --- 3. Generate personalized messages via AI ---
	defaultDaily := []NotificationMessage{
		{Title: "Time to Journal", Body: "Take a moment to reflect on your day."},
		{Title: "How was your day?", Body: "A few words can make a big difference."},
		{Title: "Your journal awaits", Body: "What made you smile today?"},
		{Title: "Pause and reflect", Body: "Even one sentence counts."},
		{Title: "Evening check-in", Body: "How are you really feeling right now?"},
		{Title: "Capture this moment", Body: "Future you will thank you for writing today."},
		{Title: "Daily reflection time", Body: "What's on your mind tonight?"},
	}
	defaultStreak := []NotificationMessage{
		{Title: "Keep your streak alive!", Body: "A quick entry is all it takes."},
		{Title: "Don't break the chain!", Body: "Your consistency is building something great."},
		{Title: "Streak check!", Body: "You haven't written today yet — still time!"},
		{Title: "Almost missed today!", Body: "Just one sentence keeps your streak going."},
		{Title: "Your streak matters", Body: "Small habits lead to big changes."},
		{Title: "Last chance today!", Body: "A quick note keeps your journey alive."},
		{Title: "Streak reminder", Body: "Don't let today slip by without a word."},
	}

	if s.aiAPIKey == "" || len(entries) == 0 {
		return &NotificationConfigResponse{
			SuggestedHour:   suggestedHour,
			SuggestedMinute: suggestedMinute,
			DailyMessages:   defaultDaily,
			StreakMessages:   defaultStreak,
		}, nil
	}

	// Build context for AI
	var streak JournalStreak
	streakCount := 0
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error; err == nil {
		streakCount = streak.CurrentStreak
	}

	// Recent mood summary
	recentMoods := make(map[string]int)
	totalScore := 0
	for _, e := range entries {
		if len(entries) > 10 {
			break
		}
		recentMoods[e.MoodEmoji]++
		totalScore += e.MoodScore
	}
	avgScore := 50
	if len(entries) > 0 {
		count := len(entries)
		if count > 10 {
			count = 10
		}
		avgScore = totalScore / count
	}

	topMood := ""
	topMoodCount := 0
	for emoji, count := range recentMoods {
		if count > topMoodCount {
			topMoodCount = count
			topMood = emoji
		}
	}

	systemPrompt := `You are a notification copywriter for a journaling app. Generate personalized push notification messages. Respond with JSON only (no markdown, no code fences):
{"daily":[{"title":"short title","body":"short body under 60 chars"}],"streak":[{"title":"short title","body":"short body under 60 chars"}]}

Rules:
- Generate exactly 7 daily messages and 7 streak messages
- Daily: warm, inviting, reference user's mood patterns subtly
- Streak: urgent but friendly, motivate them to not break their chain
- Keep titles under 30 chars, body under 60 chars
- Vary tone: some warm, some playful, some reflective
- Never be pushy or guilt-tripping
- Reference their patterns naturally (e.g. "you've been feeling reflective lately")`

	userContext := fmt.Sprintf("User context: %d-day streak, avg mood %d/100, top mood %s, %d entries in last 30 days",
		streakCount, avgScore, topMood, len(entries))

	content, err := s.callOpenAI(systemPrompt, userContext)
	if err != nil {
		return &NotificationConfigResponse{
			SuggestedHour:   suggestedHour,
			SuggestedMinute: suggestedMinute,
			DailyMessages:   defaultDaily,
			StreakMessages:   defaultStreak,
		}, nil
	}

	var parsed struct {
		Daily  []NotificationMessage `json:"daily"`
		Streak []NotificationMessage `json:"streak"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil || len(parsed.Daily) == 0 {
		return &NotificationConfigResponse{
			SuggestedHour:   suggestedHour,
			SuggestedMinute: suggestedMinute,
			DailyMessages:   defaultDaily,
			StreakMessages:   defaultStreak,
		}, nil
	}

	// Pad to 7 if AI returned fewer
	for len(parsed.Daily) < 7 {
		parsed.Daily = append(parsed.Daily, defaultDaily[len(parsed.Daily)%len(defaultDaily)])
	}
	for len(parsed.Streak) < 7 {
		parsed.Streak = append(parsed.Streak, defaultStreak[len(parsed.Streak)%len(defaultStreak)])
	}

	// Cache the result
	msgsJSON, _ := json.Marshal(parsed)
	s.db.Where("app_id = ? AND user_id = ? AND config_date = ?", appID, userID, today).Delete(&NotificationConfigCache{})
	s.db.Create(&NotificationConfigCache{
		ID:           uuid.New(),
		AppID:        appID,
		UserID:       userID,
		ConfigDate:   today,
		MessagesJSON: string(msgsJSON),
	})

	return &NotificationConfigResponse{
		SuggestedHour:   suggestedHour,
		SuggestedMinute: suggestedMinute,
		DailyMessages:   parsed.Daily,
		StreakMessages:   parsed.Streak,
	}, nil
}

// --- B1: Therapist Export ---

// TherapistExport returns an AI-generated, therapist-ready summary of the user's last 30 days.
// Result is cached for 6 hours per user. This is a PREMIUM feature — subscription gating
// should be enforced at the handler level once RevenueCat entitlement checks are wired up.
func (s *JournalService) TherapistExport(appID string, userID uuid.UUID) (*TherapistExportResponse, error) {
	now := time.Now().UTC()

	// Check 6-hour cache.
	var cached TherapistExportCache
	cacheErr := s.db.Where("app_id = ? AND user_id = ?", appID, userID).First(&cached).Error
	if cacheErr == nil && now.Sub(cached.GeneratedAt) < 6*time.Hour {
		var report TherapistExportResponse
		if err := json.Unmarshal([]byte(cached.ReportJSON), &report); err == nil {
			return &report, nil
		}
	}

	// Fetch last 30 days of entries.
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	var entries []JournalEntry
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND entry_date >= ?", userID, thirtyDaysAgo).
		Order("entry_date ASC").
		Find(&entries).Error; err != nil {
		return nil, err
	}

	entryCount := len(entries)
	periodLabel := thirtyDaysAgo.Format("Jan 2") + " – " + now.Format("Jan 2, 2006")

	if entryCount == 0 {
		return &TherapistExportResponse{
			Period:            periodLabel,
			EntryCount:        0,
			AvgMoodScore:      0,
			MoodTrend:         "no_data",
			DominantThemes:    []string{},
			EmotionalPatterns: "No entries in the last 30 days.",
			NotableEntries:    []NotableEntry{},
			AINarrative:       "No journal entries were found for this period.",
			Suggestions:       "Start journaling to generate a therapist-ready summary.",
			GeneratedAt:       now.Format(time.RFC3339),
		}, nil
	}

	// Compute basic stats.
	totalScore := 0
	for _, e := range entries {
		totalScore += e.MoodScore
	}
	avgScore := totalScore / entryCount

	// Mood trend: compare first half vs second half.
	moodTrend := "stable"
	if entryCount >= 2 {
		mid := entryCount / 2
		firstSum, secondSum := 0, 0
		for i := 0; i < mid; i++ {
			firstSum += entries[i].MoodScore
		}
		for i := mid; i < entryCount; i++ {
			secondSum += entries[i].MoodScore
		}
		firstAvg := firstSum / mid
		secondAvg := secondSum / (entryCount - mid)
		diff := secondAvg - firstAvg
		switch {
		case diff > 10:
			moodTrend = "improving"
		case diff > 3:
			moodTrend = "slightly_improving"
		case diff < -10:
			moodTrend = "declining"
		case diff < -3:
			moodTrend = "slightly_declining"
		}
	}

	// Select notable entries: highest and lowest mood score entries.
	notableEntries := buildNotableEntries(entries)

	// If no AI key, return a stats-only response without narrative.
	if s.aiAPIKey == "" {
		return &TherapistExportResponse{
			Period:            periodLabel,
			EntryCount:        entryCount,
			AvgMoodScore:      avgScore,
			MoodTrend:         moodTrend,
			DominantThemes:    []string{},
			EmotionalPatterns: "",
			NotableEntries:    notableEntries,
			AINarrative:       fmt.Sprintf("This month you wrote %d entries with an average mood score of %d/100.", entryCount, avgScore),
			Suggestions:       "Continue journaling to build deeper insights over time.",
			GeneratedAt:       now.Format(time.RFC3339),
		}, nil
	}

	// Build entry summaries for the AI (truncated to 500 chars per entry for security).
	var summary strings.Builder
	for _, e := range entries {
		preview := e.Content
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		summary.WriteString(fmt.Sprintf("- %s %s (score:%d): %s\n",
			e.EntryDate.Format("2006-01-02"), e.MoodEmoji, e.MoodScore, preview))
	}

	systemPrompt := `You are a clinical-grade journal analyst preparing a therapist briefing. Analyze the provided journal entries and respond with JSON only (no markdown, no code fences):
{"dominant_themes":["theme1","theme2","theme3"],"emotional_patterns":"2-3 sentences describing recurring emotional patterns","ai_narrative":"3-5 sentence warm summary the user can share with their therapist","suggestions":"1-2 concrete suggestions for the therapist to explore"}

Rules:
- dominant_themes: 2-5 most recurring topics (e.g. "work stress", "gratitude", "family", "sleep issues")
- emotional_patterns: clinical but empathetic; describe patterns by day/context if visible (e.g. "anxiety appears most on Mondays")
- ai_narrative: addressed to the user ("This month you..."); warm, non-judgmental; max 5000 chars total response
- suggestions: professional, specific, actionable for a therapist session
- Never diagnose or use clinical disorder labels`

	statsContext := fmt.Sprintf(
		"Period: %s\nEntry count: %d\nAvg mood: %d/100\nTrend: %s\n\nEntries (chronological):\n%s",
		periodLabel, entryCount, avgScore, moodTrend, summary.String(),
	)

	aiContent, err := s.callOpenAIChat(systemPrompt, statsContext)
	if err != nil {
		slog.Warn("[daiyly] therapist export AI call failed", "user", userID, "error", err)
		return &TherapistExportResponse{
			Period:            periodLabel,
			EntryCount:        entryCount,
			AvgMoodScore:      avgScore,
			MoodTrend:         moodTrend,
			DominantThemes:    []string{},
			EmotionalPatterns: "",
			NotableEntries:    notableEntries,
			AINarrative:       fmt.Sprintf("This month you wrote %d entries with an average mood score of %d/100.", entryCount, avgScore),
			Suggestions:       "Continue journaling to build deeper insights over time.",
			GeneratedAt:       now.Format(time.RFC3339),
		}, nil
	}

	// Enforce 5000-char safety cap on raw AI response before unmarshalling.
	if len(aiContent) > 5000 {
		aiContent = aiContent[:5000]
	}

	var parsed struct {
		DominantThemes    []string `json:"dominant_themes"`
		EmotionalPatterns string   `json:"emotional_patterns"`
		AINarrative       string   `json:"ai_narrative"`
		Suggestions       string   `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(aiContent), &parsed); err != nil {
		slog.Warn("[daiyly] therapist export JSON parse failed", "user", userID, "error", err)
		parsed.DominantThemes = []string{}
		parsed.AINarrative = fmt.Sprintf("This month you wrote %d entries with an average mood score of %d/100.", entryCount, avgScore)
		parsed.Suggestions = "Continue journaling to build deeper insights over time."
	}

	if parsed.DominantThemes == nil {
		parsed.DominantThemes = []string{}
	}

	report := &TherapistExportResponse{
		Period:            periodLabel,
		EntryCount:        entryCount,
		AvgMoodScore:      avgScore,
		MoodTrend:         moodTrend,
		DominantThemes:    parsed.DominantThemes,
		EmotionalPatterns: parsed.EmotionalPatterns,
		NotableEntries:    notableEntries,
		AINarrative:       parsed.AINarrative,
		Suggestions:       parsed.Suggestions,
		GeneratedAt:       now.Format(time.RFC3339),
	}

	// Persist to 6h cache (upsert: delete old + create new).
	reportJSON, _ := json.Marshal(report)
	s.db.Where("app_id = ? AND user_id = ?", appID, userID).Delete(&TherapistExportCache{})
	s.db.Create(&TherapistExportCache{
		ID:          uuid.New(),
		AppID:       appID,
		UserID:      userID,
		ReportJSON:  string(reportJSON),
		GeneratedAt: now,
	})

	return report, nil
}

// buildNotableEntries selects up to 3 notable entries from the set: the highest-scored,
// the lowest-scored, and the most recent — deduped by entry ID.
func buildNotableEntries(entries []JournalEntry) []NotableEntry {
	if len(entries) == 0 {
		return []NotableEntry{}
	}

	// Find highest and lowest by mood score.
	high, low := entries[0], entries[0]
	for _, e := range entries[1:] {
		if e.MoodScore > high.MoodScore {
			high = e
		}
		if e.MoodScore < low.MoodScore {
			low = e
		}
	}
	most := entries[len(entries)-1] // most recent

	seen := map[uuid.UUID]bool{}
	var notable []NotableEntry
	for _, e := range []JournalEntry{high, low, most} {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		excerpt := e.Content
		if len(excerpt) > 200 {
			excerpt = excerpt[:200] + "..."
		}
		notable = append(notable, NotableEntry{
			Date:      e.EntryDate.Format("2006-01-02"),
			Excerpt:   excerpt,
			MoodScore: e.MoodScore,
		})
	}
	return notable
}

// --- B3: Smart Notification Timing ---

// GetNotificationTiming analyzes the last 30 days of entries to find the user's typical
// journaling hour. Returns null optimal_hour and zero confidence when fewer than 5 entries exist.
func (s *JournalService) GetNotificationTiming(appID string, userID uuid.UUID) (*NotificationTimingResponse, error) {
	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)

	var entries []JournalEntry
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND entry_date >= ?", userID, thirtyDaysAgo).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	// Not enough data.
	if len(entries) < 5 {
		return &NotificationTimingResponse{
			OptimalHour:      nil,
			OptimalHourLabel: "",
			Confidence:       0,
			DaysAnalyzed:     len(entries),
		}, nil
	}

	// Count entries per hour and track unique journaling days.
	hourCounts := make(map[int]int)
	uniqueDays := map[string]bool{}
	for _, e := range entries {
		hourCounts[e.CreatedAt.UTC().Hour()]++
		uniqueDays[e.CreatedAt.UTC().Format("2006-01-02")] = true
	}

	// Find peak hour.
	peakHour, peakCount := 0, 0
	for h, c := range hourCounts {
		if c > peakCount || (c == peakCount && h > peakHour) {
			peakHour = h
			peakCount = c
		}
	}

	// Confidence = fraction of days that have an entry in the peak hour.
	daysAnalyzed := len(uniqueDays)
	confidence := float64(peakCount) / float64(daysAnalyzed)
	// Round to 2 decimal places.
	confidence = float64(int(confidence*100+0.5)) / 100

	// Build human-readable label (12-hour format).
	label := formatHourLabel(peakHour)

	hour := peakHour
	return &NotificationTimingResponse{
		OptimalHour:      &hour,
		OptimalHourLabel: label,
		Confidence:       confidence,
		DaysAnalyzed:     daysAnalyzed,
	}, nil
}

// TherapistReport wraps TherapistExport and returns the spec-compatible TherapistReportResponse
// shape for the GET /journals/therapist-report endpoint.
func (s *JournalService) TherapistReport(appID string, userID uuid.UUID) (*TherapistReportResponse, error) {
	now := time.Now().UTC()
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	export, err := s.TherapistExport(appID, userID)
	if err != nil {
		return nil, err
	}

	// Build a prose report from the rich export for the simpler envelope.
	var reportParts []string
	if export.AINarrative != "" {
		reportParts = append(reportParts, export.AINarrative)
	}
	if export.EmotionalPatterns != "" {
		reportParts = append(reportParts, "\nEmotional Patterns: "+export.EmotionalPatterns)
	}
	if len(export.DominantThemes) > 0 {
		reportParts = append(reportParts, "\nKey Themes: "+strings.Join(export.DominantThemes, ", ")+".")
	}
	if export.Suggestions != "" {
		reportParts = append(reportParts, "\nSuggested Discussion Points: "+export.Suggestions)
	}
	report := strings.Join(reportParts, " ")
	if report == "" {
		report = fmt.Sprintf("No journal entries found in the last 30 days (from %s to %s).",
			thirtyDaysAgo.Format("Jan 2, 2006"), now.Format("Jan 2, 2006"))
	}

	return &TherapistReportResponse{
		Report:      report,
		GeneratedAt: export.GeneratedAt,
		EntryCount:  export.EntryCount,
		DateRange: TherapistReportDateRange{
			From: thirtyDaysAgo.Format("2006-01-02"),
			To:   now.Format("2006-01-02"),
		},
	}, nil
}

// formatHourLabel converts a 0-23 hour integer to a 12-hour AM/PM string (e.g. 21 → "9:00 PM").
func formatHourLabel(hour int) string {
	suffix := "AM"
	h := hour
	if h == 0 {
		h = 12
	} else if h >= 12 {
		suffix = "PM"
		if h > 12 {
			h -= 12
		}
	}
	return fmt.Sprintf("%d:00 %s", h, suffix)
}

// --- AI Semantic Search & Ask ---

// callOpenAIDirect calls the OpenAI API (not the GLM endpoint) using the service's
// openAIAPIKey and openAIModel. Used by AISearchEntries and AskJournal.
func (s *JournalService) callOpenAIDirect(systemPrompt, userPrompt string) (string, error) {
	if s.openAIAPIKey == "" {
		return "", fmt.Errorf("openai api key not configured")
	}

	reqBody := openAIChatRequest{
		Model: s.openAIModel,
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
	req.Header.Set("Authorization", "Bearer "+s.openAIAPIKey)

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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown code fences if present.
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) > 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	return content, nil
}

// aiSearchStopWords is the minimal set of common English words skipped during keyword
// pre-filtering. Keeping this small avoids discarding domain-specific terms.
var aiSearchStopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "but": {}, "in": {},
	"on": {}, "at": {}, "to": {}, "for": {}, "of": {}, "with": {}, "by": {},
	"from": {}, "is": {}, "was": {}, "are": {}, "were": {}, "be": {}, "been": {},
	"have": {}, "has": {}, "had": {}, "do": {}, "does": {}, "did": {}, "will": {},
	"would": {}, "could": {}, "should": {}, "may": {}, "might": {}, "i": {},
	"me": {}, "my": {}, "we": {}, "our": {}, "you": {}, "your": {}, "he": {},
	"she": {}, "it": {}, "they": {}, "their": {}, "what": {}, "when": {},
	"where": {}, "who": {}, "how": {}, "why": {}, "that": {}, "this": {},
	"about": {}, "last": {}, "month": {}, "week": {}, "year": {}, "day": {},
	"time": {}, "not": {}, "no": {}, "so": {}, "up": {}, "out": {}, "if": {},
	"then": {}, "than": {}, "into": {}, "through": {}, "over": {}, "more": {},
	"just": {}, "also": {}, "some": {}, "all": {}, "any": {}, "most": {},
}

// extractKeywords splits the query into meaningful words, stripping stop words.
func extractKeywords(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	seen := map[string]struct{}{}
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:'\"()-")
		if len(w) < 2 {
			continue
		}
		if _, stop := aiSearchStopWords[w]; stop {
			continue
		}
		if _, dup := seen[w]; dup {
			continue
		}
		seen[w] = struct{}{}
		keywords = append(keywords, w)
	}
	return keywords
}

// relevanceExcerpt returns up to 200 chars of content centred on the first keyword match.
// Falls back to the first 200 chars when no keyword appears.
func relevanceExcerpt(content string, keywords []string) string {
	lower := strings.ToLower(content)
	for _, kw := range keywords {
		idx := strings.Index(lower, kw)
		if idx == -1 {
			continue
		}
		start := idx - 40
		if start < 0 {
			start = 0
		}
		end := start + 200
		if end > len(content) {
			end = len(content)
		}
		excerpt := strings.TrimSpace(content[start:end])
		if start > 0 {
			excerpt = "..." + excerpt
		}
		if end < len(content) {
			excerpt = excerpt + "..."
		}
		return excerpt
	}
	if len(content) > 200 {
		return content[:200] + "..."
	}
	return content
}

// AISearchEntries performs a semantic journal search using GPT-4o-mini.
//
// Algorithm:
//  1. Fetch entries from the last `days` days.
//  2. If ≤ 20 entries: send all to OpenAI.
//  3. If > 20: keyword pre-filter → top 30 → send to OpenAI.
//  4. OpenAI returns a JSON array of entry IDs in relevance order.
//  5. Return those entries with a relevance_excerpt.
//
// TODO: enforce per-user daily rate limit (20 req/day) at scale.
func (s *JournalService) AISearchEntries(appID string, userID uuid.UUID, query string, limit, days int) (*AISearchResponse, error) {
	if len(query) == 0 {
		return nil, fmt.Errorf("query is required")
	}
	if len(query) > 500 {
		query = query[:500]
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if days <= 0 {
		days = 90
	}
	if days > 365 {
		days = 365
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	var entries []JournalEntry
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND entry_date >= ?", userID, since).
		Order("entry_date DESC").
		Limit(500).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return &AISearchResponse{Query: query, Results: []AISearchResult{}, Total: 0}, nil
	}

	// Keyword pre-filter when there are more than 20 entries.
	candidates := entries
	if len(entries) > 20 {
		keywords := extractKeywords(query)
		if len(keywords) > 0 {
			type scoredEntry struct {
				entry JournalEntry
				hits  int
			}
			var scored []scoredEntry
			for _, e := range entries {
				lower := strings.ToLower(e.Content)
				hits := 0
				for _, kw := range keywords {
					if strings.Contains(lower, kw) {
						hits++
					}
				}
				if hits > 0 {
					scored = append(scored, scoredEntry{entry: e, hits: hits})
				}
			}
			sort.Slice(scored, func(i, j int) bool {
				return scored[i].hits > scored[j].hits
			})
			top := 30
			if len(scored) < top {
				top = len(scored)
			}
			if top == 0 {
				// No keyword hits — fall back to first 30 by date.
				if len(entries) > 30 {
					candidates = entries[:30]
				}
			} else {
				candidates = make([]JournalEntry, top)
				for i := 0; i < top; i++ {
					candidates[i] = scored[i].entry
				}
			}
		} else {
			// No usable keywords — take most recent 30.
			if len(entries) > 30 {
				candidates = entries[:30]
			}
		}
	}

	// Build a compact entry list for the AI prompt.
	var sb strings.Builder
	entryByID := make(map[string]JournalEntry, len(candidates))
	for _, e := range candidates {
		id := e.ID.String()
		entryByID[id] = e
		preview := e.Content
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("ID: %s | Date: %s | MoodScore: %d | Content: %s\n",
			id, e.EntryDate.Format("2006-01-02"), e.MoodScore, preview))
	}

	systemPrompt := fmt.Sprintf(
		`You are searching a user's private journal. Given the query and journal entries below, return the IDs of the most relevant entries (up to %d), ordered by relevance. Only return a JSON array of entry IDs, nothing else. Example: ["id1","id2","id3"]. If no entries are relevant, return an empty array: [].`,
		limit,
	)
	userPrompt := fmt.Sprintf("Query: %s\n\nEntries:\n%s", query, sb.String())

	rawContent, err := s.callOpenAIDirect(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("openai search: %w", err)
	}

	var ids []string
	if parseErr := json.Unmarshal([]byte(rawContent), &ids); parseErr != nil {
		// Do NOT log rawContent — it is a model-generated string and may contain
		// any text the model decided to produce (potentially reflecting user content).
		slog.Warn("[daiyly] ai-search: failed to parse id array from model response", "error", parseErr)
		return &AISearchResponse{Query: query, Results: []AISearchResult{}, Total: 0}, nil
	}

	keywords := extractKeywords(query)
	results := make([]AISearchResult, 0, len(ids))
	for _, id := range ids {
		e, ok := entryByID[id]
		if !ok {
			continue // AI hallucinated an ID — skip
		}
		results = append(results, AISearchResult{
			ID:               e.ID.String(),
			Date:             e.EntryDate.Format("2006-01-02"),
			Content:          e.Content,
			MoodScore:        e.MoodScore,
			RelevanceExcerpt: relevanceExcerpt(e.Content, keywords),
		})
	}

	return &AISearchResponse{
		Query:   query,
		Results: results,
		Total:   len(results),
	}, nil
}

// AskJournal answers a natural-language question about the user's journal using GPT-4o-mini.
//
// Fetches the last 90 days of entries, sends them to OpenAI, and returns the answer
// together with any referenced dates the model identifies.
//
// TODO: enforce per-user daily rate limit (20 req/day) at scale.
func (s *JournalService) AskJournal(appID string, userID uuid.UUID, question string) (*AskJournalResponse, error) {
	if len(question) == 0 {
		return nil, fmt.Errorf("question is required")
	}
	if len(question) > 1000 {
		question = question[:1000]
	}

	since := time.Now().UTC().AddDate(0, 0, -90)
	var entries []JournalEntry
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND entry_date >= ?", userID, since).
		Order("entry_date ASC").
		Limit(200).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return &AskJournalResponse{
			Answer:          "You have no journal entries in the last 90 days. Start journaling to ask questions about your entries.",
			ReferencedDates: []string{},
		}, nil
	}

	// Hard cap total characters sent to OpenAI to bound token costs.
	// 200 entries × 400 chars each = 80 000 chars ≈ 20 000 tokens at $0.15/1M input.
	// Cap at 50 000 chars (~12 500 tokens) which covers ~125 full entries.
	const askMaxChars = 50_000
	var sb strings.Builder
	for _, e := range entries {
		if sb.Len() >= askMaxChars {
			break
		}
		preview := e.Content
		if len(preview) > 400 {
			preview = preview[:400] + "..."
		}
		line := fmt.Sprintf("[%s] MoodScore:%d %s — %s\n",
			e.EntryDate.Format("2006-01-02"), e.MoodScore, e.MoodEmoji, preview)
		// Stop adding entries once the cap is reached to avoid partial mid-sentence cutoffs.
		if sb.Len()+len(line) > askMaxChars {
			break
		}
		sb.WriteString(line)
	}

	systemPrompt := `You are an AI that has read all of a user's journal entries. Answer their question about their own journal truthfully and concisely. Base your answer only on the journal entries provided. At the end of your answer, on its own line, include a JSON object with any dates you referenced: {"referenced_dates":["2026-02-15","2026-02-20"]}. If no specific dates are referenced use an empty array.`

	userPrompt := fmt.Sprintf("Journal entries (last 90 days):\n%s\n\nQuestion: %s", sb.String(), question)

	rawContent, err := s.callOpenAIDirect(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("openai ask: %w", err)
	}

	// Split answer text from the trailing JSON block.
	answer := rawContent
	var referencedDates []string

	lastBrace := strings.LastIndex(rawContent, "{")
	if lastBrace != -1 {
		jsonPart := strings.TrimSpace(rawContent[lastBrace:])
		var parsed struct {
			ReferencedDates []string `json:"referenced_dates"`
		}
		if err := json.Unmarshal([]byte(jsonPart), &parsed); err == nil {
			referencedDates = parsed.ReferencedDates
			answer = strings.TrimSpace(rawContent[:lastBrace])
		}
	}

	if referencedDates == nil {
		referencedDates = []string{}
	}

	return &AskJournalResponse{
		Answer:          answer,
		ReferencedDates: referencedDates,
	}, nil
}
