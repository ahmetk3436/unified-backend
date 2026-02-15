package feelsy

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/services"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FeelService handles mood check-in business logic.
type FeelService struct {
	db                *gorm.DB
	moderationService *services.ModerationService
}

// NewFeelService creates a new FeelService.
func NewFeelService(db *gorm.DB, moderationService *services.ModerationService) *FeelService {
	return &FeelService{db: db, moderationService: moderationService}
}

// CreateFeelCheck creates a new daily mood check-in.
func (s *FeelService) CreateFeelCheck(appID string, userID uuid.UUID, moodScore, energyScore int, moodEmoji, note, journalEntry string) (*FeelCheck, error) {
	if moodScore < 1 || moodScore > 100 || energyScore < 1 || energyScore > 100 {
		return nil, errors.New("scores must be between 1 and 100")
	}

	today := time.Now().Truncate(24 * time.Hour)

	var existing FeelCheck
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND check_date = ?", userID, today).First(&existing).Error; err == nil {
		return nil, errors.New("already checked in today")
	}

	// Filter note for prohibited content
	if s.moderationService != nil && note != "" {
		if isClean, _ := s.moderationService.FilterContent(note); !isClean {
			note = "[content filtered]"
		}
	}

	// Filter journal entry for prohibited content
	if s.moderationService != nil && journalEntry != "" {
		if isClean, _ := s.moderationService.FilterContent(journalEntry); !isClean {
			journalEntry = "[content filtered]"
		}
	}

	check := &FeelCheck{
		AppID:        appID,
		UserID:       userID,
		MoodScore:    moodScore,
		EnergyScore:  energyScore,
		MoodEmoji:    moodEmoji,
		Note:         note,
		JournalEntry: journalEntry,
		CheckDate:    today,
	}
	check.CalculateFeelScore()
	check.ColorHex = check.GetColorHex()

	if err := s.db.Create(check).Error; err != nil {
		return nil, err
	}

	go s.UpdateStreak(appID, userID)

	return check, nil
}

// UpdateJournalEntry updates the journal entry for a specific check-in.
func (s *FeelService) UpdateJournalEntry(appID string, userID, checkID uuid.UUID, journalEntry string) (*FeelCheck, error) {
	var check FeelCheck
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ? AND user_id = ?", checkID, userID).First(&check).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("check-in not found")
		}
		return nil, err
	}

	if s.moderationService != nil && journalEntry != "" {
		if isClean, _ := s.moderationService.FilterContent(journalEntry); !isClean {
			journalEntry = "[content filtered]"
		}
	}

	check.JournalEntry = journalEntry
	if err := s.db.Save(&check).Error; err != nil {
		return nil, err
	}

	return &check, nil
}

// GetTodayCheck returns today's check-in for a user.
func (s *FeelService) GetTodayCheck(appID string, userID uuid.UUID) (*FeelCheck, error) {
	today := time.Now().Truncate(24 * time.Hour)
	var check FeelCheck
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND check_date = ?", userID, today).First(&check).Error; err != nil {
		return nil, err
	}
	return &check, nil
}

// GetFeelHistory returns check-in history for a user.
func (s *FeelService) GetFeelHistory(appID string, userID uuid.UUID, limit, offset int) ([]FeelCheck, int64, error) {
	var checks []FeelCheck
	var total int64

	s.db.Model(&FeelCheck{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).Count(&total)

	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).
		Order("check_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&checks).Error

	return checks, total, err
}

// GetFeelStats returns statistics for a user.
func (s *FeelService) GetFeelStats(appID string, userID uuid.UUID) (map[string]interface{}, error) {
	var streak FeelStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]interface{}{
				"current_streak":  0,
				"longest_streak":  0,
				"total_check_ins": 0,
				"average_score":   0,
				"unlocked_badges": []string{},
			}, nil
		}
		return nil, err
	}

	var badgeList []string
	if streak.UnlockedBadges != "" {
		badgeList = strings.Split(streak.UnlockedBadges, ",")
	} else {
		badgeList = []string{}
	}

	return map[string]interface{}{
		"current_streak":  streak.CurrentStreak,
		"longest_streak":  streak.LongestStreak,
		"total_check_ins": streak.TotalCheckIns,
		"average_score":   streak.AverageScore,
		"unlocked_badges": badgeList,
	}, nil
}

