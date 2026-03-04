package moodpulse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidIntensity = errors.New("intensity must be between 1 and 10")
	ErrMissingEmotion   = errors.New("emotion is required")
	ErrNotFound         = errors.New("mood entry not found")
	ErrNotOwner         = errors.New("not the owner of this entry")
	ErrInvalidPhotoURL  = errors.New("photo_url must be an https:// URL of at most 2048 characters")
	ErrInvalidAudioURL  = errors.New("audio_url must be an https:// URL of at most 2048 characters")
)

type MoodService struct {
	db                *gorm.DB
	emotionSenseMLURL string
	aiAPIKey          string
	aiAPIURL          string
	openAIKey         string
	openAIModel       string
	aiTimeout         time.Duration
}

func NewMoodService(db *gorm.DB, cfg *config.Config) *MoodService {
	openAIModel := cfg.OpenAIModel
	if openAIModel == "" {
		openAIModel = "gpt-4o-mini"
	}
	aiTimeout := cfg.AITimeout
	if aiTimeout == 0 {
		aiTimeout = 60 * time.Second
	}
	return &MoodService{
		db:                db,
		emotionSenseMLURL: cfg.EmotionSenseMLURL,
		aiAPIKey:          cfg.GLMAPIKey,
		aiAPIURL:          cfg.GLMAPIURL,
		openAIKey:         cfg.OpenAIAPIKey,
		openAIModel:       openAIModel,
		aiTimeout:         aiTimeout,
	}
}

// isValidStorageURL accepts empty strings (field is optional) and requires an https://
// scheme with a max length of 2048 characters. Guards against mixed-content and
// excessively long URLs reaching the database.
func isValidStorageURL(url string) bool {
	if url == "" {
		return true
	}
	if len(url) > 2048 {
		return false
	}
	return strings.HasPrefix(url, "https://")
}

// --- OpenAI structs (shared within this package) ---

type moodOpenAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []moodOpenAIMessage `json:"messages"`
}

type moodOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type moodOpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// callOpenAIDirect calls the OpenAI chat completions endpoint using s.openAIKey.
// Returns an error if openAIKey is not configured.
func (s *MoodService) callOpenAIDirect(systemPrompt, userPrompt string) (string, error) {
	if s.openAIKey == "" {
		return "", fmt.Errorf("openai api key not configured")
	}

	reqBody := moodOpenAIChatRequest{
		Model: s.openAIModel,
		Messages: []moodOpenAIMessage{
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
	req.Header.Set("Authorization", "Bearer "+s.openAIKey)

	client := &http.Client{Timeout: s.aiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	var chatResp moodOpenAIChatResponse
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

// analyzeEmotionAsync calls EmotionSenseML in a goroutine after entry creation or note update.
// Updates the entry with detected_emotion, emotion_scores, and emotion_analyzed_at.
// Never blocks the main request. All failures are silently logged and discarded.
func (s *MoodService) analyzeEmotionAsync(appID, checkInID, content string) {
	if s.emotionSenseMLURL == "" || len(content) < 10 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		reqBody := map[string]string{"content": content}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return
		}
		req, err := http.NewRequestWithContext(ctx, "POST",
			s.emotionSenseMLURL+"/api/v1/analyze/text", bytes.NewReader(bodyBytes))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			slog.Warn("[moodpulse] analyzeEmotionAsync non-200", "status", resp.StatusCode)
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

		s.db.Model(&MoodCheckIn{}).
			Where("id = ? AND app_id = ?", checkInID, appID).
			Updates(map[string]interface{}{
				"detected_emotion":    emotion,
				"emotion_scores":      scoresStr,
				"emotion_analyzed_at": now,
			})
	}()
}

// AIInsights fetches the user's last N days of mood entries and asks GPT-4o-mini
// for longitudinal analysis. Returns the AI response as a plain string.
func (s *MoodService) AIInsights(appID string, userID uuid.UUID, days int) (string, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Limit(200).
		Find(&entries).Error; err != nil {
		return "", fmt.Errorf("fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Sprintf("You have no mood entries in the last %d days. Start tracking to get personalized insights.", days), nil
	}

	const maxChars = 30_000
	var sb strings.Builder
	for _, e := range entries {
		if sb.Len() >= maxChars {
			break
		}
		var triggers []TagItem
		_ = json.Unmarshal([]byte(e.TriggersJSON), &triggers)
		var activities []TagItem
		_ = json.Unmarshal([]byte(e.ActivitiesJSON), &activities)

		triggerNames := make([]string, 0, len(triggers))
		for _, t := range triggers {
			triggerNames = append(triggerNames, t.Name)
		}
		activityNames := make([]string, 0, len(activities))
		for _, a := range activities {
			activityNames = append(activityNames, a.Name)
		}

		note := e.Note
		if len(note) > 300 {
			note = note[:300] + "..."
		}

		line := fmt.Sprintf("[%s] Emotion:%s Intensity:%d/10 Triggers:[%s] Activities:[%s] Note:%s\n",
			e.CreatedAt.Format("2006-01-02"),
			e.EmotionName,
			e.Intensity,
			strings.Join(triggerNames, ","),
			strings.Join(activityNames, ","),
			note,
		)
		if sb.Len()+len(line) > maxChars {
			break
		}
		sb.WriteString(line)
	}

	systemPrompt := fmt.Sprintf(
		"You are a professional mood coach. Analyze this user's mood entries from the last %d days. "+
			"Identify patterns, recurring emotions, triggers, and activities. "+
			"Give evidence-based recommendations. Be warm but factual. "+
			"Focus on actionable insights the user can apply today.", days)

	rawContent, err := s.callOpenAIDirect(systemPrompt, sb.String())
	if err != nil {
		return "", fmt.Errorf("ai insights: %w", err)
	}

	return rawContent, nil
}

// AskMood sends the last 90 days of mood entries to GPT-4o-mini with the user's question.
// Returns the AI answer as a plain string.
func (s *MoodService) AskMood(appID string, userID uuid.UUID, question string) (string, error) {
	if len(question) == 0 {
		return "", fmt.Errorf("question is required")
	}
	if len(question) > 500 {
		question = question[:500]
	}

	since := time.Now().UTC().AddDate(0, 0, -90)
	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Limit(200).
		Find(&entries).Error; err != nil {
		return "", fmt.Errorf("fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return "You have no mood entries in the last 90 days. Start tracking your mood to ask questions about it.", nil
	}

	const askMaxChars = 50_000
	var sb strings.Builder
	for _, e := range entries {
		if sb.Len() >= askMaxChars {
			break
		}
		note := e.Note
		if len(note) > 400 {
			note = note[:400] + "..."
		}
		line := fmt.Sprintf("[%s] Emotion:%s Intensity:%d/10 — %s\n",
			e.CreatedAt.Format("2006-01-02"), e.EmotionName, e.Intensity, note)
		if sb.Len()+len(line) > askMaxChars {
			break
		}
		sb.WriteString(line)
	}

	systemPrompt := `You are an AI that has access to all of a user's mood tracking entries. Answer their question about their own mood data truthfully and concisely. Base your answer only on the mood entries provided.`
	userPrompt := fmt.Sprintf("Mood entries (last 90 days):\n%s\n\nQuestion: %s", sb.String(), question)

	rawContent, err := s.callOpenAIDirect(systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("ask mood: %w", err)
	}

	return rawContent, nil
}

func (s *MoodService) Create(appID string, userID uuid.UUID, req CreateMoodRequest) (*MoodEntryResponse, error) {
	if req.Emotion.ID == "" || req.Emotion.Name == "" {
		return nil, ErrMissingEmotion
	}
	if req.Intensity < 1 || req.Intensity > 10 {
		return nil, ErrInvalidIntensity
	}
	if len(req.Note) > 2000 {
		return nil, errors.New("note exceeds 2000 characters")
	}

	if !isValidStorageURL(req.PhotoURL) {
		return nil, ErrInvalidPhotoURL
	}
	if !isValidStorageURL(req.AudioURL) {
		return nil, ErrInvalidAudioURL
	}
	if req.Transcript != nil && len(*req.Transcript) > 50000 {
		return nil, errors.New("transcript too long (max 50000 characters)")
	}

	// Validate and cap new context/medication fields.
	validWhere := map[string]bool{"home": true, "work": true, "outside": true, "commuting": true, "social": true, "gym": true}
	validWith := map[string]bool{"alone": true, "partner": true, "friends": true, "family": true, "colleagues": true, "strangers": true}
	validActivity := map[string]bool{"working": true, "relaxing": true, "exercising": true, "eating": true, "socializing": true, "commuting": true}
	if req.WhereContext != nil && *req.WhereContext != "" && !validWhere[*req.WhereContext] {
		return nil, errors.New("invalid where_context value")
	}
	if req.WithContext != nil && *req.WithContext != "" && !validWith[*req.WithContext] {
		return nil, errors.New("invalid with_context value")
	}
	if req.ActivityContext != nil && *req.ActivityContext != "" && !validActivity[*req.ActivityContext] {
		return nil, errors.New("invalid activity_context value")
	}
	if req.SubEmotion != nil && len(*req.SubEmotion) > 50 {
		return nil, errors.New("sub_emotion too long (max 50 characters)")
	}
	if req.MedName != nil && len(*req.MedName) > 100 {
		return nil, errors.New("med_name too long (max 100 characters)")
	}

	triggersJSON, _ := json.Marshal(req.Triggers)
	activitiesJSON, _ := json.Marshal(req.Activities)

	entry := MoodCheckIn{
		AppID:           appID,
		UserID:          userID,
		EmotionID:       req.Emotion.ID,
		EmotionName:     req.Emotion.Name,
		EmotionEmoji:    req.Emotion.Emoji,
		EmotionColor:    req.Emotion.Color,
		EmotionCustom:   req.Emotion.IsCustom,
		Intensity:       req.Intensity,
		Note:            req.Note,
		TriggersJSON:    string(triggersJSON),
		ActivitiesJSON:  string(activitiesJSON),
		PhotoURL:        req.PhotoURL,
		AudioURL:        req.AudioURL,
		Transcript:      req.Transcript,
		WhereContext:    req.WhereContext,
		WithContext:     req.WithContext,
		ActivityContext: req.ActivityContext,
		SubEmotion:      req.SubEmotion,
		MedTaken:        req.MedTaken,
		MedName:         req.MedName,
	}

	if err := s.db.Create(&entry).Error; err != nil {
		return nil, fmt.Errorf("create failed: %w", err)
	}

	// Update streak
	go s.updateStreak(appID, userID)

	// Fire async emotion analysis if a note is present (non-blocking).
	s.analyzeEmotionAsync(appID, entry.ID.String(), entry.Note)

	return s.toResponse(entry), nil
}

func (s *MoodService) List(appID string, userID uuid.UUID, limit, offset, month, year int) (*MoodListResponse, error) {
	var entries []MoodCheckIn
	var total int64

	base := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)

	// Optional month/year filter for calendar efficiency
	if month >= 1 && month <= 12 && year >= 2000 {
		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, 0)
		base = base.Where("created_at >= ? AND created_at < ?", start, end)
	}

	base.Model(&MoodCheckIn{}).Count(&total)

	if err := base.Order("created_at DESC").Limit(limit).Offset(offset).Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &MoodListResponse{
		Entries: make([]MoodEntryResponse, len(entries)),
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}
	for i, e := range entries {
		resp.Entries[i] = *s.toResponse(e)
	}
	return resp, nil
}

func (s *MoodService) Calendar(appID string, userID uuid.UUID, month, year int) (*CalendarResponse, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Select("id, emotion_color, emotion_emoji, created_at").
		Order("created_at ASC").
		Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &CalendarResponse{
		Entries: make([]CalendarEntry, len(entries)),
		Month:   month,
		Year:    year,
	}
	for i, e := range entries {
		resp.Entries[i] = CalendarEntry{
			ID:    e.ID,
			Date:  e.CreatedAt.Format("2006-01-02"),
			Color: e.EmotionColor,
			Emoji: e.EmotionEmoji,
		}
	}
	return resp, nil
}

func (s *MoodService) Get(appID string, userID uuid.UUID, id uuid.UUID) (*MoodEntryResponse, error) {
	var entry MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&entry).Error; err != nil {
		return nil, ErrNotFound
	}
	if entry.UserID != userID {
		return nil, ErrNotOwner
	}
	return s.toResponse(entry), nil
}

func (s *MoodService) Update(appID string, userID uuid.UUID, id uuid.UUID, req UpdateMoodRequest) (*MoodEntryResponse, error) {
	var entry MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&entry).Error; err != nil {
		return nil, ErrNotFound
	}
	if entry.UserID != userID {
		return nil, ErrNotOwner
	}

	if req.Emotion != nil {
		if req.Emotion.ID == "" || req.Emotion.Name == "" {
			return nil, ErrMissingEmotion
		}
		entry.EmotionID = req.Emotion.ID
		entry.EmotionName = req.Emotion.Name
		entry.EmotionEmoji = req.Emotion.Emoji
		entry.EmotionColor = req.Emotion.Color
		entry.EmotionCustom = req.Emotion.IsCustom
	}
	if req.Intensity != nil {
		if *req.Intensity < 1 || *req.Intensity > 10 {
			return nil, ErrInvalidIntensity
		}
		entry.Intensity = *req.Intensity
	}
	if req.Note != nil {
		if len(*req.Note) > 2000 {
			return nil, errors.New("note exceeds 2000 characters")
		}
		entry.Note = *req.Note
	}
	if req.Triggers != nil {
		j, _ := json.Marshal(*req.Triggers)
		entry.TriggersJSON = string(j)
	}
	if req.Activities != nil {
		j, _ := json.Marshal(*req.Activities)
		entry.ActivitiesJSON = string(j)
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
		entry.Transcript = req.Transcript
	}
	if req.WhereContext != nil {
		validWhereUpd := map[string]bool{"home": true, "work": true, "outside": true, "commuting": true, "social": true, "gym": true}
		if *req.WhereContext != "" && !validWhereUpd[*req.WhereContext] {
			return nil, errors.New("invalid where_context value")
		}
		entry.WhereContext = req.WhereContext
	}
	if req.WithContext != nil {
		validWithUpd := map[string]bool{"alone": true, "partner": true, "friends": true, "family": true, "colleagues": true, "strangers": true}
		if *req.WithContext != "" && !validWithUpd[*req.WithContext] {
			return nil, errors.New("invalid with_context value")
		}
		entry.WithContext = req.WithContext
	}
	if req.ActivityContext != nil {
		validActivityUpd := map[string]bool{"working": true, "relaxing": true, "exercising": true, "eating": true, "socializing": true, "commuting": true}
		if *req.ActivityContext != "" && !validActivityUpd[*req.ActivityContext] {
			return nil, errors.New("invalid activity_context value")
		}
		entry.ActivityContext = req.ActivityContext
	}
	if req.SubEmotion != nil {
		if len(*req.SubEmotion) > 50 {
			return nil, errors.New("sub_emotion too long (max 50 characters)")
		}
		entry.SubEmotion = req.SubEmotion
	}
	if req.MedTaken != nil {
		entry.MedTaken = req.MedTaken
	}
	if req.MedName != nil {
		if len(*req.MedName) > 100 {
			return nil, errors.New("med_name too long (max 100 characters)")
		}
		entry.MedName = req.MedName
	}

	if err := s.db.Save(&entry).Error; err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	// Fire async emotion analysis if note was updated (non-blocking).
	if req.Note != nil && *req.Note != "" {
		s.analyzeEmotionAsync(appID, entry.ID.String(), entry.Note)
	}

	return s.toResponse(entry), nil
}

