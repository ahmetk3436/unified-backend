package daiyly

import (
	"errors"
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
}

func NewJournalService(db *gorm.DB) *JournalService {
	return &JournalService{db: db}
}

func NewJournalServiceWithFilter(db *gorm.DB, filter *ContentFilterService) *JournalService {
	return &JournalService{db: db, contentFilter: filter}
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

	_ = s.UpdateStreak(appID, userID)

	return &entry, nil
}

func (s *JournalService) GetEntries(appID string, userID uuid.UUID, limit, offset int) ([]JournalEntry, int64, error) {
	var entries []JournalEntry
	var total int64

	s.db.Model(&JournalEntry{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&total)

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