// UpdateStreak updates the user's streak after a check-in.
func (s *FeelService) UpdateStreak(appID string, userID uuid.UUID) error {
	today := time.Now().Truncate(24 * time.Hour)

	var streak FeelStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = FeelStreak{
			AppID:          appID,
			UserID:         userID,
			CurrentStreak:  1,
			LongestStreak:  1,
			TotalCheckIns:  1,
			LastCheckDate:  &today,
			UnlockedBadges: "",
		}
		return s.db.Create(&streak).Error
	}

	if streak.LastCheckDate != nil {
		daysSince := int(today.Sub(*streak.LastCheckDate).Hours() / 24)
		if daysSince == 1 {
			streak.CurrentStreak++
		} else if daysSince > 1 {
			streak.CurrentStreak = 1
		}
	} else {
		streak.CurrentStreak = 1
	}

	streak.TotalCheckIns++
	streak.LastCheckDate = &today

	if streak.CurrentStreak > streak.LongestStreak {
		streak.LongestStreak = streak.CurrentStreak
	}

	var avgScore float64
	s.db.Model(&FeelCheck{}).Scopes(tenant.ForTenant(appID)).
		Where("user_id = ?", userID).
		Select("AVG(feel_score)").
		Scan(&avgScore)
	streak.AverageScore = avgScore

	streak.UnlockedBadges = checkBadgeUnlocks(streak.CurrentStreak, streak.TotalCheckIns, streak.UnlockedBadges)

	return s.db.Save(&streak).Error
}

func checkBadgeUnlocks(streak, total int, current string) string {
	badges := make(map[string]bool)
	if current != "" {
		for _, b := range strings.Split(current, ",") {
			badges[b] = true
		}
	}

	if streak >= 3 {
		badges["streak_3"] = true
	}
	if streak >= 7 {
		badges["streak_7"] = true
	}
	if streak >= 14 {
		badges["streak_14"] = true
	}
	if streak >= 30 {
		badges["streak_30"] = true
	}
	if total >= 10 {
		badges["total_10"] = true
	}
	if total >= 50 {
		badges["total_50"] = true
	}
	if total >= 100 {
		badges["total_100"] = true
	}

	result := make([]string, 0, len(badges))
	for badge := range badges {
		result = append(result, badge)
	}
	return strings.Join(result, ",")
}

// SendGoodVibe sends positive energy to a friend.
func (s *FeelService) SendGoodVibe(appID string, senderID, receiverID uuid.UUID, message, vibeType string) (*GoodVibe, error) {
	if senderID == receiverID {
		return nil, errors.New("cannot send vibe to yourself")
	}

	vibe := &GoodVibe{
		AppID:      appID,
		SenderID:   senderID,
		ReceiverID: receiverID,
		Message:    message,
		VibeType:   vibeType,
	}

	if err := s.db.Create(vibe).Error; err != nil {
		return nil, err
	}

	return vibe, nil
}