func (s *MoodService) Delete(appID string, userID uuid.UUID, id uuid.UUID) error {
	var entry MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id).First(&entry).Error; err != nil {
		return ErrNotFound
	}
	if entry.UserID != userID {
		return ErrNotOwner
	}
	return s.db.Delete(&entry).Error
}

func (s *MoodService) Search(appID string, userID uuid.UUID, q string) (*SearchMoodResponse, error) {
	q = strings.TrimSpace(q)
	if len(q) > 100 {
		q = q[:100]
	}
	// Escape LIKE wildcards so user-supplied % and _ are treated as literals.
	escaped := strings.ReplaceAll(q, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `\%`)
	escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	pattern := "%" + strings.ToLower(escaped) + "%"

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Where("LOWER(note) LIKE ? ESCAPE '\\' OR LOWER(emotion_name) LIKE ? ESCAPE '\\' OR LOWER(triggers_json) LIKE ? ESCAPE '\\' OR LOWER(activities_json) LIKE ? ESCAPE '\\'",
			pattern, pattern, pattern, pattern).
		Order("created_at DESC").
		Limit(50).
		Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &SearchMoodResponse{
		Entries: make([]MoodEntryResponse, len(entries)),
		Total:   int64(len(entries)),
		Query:   q,
	}
	for i, e := range entries {
		resp.Entries[i] = *s.toResponse(e)
	}
	return resp, nil
}

func (s *MoodService) GetStreak(appID string, userID uuid.UUID) (*StreakResponse, error) {
	var streak MoodStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &StreakResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	lastDate := ""
	if !streak.LastEntryDate.IsZero() {
		lastDate = streak.LastEntryDate.Format(time.RFC3339)
	}

	return &StreakResponse{
		CurrentStreak: streak.CurrentStreak,
		LongestStreak: streak.LongestStreak,
		TotalEntries:  streak.TotalEntries,
		LastEntryDate: lastDate,
	}, nil
}

