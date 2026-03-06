package lucky_draw

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/config"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrInvalidInput   = errors.New("input is required and must be at most 5000 characters")
	ErrNotFound       = errors.New("draw result not found")
	ErrNotOwner       = errors.New("not the owner of this result")
	ErrInvalidGuestID = errors.New("guest results cannot be associated with a user")
)

type LuckyDrawService struct {
	db *gorm.DB
}

func NewLuckyDrawService(db *gorm.DB, cfg *config.Config) *LuckyDrawService {
	return &LuckyDrawService{db: db}
}

func (s *LuckyDrawService) Create(appID string, userID *uuid.UUID, req CreateDrawRequest) (*LuckyDraw, error) {
	// Validate input
	if req.Input == "" || len(req.Input) > 5000 {
		return nil, ErrInvalidInput
	}

	// Validate guest/user logic
	if req.IsGuest && userID != nil {
		return nil, ErrInvalidGuestID
	}

	// Generate result (simplified - in real app this would call AI/analysis logic)
	result := "Your lucky analysis: " + req.Input[:min(50, len(req.Input))] + "..."
	category := "general"
	score := 75

	// Prepare metadata
	var metadataJSON datatypes.JSON
	if req.Metadata != nil {
		jsonBytes, _ := json.Marshal(req.Metadata)
		metadataJSON = jsonBytes
	} else {
		metadataJSON = []byte("{}")
	}

	draw := &LuckyDraw{
		UserID:   userID,
		Input:    req.Input,
		Result:   result,
		Score:    &score,
		Category: category,
		Metadata: metadataJSON,
		IsGuest:  req.IsGuest,
	}

	if err := s.db.Scopes(tenant.ForTenant(appID)).Create(draw).Error; err != nil {
		return nil, err
	}

	// Update user history if not guest
	if !req.IsGuest && userID != nil {
		s.updateUserHistory(appID, *userID)
	}

	return draw, nil
}

func (s *LuckyDrawService) updateUserHistory(appID string, userID uuid.UUID) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")

	var history UserHistory
	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND date = ?", userID, todayStr).
		First(&history).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Check yesterday for streak calculation
		yesterday := today.Add(-24 * time.Hour)
		yesterdayStr := yesterday.Format("2006-01-02")
		var yesterdayHistory UserHistory
		yesterdayErr := s.db.Scopes(tenant.ForTenant(appID)).
			Where("user_id = ? AND date = ?", userID, yesterdayStr).
			First(&yesterdayHistory).Error

		streak := 1
		if yesterdayErr == nil {
			streak = yesterdayHistory.Streak + 1
		}

		// Create new history entry
		history = UserHistory{
			UserID: userID,
			Date:   todayStr,
			Count:  1,
			Streak: streak,
		}
		s.db.Scopes(tenant.ForTenant(appID)).Create(&history)
	} else if err == nil {
		// Update existing entry
		history.Count++
		s.db.Scopes(tenant.ForTenant(appID)).Save(&history)
	}
}

func (s *LuckyDrawService) Get(appID string, userID *uuid.UUID, id uuid.UUID) (*LuckyDraw, error) {
	var draw LuckyDraw

	query := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id)

	// If userID is provided, ensure they own the result (unless it's a guest result they created)
	if userID != nil {
		query = query.Where("(user_id = ? OR is_guest = true)", userID)
	}

	if err := query.First(&draw).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &draw, nil
}

func (s *LuckyDrawService) List(appID string, userID *uuid.UUID, limit, offset int) ([]LuckyDraw, int64, error) {
	var draws []LuckyDraw
	var total int64

	query := s.db.Scopes(tenant.ForTenant(appID))

	// Filter by user if provided
	if userID != nil {
		query = query.Where("user_id = ?", userID)
	} else {
		// Only show guest results when no userID
		query = query.Where("is_guest = true")
	}

	// Count total
	if err := query.Model(&LuckyDraw{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	if err := query.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&draws).Error; err != nil {
		return nil, 0, err
	}

	return draws, total, nil
}

func (s *LuckyDrawService) Delete(appID string, userID *uuid.UUID, id uuid.UUID) error {
	query := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", id)

	// Ensure user owns the result
	if userID != nil {
		query = query.Where("user_id = ?", userID)
	}

	result := query.Delete(&LuckyDraw{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *LuckyDrawService) GetStats(appID string, userID uuid.UUID) (*UserStatsResponse, error) {
	// Get total draws
	var totalDraws int64
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Model(&LuckyDraw{}).
		Where("user_id = ?", userID).
		Count(&totalDraws).Error; err != nil {
		return nil, err
	}

	// Get current streak from today's history
	today := time.Now().UTC().Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")
	var todayHistory UserHistory
	currentStreak := 0
	longestStreak := 0

	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND date = ?", userID, todayStr).
		First(&todayHistory).Error

	if err == nil {
		currentStreak = todayHistory.Streak
	}

	// Get longest streak from all history
	var maxStreak struct {
		MaxStreak int
	}
	s.db.Scopes(tenant.ForTenant(appID)).
		Model(&UserHistory{}).
		Select("MAX(streak) as max_streak").
		Where("user_id = ?", userID).
		Scan(&maxStreak)

	if maxStreak.MaxStreak > 0 {
		longestStreak = maxStreak.MaxStreak
	}

	// Get last 30 days history
	var histories []UserHistory
	s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND date >= ?", userID, today.AddDate(0, 0, -30).Format("2006-01-02")).
		Order("date DESC").
		Find(&histories)

	dailyHistory := make([]HistoryResponse, len(histories))
	for i, h := range histories {
		dailyHistory[i] = HistoryResponse{
			Date:   h.Date,
			Count:  h.Count,
			Streak: h.Streak,
		}
	}

	return &UserStatsResponse{
		TotalDraws:    totalDraws,
		CurrentStreak: currentStreak,
		LongestStreak: longestStreak,
		DailyHistory:  dailyHistory,
	}, nil
}

func (s *LuckyDrawService) GetHistory(appID string, userID uuid.UUID, days int) ([]HistoryResponse, error) {
	if days < 1 || days > 90 {
		days = 30
	}

	startDate := time.Now().UTC().AddDate(0, 0, -days).Truncate(24 * time.Hour)
	startDateStr := startDate.Format("2006-01-02")

	var histories []UserHistory
	if err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("user_id = ? AND date >= ?", userID, startDateStr).
		Order("date DESC").
		Find(&histories).Error; err != nil {
		return nil, err
	}

	response := make([]HistoryResponse, len(histories))
	for i, h := range histories {
		response[i] = HistoryResponse{
			Date:   h.Date,
			Count:  h.Count,
			Streak: h.Streak,
		}
	}

	return response, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