// GetReceivedVibes returns vibes received by a user.
func (s *FeelService) GetReceivedVibes(appID string, userID uuid.UUID, limit int) ([]GoodVibe, error) {
	var vibes []GoodVibe
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("receiver_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&vibes).Error
	return vibes, err
}

// GetFriendFeels returns today's feels for user's friends.
func (s *FeelService) GetFriendFeels(appID string, userID uuid.UUID) ([]map[string]interface{}, error) {
	today := time.Now().Truncate(24 * time.Hour)

	var friends []FeelFriend
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("(user_id = ? OR friend_id = ?) AND status = ?", userID, userID, "accepted").
		Find(&friends).Error
	if err != nil {
		return nil, err
	}

	friendIDs := make([]uuid.UUID, 0)
	for _, f := range friends {
		if f.UserID == userID {
			friendIDs = append(friendIDs, f.FriendID)
		} else {
			friendIDs = append(friendIDs, f.UserID)
		}
	}

	if len(friendIDs) == 0 {
		return []map[string]interface{}{}, nil
	}

	var checks []FeelCheck
	err = s.db.Scopes(tenant.ForTenant(appID)).Where("user_id IN ? AND check_date = ?", friendIDs, today).
		Find(&checks).Error
	if err != nil {
		return nil, err
	}

	var users []models.User
	s.db.Where("id IN ?", friendIDs).Find(&users)
	userMap := make(map[uuid.UUID]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	result := make([]map[string]interface{}, 0)
	for _, check := range checks {
		user := userMap[check.UserID]
		result = append(result, map[string]interface{}{
			"user_id":    check.UserID,
			"name":       user.Email,
			"feel_score": check.FeelScore,
			"mood_emoji": check.MoodEmoji,
			"color_hex":  check.ColorHex,
			"check_date": check.CheckDate,
		})
	}

	return result, nil
}

// SendFriendRequest sends a friend request to a user by email.
func (s *FeelService) SendFriendRequest(appID string, userID uuid.UUID, friendEmail string) (*FeelFriend, error) {
	var friend models.User
	if err := s.db.Where("email = ?", friendEmail).First(&friend).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found with that email")
		}
		return nil, err
	}

	if friend.ID == userID {
		return nil, errors.New("cannot send friend request to yourself")
	}

	var existing FeelFriend
	err := s.db.Scopes(tenant.ForTenant(appID)).Where(
		"((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?))",
		userID, friend.ID, friend.ID, userID,
	).First(&existing).Error
	if err == nil {
		switch existing.Status {
		case "pending":
			return nil, errors.New("friend request already pending")
		case "accepted":
			return nil, errors.New("already friends")
		case "blocked":
			return nil, errors.New("cannot send friend request")
		}
	}

	request := &FeelFriend{
		AppID:    appID,
		UserID:   userID,
		FriendID: friend.ID,
		Status:   "pending",
	}
	if err := s.db.Create(request).Error; err != nil {
		return nil, err
	}

	return request, nil
}

// AcceptFriendRequest accepts a pending friend request.
func (s *FeelService) AcceptFriendRequest(appID string, userID, requestID uuid.UUID) error {
	var request FeelFriend
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&request, "id = ?", requestID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("friend request not found")
		}
		return err
	}

	if request.FriendID != userID {
		return errors.New("not authorized to accept this request")
	}
	if request.Status != "pending" {
		return errors.New("request is not pending")
	}

	request.Status = "accepted"
	return s.db.Save(&request).Error
}

// RejectFriendRequest rejects and deletes a pending friend request.
func (s *FeelService) RejectFriendRequest(appID string, userID, requestID uuid.UUID) error {
	var request FeelFriend
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&request, "id = ?", requestID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("friend request not found")
		}
		return err
	}

	if request.FriendID != userID {
		return errors.New("not authorized to reject this request")
	}
	if request.Status != "pending" {
		return errors.New("request is not pending")
	}

	return s.db.Delete(&request).Error
}

// ListFriendRequests returns pending friend requests received by a user.
func (s *FeelService) ListFriendRequests(appID string, userID uuid.UUID) ([]FeelFriend, error) {
	var requests []FeelFriend
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("friend_id = ? AND status = ?", userID, "pending").
		Order("created_at DESC").
		Find(&requests).Error
	return requests, err
}