func (s *MoodService) GetStats(appID string, userID uuid.UUID, days int) (*StatsResponse, error) {
	since := time.Now().AddDate(0, 0, -days)

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at DESC").
		Find(&entries).Error; err != nil {
		return nil, err
	}

	resp := &StatsResponse{
		TotalCheckIns:    len(entries),
		EmotionBreakdown: make(map[string]int),
		DayOfWeekPattern: make(map[string]int),
		TimeOfDayPattern: make(map[string]float64),
	}

	if len(entries) == 0 {
		return resp, nil
	}

	var totalIntensity float64
	emotionCounts := make(map[string]int)
	emotionEmojis := make(map[string]string)
	triggerCounts := make(map[string]int)
	timeSlots := map[string]struct{ total float64; count int }{
		"Morning":   {},
		"Afternoon": {},
		"Evening":   {},
		"Night":     {},
	}

	for _, e := range entries {
		totalIntensity += float64(e.Intensity)
		emotionCounts[e.EmotionName]++
		emotionEmojis[e.EmotionName] = e.EmotionEmoji
		resp.EmotionBreakdown[e.EmotionName]++

		day := e.CreatedAt.Weekday().String()
		resp.DayOfWeekPattern[day]++

		hour := e.CreatedAt.Hour()
		var slot string
		switch {
		case hour < 12:
			slot = "Morning"
		case hour < 17:
			slot = "Afternoon"
		case hour < 21:
			slot = "Evening"
		default:
			slot = "Night"
		}
		ts := timeSlots[slot]
		ts.total += float64(e.Intensity)
		ts.count++
		timeSlots[slot] = ts

		var triggers []TagItem
		_ = json.Unmarshal([]byte(e.TriggersJSON), &triggers)
		for _, t := range triggers {
			triggerCounts[t.Name]++
		}
	}

	resp.AverageIntensity = totalIntensity / float64(len(entries))

	// Top emotion
	maxCount := 0
	for name, count := range emotionCounts {
		if count > maxCount {
			maxCount = count
			resp.TopEmotion = name
			resp.TopEmotionEmoji = emotionEmojis[name]
		}
	}

	// Time of day averages
	for slot, data := range timeSlots {
		if data.count > 0 {
			resp.TimeOfDayPattern[slot] = data.total / float64(data.count)
		}
	}

	// Top triggers
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range triggerCounts {
		sorted = append(sorted, kv{k, v})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	limit := 5
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for _, kv := range sorted[:limit] {
		resp.TopTriggers = append(resp.TopTriggers, TriggerStat{Name: kv.k, Count: kv.v})
	}

	return resp, nil
}

// updateStreak recalculates the user's streak after a new entry.
func (s *MoodService) updateStreak(appID string, userID uuid.UUID) {
	var streak MoodStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error

	now := time.Now().Truncate(24 * time.Hour)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = MoodStreak{
			AppID:         appID,
			UserID:        userID,
			CurrentStreak: 1,
			LongestStreak: 1,
			TotalEntries:  1,
			LastEntryDate: now,
		}
		s.db.Create(&streak)
		return
	}

	streak.TotalEntries++
	lastDate := streak.LastEntryDate.Truncate(24 * time.Hour)

	switch {
	case now.Equal(lastDate):
		// Same day, just increment total
	case now.Sub(lastDate) <= 48*time.Hour && now.Sub(lastDate) > 0:
		// Next day (or within 48h window to be safe)
		streak.CurrentStreak++
	default:
		// Streak broken
		streak.CurrentStreak = 1
	}

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}
	streak.LastEntryDate = now

	s.db.Save(&streak)
}

func (s *MoodService) BatchCreate(appID string, userID uuid.UUID, req BatchCreateMoodRequest) (*BatchCreateMoodResponse, error) {
	if len(req.Entries) == 0 {
		return &BatchCreateMoodResponse{Results: []BatchMoodResult{}}, nil
	}
	if len(req.Entries) > 100 {
		return nil, errors.New("batch limit is 100 entries")
	}

	resp := &BatchCreateMoodResponse{
		Results: make([]BatchMoodResult, 0, len(req.Entries)),
	}

	for _, item := range req.Entries {
		result := BatchMoodResult{ClientID: item.ClientID}

		// Validate
		if item.Emotion.ID == "" || item.Emotion.Name == "" {
			result.Status = "error"
			result.Error = "missing emotion"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}
		if item.Intensity < 1 || item.Intensity > 10 {
			result.Status = "error"
			result.Error = "invalid intensity"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		// Parse created_at from client (fallback to now)
		createdAt := time.Now()
		if item.CreatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
				createdAt = parsed
			}
		}

		// Dedup: check if entry with same user + created_at (±1 min) + emotion already exists
		var existing MoodCheckIn
		dupStart := createdAt.Add(-1 * time.Minute)
		dupEnd := createdAt.Add(1 * time.Minute)
		err := s.db.Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND emotion_id = ? AND created_at BETWEEN ? AND ?",
				userID, item.Emotion.ID, dupStart, dupEnd).
			First(&existing).Error
		if err == nil {
			// Duplicate found
			result.Status = "duplicate"
			result.ServerID = existing.ID.String()
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		triggersJSON, _ := json.Marshal(item.Triggers)
		activitiesJSON, _ := json.Marshal(item.Activities)

		entry := MoodCheckIn{
			AppID:          appID,
			UserID:         userID,
			EmotionID:      item.Emotion.ID,
			EmotionName:    item.Emotion.Name,
			EmotionEmoji:   item.Emotion.Emoji,
			EmotionColor:   item.Emotion.Color,
			EmotionCustom:  item.Emotion.IsCustom,
			Intensity:      item.Intensity,
			Note:           item.Note,
			TriggersJSON:   string(triggersJSON),
			ActivitiesJSON: string(activitiesJSON),
			CreatedAt:      createdAt,
		}

		if err := s.db.Create(&entry).Error; err != nil {
			result.Status = "error"
			result.Error = "db create failed"
			resp.Skipped++
			resp.Results = append(resp.Results, result)
			continue
		}

		result.Status = "created"
		result.ServerID = entry.ID.String()
		resp.Imported++
		resp.Results = append(resp.Results, result)
	}

	// Update streak once at end (not per entry)
	if resp.Imported > 0 {
		go s.updateStreak(appID, userID)
	}

	return resp, nil
}

func (s *MoodService) BatchDelete(appID string, userID uuid.UUID, req BatchDeleteMoodRequest) (*BatchDeleteMoodResponse, error) {
	if len(req.IDs) == 0 {
		return &BatchDeleteMoodResponse{}, nil
	}
	if len(req.IDs) > 100 {
		return nil, errors.New("batch delete limit is 100 entries")
	}

	// Parse valid UUIDs
	validIDs := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue // Skip invalid UUIDs
		}
		validIDs = append(validIDs, id)
	}

	if len(validIDs) == 0 {
		return &BatchDeleteMoodResponse{Skipped: len(req.IDs)}, nil
	}

	result := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND id IN ?", userID, validIDs).
		Delete(&MoodCheckIn{})

	deleted := int(result.RowsAffected)
	return &BatchDeleteMoodResponse{
		Deleted: deleted,
		Skipped: len(req.IDs) - deleted,
	}, result.Error
}

// GetCBTExercise uses GPT-4o-mini to select a single evidence-based CBT/DBT technique
// for a given emotion and intensity. Returns structured JSON as a map.
func (s *MoodService) GetCBTExercise(emotion string, intensity int) (map[string]interface{}, error) {
	if emotion == "" {
		return nil, fmt.Errorf("emotion is required")
	}
	if intensity < 1 || intensity > 10 {
		return nil, fmt.Errorf("intensity must be between 1 and 10")
	}

	systemPrompt := `You are a clinical psychologist specializing in CBT and DBT.
Select the single best evidence-based technique for the user's current emotional state.
Return ONLY valid JSON with no additional text or markdown.`

	userPrompt := fmt.Sprintf(
		`User just logged emotion: %s at intensity %d/10. `+
			`Select the SINGLE best CBT or DBT technique for this moment. `+
			`Return JSON with: {"technique": "5-4-3-2-1 Grounding", "duration_seconds": 180, `+
			`"instruction": "Brief 2-sentence instruction", "steps": ["Step 1", "Step 2", "Step 3"], `+
			`"science_note": "Brief one-line evidence note"}. Only return JSON, no other text.`,
		emotion, intensity,
	)

	raw, err := s.callOpenAIDirect(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("cbt exercise: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("cbt exercise: invalid json from ai: %w", err)
	}
	return result, nil
}

// GetMoodDrivers analyzes the last N days of mood entries and returns correlations
// by trigger, activity, time of day, and day of week. Pure Go math, no AI.
func (s *MoodService) GetMoodDrivers(appID string, userID uuid.UUID, days int) (map[string]interface{}, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Limit(1000).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return map[string]interface{}{
			"triggers":      []interface{}{},
			"activities":    []interface{}{},
			"time_of_day":   map[string]interface{}{},
			"day_of_week":   map[string]interface{}{},
			"top_positive":  []interface{}{},
			"top_negative":  []interface{}{},
			"total_entries": 0,
			"days_analyzed": days,
		}, nil
	}

	// Build trigger correlation: avg intensity with vs without each trigger
	triggerWith := map[string]struct{ total float64; count int }{}
	triggerWithout := map[string]struct{ total float64; count int }{}
	allTriggerNames := map[string]bool{}

	// Build activity correlation
	activityWith := map[string]struct{ total float64; count int }{}
	activityWithout := map[string]struct{ total float64; count int }{}
	allActivityNames := map[string]bool{}

	// Time of day
	timeSlots := map[string]struct{ total float64; count int }{
		"Morning": {}, "Afternoon": {}, "Evening": {}, "Night": {},
	}

	// Day of week
	dowSlots := map[string]struct{ total float64; count int }{
		"Sunday": {}, "Monday": {}, "Tuesday": {}, "Wednesday": {},
		"Thursday": {}, "Friday": {}, "Saturday": {},
	}

	for _, e := range entries {
		intensity := float64(e.Intensity)

		var triggers []TagItem
		var activities []TagItem
		_ = json.Unmarshal([]byte(e.TriggersJSON), &triggers)
		_ = json.Unmarshal([]byte(e.ActivitiesJSON), &activities)

		triggerSet := map[string]bool{}
		for _, t := range triggers {
			allTriggerNames[t.Name] = true
			triggerSet[t.Name] = true
			ts := triggerWith[t.Name]
			ts.total += intensity
			ts.count++
			triggerWith[t.Name] = ts
		}

		activitySet := map[string]bool{}
		for _, a := range activities {
			allActivityNames[a.Name] = true
			activitySet[a.Name] = true
			as := activityWith[a.Name]
			as.total += intensity
			as.count++
			activityWith[a.Name] = as
		}

		// Without counts for triggers
		for name := range allTriggerNames {
			if !triggerSet[name] {
				tw := triggerWithout[name]
				tw.total += intensity
				tw.count++
				triggerWithout[name] = tw
			}
		}
		// Without counts for activities
		for name := range allActivityNames {
			if !activitySet[name] {
				aw := activityWithout[name]
				aw.total += intensity
				aw.count++
				activityWithout[name] = aw
			}
		}

		// Time of day
		hour := e.CreatedAt.Hour()
		var slot string
		switch {
		case hour < 12:
			slot = "Morning"
		case hour < 17:
			slot = "Afternoon"
		case hour < 21:
			slot = "Evening"
		default:
			slot = "Night"
		}
		ts := timeSlots[slot]
		ts.total += intensity
		ts.count++
		timeSlots[slot] = ts

		// Day of week
		dow := e.CreatedAt.Weekday().String()
		ds := dowSlots[dow]
		ds.total += intensity
		ds.count++
		dowSlots[dow] = ds
	}

	// Build trigger correlations
	triggerCorrs := make([]DriverCorr, 0, len(allTriggerNames))
	for name := range allTriggerNames {
		withData := triggerWith[name]
		withoutData := triggerWithout[name]
		if withData.count < 2 {
			continue // need at least 2 data points
		}
		avgWith := withData.total / float64(withData.count)
		avgWithout := 5.0 // default if no without data
		if withoutData.count > 0 {
			avgWithout = withoutData.total / float64(withoutData.count)
		}
		diff := avgWith - avgWithout
		// Round to 2 decimal places
		triggerCorrs = append(triggerCorrs, DriverCorr{
			Name:       name,
			AvgWith:    math2dp(avgWith),
			AvgWithout: math2dp(avgWithout),
			Diff:       math2dp(diff),
			CountWith:  withData.count,
		})
	}
	sortDriverCorrs(triggerCorrs)

	activityCorrs := make([]DriverCorr, 0, len(allActivityNames))
	for name := range allActivityNames {
		withData := activityWith[name]
		withoutData := activityWithout[name]
		if withData.count < 2 {
			continue
		}
		avgWith := withData.total / float64(withData.count)
		avgWithout := 5.0
		if withoutData.count > 0 {
			avgWithout = withoutData.total / float64(withoutData.count)
		}
		diff := avgWith - avgWithout
		activityCorrs = append(activityCorrs, DriverCorr{
			Name:       name,
			AvgWith:    math2dp(avgWith),
			AvgWithout: math2dp(avgWithout),
			Diff:       math2dp(diff),
			CountWith:  withData.count,
		})
	}
	sortDriverCorrs(activityCorrs)

	// Time of day averages
	timeResult := map[string]float64{}
	for slot, data := range timeSlots {
		if data.count > 0 {
			timeResult[slot] = math2dp(data.total / float64(data.count))
		}
	}

	// Day of week averages
	dowResult := map[string]float64{}
	for day, data := range dowSlots {
		if data.count > 0 {
			dowResult[day] = math2dp(data.total / float64(data.count))
		}
	}

	// Top positive and negative triggers (top 3 each)
	topPositive := make([]DriverCorr, 0, 3)
	topNegative := make([]DriverCorr, 0, 3)
	for _, c := range triggerCorrs {
		if c.Diff > 0 && len(topPositive) < 3 {
			topPositive = append(topPositive, c)
		}
		if c.Diff < 0 && len(topNegative) < 3 {
			topNegative = append(topNegative, c)
		}
	}

	return map[string]interface{}{
		"triggers":      triggerCorrs,
		"activities":    activityCorrs,
		"time_of_day":   timeResult,
		"day_of_week":   dowResult,
		"top_positive":  topPositive,
		"top_negative":  topNegative,
		"total_entries": len(entries),
		"days_analyzed": days,
	}, nil
}