// ListFriends returns all accepted friends for a user.
func (s *FeelService) ListFriends(appID string, userID uuid.UUID) ([]map[string]interface{}, error) {
	var friendships []FeelFriend
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("(user_id = ? OR friend_id = ?) AND status = ?", userID, userID, "accepted").
		Find(&friendships).Error
	if err != nil {
		return nil, err
	}

	if len(friendships) == 0 {
		return []map[string]interface{}{}, nil
	}

	friendIDs := make([]uuid.UUID, 0, len(friendships))
	friendshipMap := make(map[uuid.UUID]uuid.UUID)
	for _, f := range friendships {
		if f.UserID == userID {
			friendIDs = append(friendIDs, f.FriendID)
			friendshipMap[f.FriendID] = f.ID
		} else {
			friendIDs = append(friendIDs, f.UserID)
			friendshipMap[f.UserID] = f.ID
		}
	}

	var users []models.User
	s.db.Where("id IN ?", friendIDs).Find(&users)

	result := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		result = append(result, map[string]interface{}{
			"id":           friendshipMap[u.ID].String(),
			"friend_id":    u.ID.String(),
			"friend_email": u.Email,
			"status":       "accepted",
		})
	}

	return result, nil
}

// RemoveFriend removes a friend connection.
func (s *FeelService) RemoveFriend(appID string, userID, friendshipID uuid.UUID) error {
	var friendship FeelFriend
	if err := s.db.Scopes(tenant.ForTenant(appID)).First(&friendship, "id = ?", friendshipID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("friendship not found")
		}
		return err
	}

	if friendship.UserID != userID && friendship.FriendID != userID {
		return errors.New("not authorized to remove this friendship")
	}

	return s.db.Delete(&friendship).Error
}

// GetWeeklyInsights returns mood trend analysis comparing current and previous week.
func (s *FeelService) GetWeeklyInsights(appID string, userID uuid.UUID) (*InsightsResponse, error) {
	now := time.Now()

	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	daysSinceMonday := int(weekday) - 1
	currentWeekStart := now.AddDate(0, 0, -daysSinceMonday).Truncate(24 * time.Hour)
	currentWeekEnd := now.Truncate(24 * time.Hour)

	previousWeekStart := currentWeekStart.AddDate(0, 0, -7)
	previousWeekEnd := currentWeekStart.AddDate(0, 0, -1)

	var currentChecks []FeelCheck
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND check_date >= ? AND check_date <= ?", userID, currentWeekStart, currentWeekEnd).
		Order("check_date ASC").
		Find(&currentChecks).Error; err != nil {
		return nil, err
	}

	var previousChecks []FeelCheck
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND check_date >= ? AND check_date <= ?", userID, previousWeekStart, previousWeekEnd).
		Order("check_date ASC").
		Find(&previousChecks).Error; err != nil {
		return nil, err
	}

	currentInsight := buildWeeklyInsight(currentChecks, currentWeekStart, currentWeekEnd)
	previousInsight := buildWeeklyInsight(previousChecks, previousWeekStart, previousWeekEnd)

	var streak FeelStreak
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error; err == nil {
		currentInsight.StreakAtEnd = streak.CurrentStreak
	}

	var improvement float64
	if previousInsight.AverageFeel > 0 {
		improvement = ((currentInsight.AverageFeel - previousInsight.AverageFeel) / previousInsight.AverageFeel) * 100
		improvement = math.Round(improvement*100) / 100
	}

	if previousInsight.AverageFeel > 0 {
		diff := ((currentInsight.AverageFeel - previousInsight.AverageFeel) / previousInsight.AverageFeel) * 100
		if diff > 5 {
			currentInsight.MoodTrend = "improving"
		} else if diff < -5 {
			currentInsight.MoodTrend = "declining"
		} else {
			currentInsight.MoodTrend = "stable"
		}
	} else {
		currentInsight.MoodTrend = "stable"
	}

	message := s.generatePersonalizedMessage(appID, currentInsight, previousInsight, improvement, &streak)

	return &InsightsResponse{
		CurrentWeek:  currentInsight,
		PreviousWeek: previousInsight,
		Improvement:  improvement,
		Message:      message,
	}, nil
}