// math2dp rounds a float64 to 2 decimal places.
func math2dp(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// DriverCorr holds mood correlation data for a single trigger or activity.
type DriverCorr struct {
	Name       string  `json:"name"`
	AvgWith    float64 `json:"avg_with"`
	AvgWithout float64 `json:"avg_without"`
	Diff       float64 `json:"diff"`
	CountWith  int     `json:"count_with"`
}

// sortDriverCorrs sorts driver correlations by absolute diff descending.
func sortDriverCorrs(corrs []DriverCorr) {
	for i := 0; i < len(corrs); i++ {
		for j := i + 1; j < len(corrs); j++ {
			absi := corrs[i].Diff
			if absi < 0 {
				absi = -absi
			}
			absj := corrs[j].Diff
			if absj < 0 {
				absj = -absj
			}
			if absj > absi {
				corrs[i], corrs[j] = corrs[j], corrs[i]
			}
		}
	}
}

// GetMoodForecast pattern-matches the last 12 weeks of data (grouped by day of week and
// time of day) and returns next 7 days of predicted moods based on historical averages.
func (s *MoodService) GetMoodForecast(appID string, userID uuid.UUID) (map[string]interface{}, error) {
	since := time.Now().UTC().AddDate(0, 0, -84) // 12 weeks

	var entries []MoodCheckIn
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at ASC").
		Limit(1000).
		Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("fetch entries: %w", err)
	}

	// Group by day of week: 0=Sunday, 1=Monday, ... 6=Saturday
	dowStats := [7]struct {
		total       float64
		count       int
		emotionVote map[string]int
	}{}
	for i := range dowStats {
		dowStats[i].emotionVote = map[string]int{}
	}

	for _, e := range entries {
		dow := int(e.CreatedAt.Weekday())
		dowStats[dow].total += float64(e.Intensity)
		dowStats[dow].count++
		dowStats[dow].emotionVote[e.EmotionName]++
	}

	// Build confidence based on data count
	confidenceLabel := func(count int) string {
		switch {
		case count >= 8:
			return "high"
		case count >= 4:
			return "moderate"
		case count >= 1:
			return "low"
		default:
			return "none"
		}
	}

	// Day name → note template
	dayNotes := [7]string{
		"You typically log on Sundays around this intensity.",
		"Mondays tend to bring this mood pattern for you.",
		"Tuesdays typically look like this for you.",
		"You tend to feel this way on Wednesdays.",
		"Thursdays usually look like this in your data.",
		"Your Fridays tend toward this mood level.",
		"Saturdays typically show this pattern for you.",
	}

	type ForecastDay struct {
		Date               string  `json:"date"`
		DayOfWeek          string  `json:"day_of_week"`
		PredictedEmotion   string  `json:"predicted_emotion"`
		PredictedIntensity float64 `json:"predicted_intensity"`
		Confidence         string  `json:"confidence"`
		Note               string  `json:"note"`
	}

	forecast := make([]ForecastDay, 7)
	now := time.Now().UTC()
	dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

	for i := 0; i < 7; i++ {
		target := now.AddDate(0, 0, i+1)
		dow := int(target.Weekday())
		stats := dowStats[dow]

		day := ForecastDay{
			Date:      target.Format("2006-01-02"),
			DayOfWeek: dayNames[dow],
		}

		if stats.count == 0 {
			day.PredictedEmotion = "unknown"
			day.PredictedIntensity = 5.0
			day.Confidence = "none"
			day.Note = "Not enough data for this day yet."
		} else {
			avgIntensity := math2dp(stats.total / float64(stats.count))
			day.PredictedIntensity = avgIntensity
			day.Confidence = confidenceLabel(stats.count)

			// Top emotion vote
			topEmotion := "calm"
			topCount := 0
			for emotion, cnt := range stats.emotionVote {
				if cnt > topCount {
					topCount = cnt
					topEmotion = emotion
				}
			}
			day.PredictedEmotion = topEmotion
			day.Note = dayNotes[dow]
		}

		forecast[i] = day
	}

	return map[string]interface{}{
		"forecast":      forecast,
		"weeks_analyzed": 12,
		"total_entries": len(entries),
	}, nil
}

func (s *MoodService) toResponse(e MoodCheckIn) *MoodEntryResponse {
	var triggers []TagItem
	var activities []TagItem
	_ = json.Unmarshal([]byte(e.TriggersJSON), &triggers)
	_ = json.Unmarshal([]byte(e.ActivitiesJSON), &activities)

	if triggers == nil {
		triggers = []TagItem{}
	}
	if activities == nil {
		activities = []TagItem{}
	}

	return &MoodEntryResponse{
		ID: e.ID,
		Emotion: EmotionDTO{
			ID:       e.EmotionID,
			Name:     e.EmotionName,
			Emoji:    e.EmotionEmoji,
			Color:    e.EmotionColor,
			IsCustom: e.EmotionCustom,
		},
		Intensity:  e.Intensity,
		Note:       e.Note,
		Triggers:   triggers,
		Activities: activities,
		CreatedAt:  e.CreatedAt.Format(time.RFC3339),
	}
}

// VocabularyService handles custom vocabulary CRUD.
type VocabularyService struct {
	db *gorm.DB
}

func NewVocabularyService(db *gorm.DB) *VocabularyService {
	return &VocabularyService{db: db}
}

// --- Emotions ---

func (s *VocabularyService) ListEmotions(appID string, userID uuid.UUID) ([]CustomEmotion, error) {
	var items []CustomEmotion
	if err := s.db.Where("app_id = ? AND user_id = ?", appID, userID).Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *VocabularyService) UpsertEmotion(appID string, userID uuid.UUID, req CreateCustomEmotionRequest) (*CustomEmotion, error) {
	if req.Name == "" || req.Emoji == "" || req.Color == "" {
		return nil, errors.New("name, emoji, and color are required")
	}
	if len(req.Name) > 50 {
		return nil, errors.New("name must be at most 50 characters")
	}
	if len(req.Emoji) > 10 {
		return nil, errors.New("emoji must be at most 10 characters")
	}
	if len(req.Color) > 20 {
		return nil, errors.New("color must be at most 20 characters")
	}
	item := CustomEmotion{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Emoji:  req.Emoji,
		Color:  req.Color,
	}
	result := s.db.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, req.Name).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else {
		item.Emoji = req.Emoji
		item.Color = req.Color
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	}
	return &item, nil
}

func (s *VocabularyService) DeleteEmotion(appID string, userID uuid.UUID, id uuid.UUID) error {
	result := s.db.Where("app_id = ? AND user_id = ? AND id = ?", appID, userID, id).Delete(&CustomEmotion{})
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// --- Triggers ---

func (s *VocabularyService) ListTriggers(appID string, userID uuid.UUID) ([]CustomTrigger, error) {
	var items []CustomTrigger
	if err := s.db.Where("app_id = ? AND user_id = ?", appID, userID).Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *VocabularyService) UpsertTrigger(appID string, userID uuid.UUID, req CreateCustomTriggerRequest) (*CustomTrigger, error) {
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if len(req.Name) > 50 {
		return nil, errors.New("name must be at most 50 characters")
	}
	if req.Icon == "" {
		req.Icon = "flash-outline"
	}
	if len(req.Icon) > 50 {
		return nil, errors.New("icon must be at most 50 characters")
	}
	item := CustomTrigger{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Icon:   req.Icon,
	}
	result := s.db.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, req.Name).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else {
		item.Icon = req.Icon
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	}
	return &item, nil
}

func (s *VocabularyService) DeleteTrigger(appID string, userID uuid.UUID, id uuid.UUID) error {
	result := s.db.Where("app_id = ? AND user_id = ? AND id = ?", appID, userID, id).Delete(&CustomTrigger{})
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// --- Activities ---

func (s *VocabularyService) ListActivities(appID string, userID uuid.UUID) ([]CustomActivity, error) {
	var items []CustomActivity
	if err := s.db.Where("app_id = ? AND user_id = ?", appID, userID).Order("created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *VocabularyService) UpsertActivity(appID string, userID uuid.UUID, req CreateCustomActivityRequest) (*CustomActivity, error) {
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if len(req.Name) > 50 {
		return nil, errors.New("name must be at most 50 characters")
	}
	if req.Icon == "" {
		req.Icon = "ellipse-outline"
	}
	if len(req.Icon) > 50 {
		return nil, errors.New("icon must be at most 50 characters")
	}
	item := CustomActivity{
		AppID:  appID,
		UserID: userID,
		Name:   req.Name,
		Icon:   req.Icon,
	}
	result := s.db.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, req.Name).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, result.Error
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if err := s.db.Create(&item).Error; err != nil {
			return nil, err
		}
	} else {
		item.Icon = req.Icon
		if err := s.db.Save(&item).Error; err != nil {
			return nil, err
		}
	}
	return &item, nil
}

func (s *VocabularyService) DeleteActivity(appID string, userID uuid.UUID, id uuid.UUID) error {
	result := s.db.Where("app_id = ? AND user_id = ? AND id = ?", appID, userID, id).Delete(&CustomActivity{})
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return result.Error
}

// --- Bulk Sync ---

func (s *VocabularyService) BulkSync(appID string, userID uuid.UUID, req BulkSyncVocabularyRequest) (*BulkSyncVocabularyResponse, error) {
	// Limit total items to prevent storage abuse
	total := len(req.Emotions) + len(req.Triggers) + len(req.Activities)
	if total > 300 {
		return nil, errors.New("bulk sync limit is 300 items total")
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Upsert emotions — check each sub-operation error and rollback on failure
	for _, e := range req.Emotions {
		if e.Name == "" || e.Emoji == "" || e.Color == "" {
			continue
		}
		var existing CustomEmotion
		err := tx.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, e.Name).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&CustomEmotion{AppID: appID, UserID: userID, Name: e.Name, Emoji: e.Emoji, Color: e.Color}).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("bulk sync failed: %w", err)
			}
		} else if err == nil {
			existing.Emoji = e.Emoji
			existing.Color = e.Color
			if err := tx.Save(&existing).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("bulk sync failed: %w", err)
			}
		}
	}

	// Upsert triggers
	for _, t := range req.Triggers {
		if t.Name == "" {
			continue
		}
		icon := t.Icon
		if icon == "" {
			icon = "flash-outline"
		}
		var existing CustomTrigger
		err := tx.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, t.Name).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&CustomTrigger{AppID: appID, UserID: userID, Name: t.Name, Icon: icon}).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("bulk sync failed: %w", err)
			}
		} else if err == nil {
			existing.Icon = icon
			if err := tx.Save(&existing).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("bulk sync failed: %w", err)
			}
		}
	}

	// Upsert activities
	for _, a := range req.Activities {
		if a.Name == "" {
			continue
		}
		icon := a.Icon
		if icon == "" {
			icon = "ellipse-outline"
		}
		var existing CustomActivity
		err := tx.Where("app_id = ? AND user_id = ? AND name = ?", appID, userID, a.Name).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&CustomActivity{AppID: appID, UserID: userID, Name: a.Name, Icon: icon}).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("bulk sync failed: %w", err)
			}
		} else if err == nil {
			existing.Icon = icon
			if err := tx.Save(&existing).Error; err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("bulk sync failed: %w", err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("bulk sync commit failed: %w", err)
	}

	// Fetch all after sync
	emotions, _ := s.ListEmotions(appID, userID)
	triggers, _ := s.ListTriggers(appID, userID)
	activities, _ := s.ListActivities(appID, userID)

	return &BulkSyncVocabularyResponse{
		Emotions:   emotions,
		Triggers:   triggers,
		Activities: activities,
	}, nil
}