func (s *FeelService) generatePersonalizedMessage(appID string, current, previous WeeklyInsight, improvement float64, streak *FeelStreak) string {
	type messageTemplate struct {
		condition func() bool
		message   string
		idx       int
	}

	templates := []messageTemplate{
		{func() bool { return current.AverageFeel >= 80 && improvement > 5 }, "You're on fire this week! Your mood has been climbing steadily. Keep up whatever you're doing!", 0},
		{func() bool { return current.AverageFeel >= 80 && improvement <= 5 }, "Consistently great vibes! You're maintaining a strong positive mindset.", 1},
		{func() bool { return current.AverageFeel >= 60 && improvement < -5 }, fmt.Sprintf("Your scores are solid but trending down slightly. Consider what made %s feel so great and replicate it.", current.BestDay), 2},
		{func() bool { return current.AverageFeel >= 60 && improvement > 5 }, fmt.Sprintf("Nice improvement! Your mood climbed %.1f%% this week. That positive momentum is powerful.", math.Abs(improvement)), 3},
		{func() bool { return current.AverageMood > current.AverageEnergy+15 }, "Your mood is good but energy is lagging. Try getting more sleep or adding a short walk to your routine.", 4},
		{func() bool { return current.AverageEnergy > current.AverageMood+15 }, "Interesting -- high energy but your mood hasn't caught up. Try a calming activity or mindfulness exercise.", 5},
		{func() bool { return current.TotalCheckIns >= 7 }, "7 days straight -- your consistency is impressive! Regular check-ins build self-awareness.", 6},
		{func() bool { return current.TotalCheckIns >= 5 }, "Great check-in habit forming! Just a couple more days for a perfect week.", 7},
		{func() bool {
			return previous.AverageFeel < 50 && current.AverageFeel >= 50 && improvement > 10
		}, fmt.Sprintf("You're bouncing back! Your scores improved by %.1f%% this week. That takes real resilience.", math.Abs(improvement)), 8},
		{func() bool { return current.AverageFeel < 40 && improvement < -10 }, "Tough week. Remember: tracking even the hard days builds awareness. Consider reaching out to someone you trust.", 9},
		{func() bool { return current.AverageFeel < 40 }, "Hang in there. Low periods are part of the journey. Each day you check in is a step toward understanding yourself better.", 10},
		{func() bool { return streak != nil && streak.CurrentStreak >= 14 }, fmt.Sprintf("Two weeks strong! Your %d-day streak shows incredible dedication to self-awareness.", streak.CurrentStreak), 11},
		{func() bool { return streak != nil && streak.CurrentStreak >= 7 }, "One week streak! You're building a powerful habit of self-reflection.", 12},
		{func() bool { return current.AverageFeel >= 60 }, fmt.Sprintf("Solid week! Your average feel score is %.0f. Keep the positive energy flowing.", current.AverageFeel), 13},
		{func() bool { return improvement > 0 }, fmt.Sprintf("Progress! Your mood improved by %.1f%% compared to last week.", math.Abs(improvement)), 14},
		{func() bool { return improvement < 0 }, fmt.Sprintf("Your mood dipped %.1f%% from last week. Try to identify what changed and adjust.", math.Abs(improvement)), 15},
		{func() bool { return current.TotalCheckIns > 0 }, "Keep checking in daily to unlock deeper insights about your mood patterns!", 16},
		{func() bool { return true }, "Start checking in to see your weekly mood insights!", 17},
	}

	lastIdx := 0
	if streak != nil {
		lastIdx = streak.LastMessageIdx
	}

	for _, t := range templates {
		if t.condition() && t.idx != lastIdx {
			if streak != nil && streak.ID != uuid.Nil {
				streak.LastMessageIdx = t.idx
				s.db.Model(streak).Update("last_message_idx", t.idx)
			}
			return t.message
		}
	}

	for _, t := range templates {
		if t.condition() {
			return t.message
		}
	}

	return "Start checking in to see your weekly mood insights!"
}