// GetContextInsights returns average mood intensity grouped by context category
// (where, with_whom, activity). Pure SQL aggregation, no AI.
func (s *MoodService) GetContextInsights(appID string, userID uuid.UUID, days int) (*ContextInsightsResponse, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)

	// allowedContextColumns is the complete set of column names that buildMap may
	// query. The col string is interpolated directly into raw SQL, so we must
	// validate it against this allowlist before use to prevent SQL injection.
	allowedContextColumns := map[string]bool{
		"where_context":    true,
		"with_context":     true,
		"activity_context": true,
	}

	roundAvg := func(v float64) float64 {
		return float64(int(v*100+0.5)) / 100
	}

	buildMap := func(col string) (map[string]float64, error) {
		if !allowedContextColumns[col] {
			return nil, fmt.Errorf("invalid context column: %s", col)
		}
		var rows []struct {
			Context string  `gorm:"column:ctx"`
			Avg     float64 `gorm:"column:avg_intensity"`
		}
		err := s.db.Raw(
			"SELECT "+col+" AS ctx, ROUND(AVG(intensity)::numeric, 2) AS avg_intensity "+
				"FROM mood_check_ins "+
				"WHERE app_id = ? AND user_id = ? AND created_at >= ? AND "+col+" IS NOT NULL AND "+col+" != '' "+
				"GROUP BY "+col,
			appID, userID, since,
		).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		m := make(map[string]float64, len(rows))
		for _, r := range rows {
			m[r.Context] = roundAvg(r.Avg)
		}
		return m, nil
	}

	whereMap, err := buildMap("where_context")
	if err != nil {
		return nil, fmt.Errorf("where context: %w", err)
	}
	withMap, err := buildMap("with_context")
	if err != nil {
		return nil, fmt.Errorf("with context: %w", err)
	}
	activityMap, err := buildMap("activity_context")
	if err != nil {
		return nil, fmt.Errorf("activity context: %w", err)
	}

	return &ContextInsightsResponse{
		Where:    whereMap,
		With:     withMap,
		Activity: activityMap,
		Days:     days,
	}, nil
}

// GetMedCorrelation returns average mood intensity on days where the user
// took their medication vs days they did not (last N days).
func (s *MoodService) GetMedCorrelation(appID string, userID uuid.UUID, medName string, days int) (*MedCorrelationResponse, error) {
	since := time.Now().UTC().AddDate(0, 0, -days)

	type aggRow struct {
		MedTaken bool    `gorm:"column:med_taken"`
		AvgInt   float64 `gorm:"column:avg_intensity"`
		Cnt      int     `gorm:"column:cnt"`
	}
	var rows []aggRow
	err := s.db.Raw(
		"SELECT med_taken, ROUND(AVG(intensity)::numeric, 2) AS avg_intensity, COUNT(*) AS cnt "+
			"FROM mood_check_ins "+
			"WHERE app_id = ? AND user_id = ? AND created_at >= ? AND med_name = ? AND med_taken IS NOT NULL "+
			"GROUP BY med_taken",
		appID, userID, since, medName,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("med correlation: %w", err)
	}

	resp := &MedCorrelationResponse{
		MedName: medName,
		Days:    days,
	}
	for _, r := range rows {
		if r.MedTaken {
			resp.TakenAvg = r.AvgInt
			resp.TakenCount = r.Cnt
		} else {
			resp.NotTakenAvg = r.AvgInt
			resp.NotTakenCount = r.Cnt
		}
	}
	return resp, nil
}

// SubEmotionVocabulary is the static sub-emotion map returned by GetSubEmotions.
var SubEmotionVocabulary = map[string][]string{
	"joy":     {"elated", "content", "grateful", "excited", "proud", "optimistic"},
	"calm":    {"peaceful", "relaxed", "centered", "balanced", "grounded", "serene"},
	"sad":     {"hopeless", "disappointed", "lonely", "melancholy", "grief", "numb"},
	"angry":   {"frustrated", "irritated", "resentful", "furious", "enraged", "bitter"},
	"anxious": {"overwhelmed", "nervous", "worried", "panicked", "restless", "dread"},
	"tired":   {"exhausted", "drained", "sluggish", "burned_out", "fatigued", "sleepy"},
	"love":    {"affectionate", "connected", "warmth", "adored", "cherished", "devoted"},
	"energy":  {"motivated", "inspired", "focused", "alive", "enthusiastic", "driven"},
}

// GetSubEmotions returns the static sub-emotion vocabulary. No DB access needed.
func (s *MoodService) GetSubEmotions() *SubEmotionsResponse {
	return &SubEmotionsResponse{SubEmotions: SubEmotionVocabulary}
}

// GetCrisisCheck queries the last 7 days of check-ins for the user and determines
// whether they are in a crisis pattern (5+ consecutive low-mood days).
// "Low" is defined as a daily average intensity <= 3.0 on the 1-10 scale.
func (s *MoodService) GetCrisisCheck(appID string, userID uuid.UUID) (*CrisisCheckResponse, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -7)

	type DayRow struct {
		Day      string  `gorm:"column:day"`
		AvgInt   float64 `gorm:"column:avg_int"`
		EntryCount int   `gorm:"column:entry_count"`
	}

	var rows []DayRow
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Model(&MoodCheckIn{}).
		Select("DATE(created_at AT TIME ZONE 'UTC') AS day, AVG(intensity) AS avg_int, COUNT(*) AS entry_count").
		Where("user_id = ? AND created_at >= ? AND deleted_at IS NULL", userID, cutoff).
		Group("DATE(created_at AT TIME ZONE 'UTC')").
		Order("day DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	// Build day -> avg map for quick lookup.
	dayAvg := make(map[string]float64, len(rows))
	totalEntries := 0
	var sumIntensity float64
	for _, r := range rows {
		dayAvg[r.Day] = r.AvgInt
		totalEntries += r.EntryCount
		sumIntensity += r.AvgInt * float64(r.EntryCount)
	}

	avgLast7 := 0.0
	if totalEntries > 0 {
		avgLast7 = sumIntensity / float64(totalEntries)
	}

	// Count consecutive low days ending at today or yesterday (UTC).
	today := time.Now().UTC().Truncate(24 * time.Hour)
	consecutiveLow := 0
	for i := 0; i < 7; i++ {
		day := today.AddDate(0, 0, -i).Format("2006-01-02")
		avg, exists := dayAvg[day]
		if !exists {
			// No entry for that day — streak is broken.
			break
		}
		if avg <= 3.0 {
			consecutiveLow++
		} else {
			break
		}
	}

	recommendation := "Keep checking in — you're doing the right thing."
	if consecutiveLow >= 7 {
		recommendation = "We recommend speaking with a mental health professional."
	} else if consecutiveLow >= 5 {
		recommendation = "Consider reaching out to a trusted person today."
	}

	return &CrisisCheckResponse{
		InCrisis:           consecutiveLow >= 5,
		ConsecutiveLowDays: consecutiveLow,
		AvgIntensityLast7:  avgLast7,
		TotalEntriesLast7:  totalEntries,
		Recommendation:     recommendation,
	}, nil
}