// GetWeeklyRecap returns a summary of the past 7 days for the recap card.
func (s *FeelService) GetWeeklyRecap(appID string, userID uuid.UUID) (*WeeklyRecapResponse, error) {
	now := time.Now()
	weekEnd := now.Truncate(24 * time.Hour)
	weekStart := weekEnd.AddDate(0, 0, -6)

	var checks []FeelCheck
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND check_date >= ? AND check_date <= ?", userID, weekStart, weekEnd).
		Order("check_date ASC").
		Find(&checks).Error; err != nil {
		return nil, err
	}

	if len(checks) == 0 {
		return nil, errors.New("no check-ins this week")
	}

	var totalScore float64
	bestScore := 0
	bestDay := ""
	emojiCount := make(map[string]int)

	dailyScores := make([]int, 7)
	dateToIndex := make(map[string]int)
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dateToIndex[day.Format("2006-01-02")] = i
	}

	for _, check := range checks {
		totalScore += float64(check.FeelScore)
		dateKey := check.CheckDate.Format("2006-01-02")
		if idx, ok := dateToIndex[dateKey]; ok {
			dailyScores[idx] = check.FeelScore
		}
		if check.FeelScore > bestScore {
			bestScore = check.FeelScore
			bestDay = check.CheckDate.Format("Monday")
		}
		if check.MoodEmoji != "" {
			emojiCount[check.MoodEmoji]++
		}
	}

	topEmoji := ""
	maxCount := 0
	for emoji, count := range emojiCount {
		if count > maxCount {
			maxCount = count
			topEmoji = emoji
		}
	}

	currentStreak := 0
	var streak FeelStreak
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error; err == nil {
		currentStreak = streak.CurrentStreak
	}

	avgScore := math.Round((totalScore/float64(len(checks)))*100) / 100

	return &WeeklyRecapResponse{
		TotalCheckins: len(checks),
		AverageScore:  avgScore,
		BestScore:     bestScore,
		BestDay:       bestDay,
		TopEmoji:      topEmoji,
		DailyScores:   dailyScores,
		CurrentStreak: currentStreak,
		WeekStart:     weekStart.Format("2006-01-02"),
		WeekEnd:       weekEnd.Format("2006-01-02"),
	}, nil
}

func buildWeeklyInsight(checks []FeelCheck, weekStart, weekEnd time.Time) WeeklyInsight {
	insight := WeeklyInsight{
		WeekStart: weekStart.Format("2006-01-02"),
		WeekEnd:   weekEnd.Format("2006-01-02"),
	}

	if len(checks) == 0 {
		return insight
	}

	var totalMood, totalEnergy, totalFeel float64
	bestCheck := checks[0]
	worstCheck := checks[0]
	emojiCount := make(map[string]int)

	for _, check := range checks {
		totalMood += float64(check.MoodScore)
		totalEnergy += float64(check.EnergyScore)
		totalFeel += float64(check.FeelScore)

		if check.FeelScore > bestCheck.FeelScore {
			bestCheck = check
		}
		if check.FeelScore < worstCheck.FeelScore {
			worstCheck = check
		}

		if check.MoodEmoji != "" {
			emojiCount[check.MoodEmoji]++
		}
	}

	count := float64(len(checks))
	insight.AverageMood = math.Round((totalMood/count)*100) / 100
	insight.AverageEnergy = math.Round((totalEnergy/count)*100) / 100
	insight.AverageFeel = math.Round((totalFeel/count)*100) / 100
	insight.TotalCheckIns = len(checks)
	insight.BestDay = bestCheck.CheckDate.Format("2006-01-02")
	insight.WorstDay = worstCheck.CheckDate.Format("2006-01-02")

	var maxCount int
	for emoji, c := range emojiCount {
		if c > maxCount {
			maxCount = c
			insight.DominantEmoji = emoji
		}
	}

	return insight
}
