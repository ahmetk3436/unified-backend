package confessit

import (
	"errors"
	"time"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ConfessionService handles confession CRUD and engagement.
type ConfessionService struct {
	db *gorm.DB
}

func NewConfessionService(db *gorm.DB) *ConfessionService {
	return &ConfessionService{db: db}
}

func (s *ConfessionService) CreateConfession(appID string, userID uuid.UUID, content, category, mood string) (*Confession, error) {
	if len(content) < 10 {
		return nil, errors.New("confession must be at least 10 characters")
	}
	if len(content) > 1000 {
		return nil, errors.New("confession must be under 1000 characters")
	}

	confession := &Confession{
		AppID:       appID,
		UserID:      userID,
		Content:     content,
		Category:    category,
		Mood:        mood,
		IsAnonymous: true,
	}

	if err := s.db.Create(confession).Error; err != nil {
		return nil, err
	}

	// Update streak
	s.updateStreak(appID, userID)

	return confession, nil
}

func (s *ConfessionService) GetFeed(appID string, page, limit int) ([]Confession, int64, error) {
	var confessions []Confession
	var total int64

	offset := (page - 1) * limit

	s.db.Model(&Confession{}).Scopes(tenant.ForTenant(appID)).Count(&total)

	err := s.db.Scopes(tenant.ForTenant(appID)).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&confessions).Error

	if err != nil {
		return nil, 0, err
	}

	// Increment view count for each
	for i := range confessions {
		s.db.Model(&confessions[i]).Update("view_count", gorm.Expr("view_count + 1"))
	}

	return confessions, total, nil
}

func (s *ConfessionService) GetByCategory(appID, category string, page, limit int) ([]Confession, int64, error) {
	var confessions []Confession
	var total int64

	offset := (page - 1) * limit

	query := s.db.Model(&Confession{}).Scopes(tenant.ForTenant(appID)).Where("category = ?", category)
	query.Count(&total)

	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&confessions).Error

	return confessions, total, err
}

func (s *ConfessionService) LikeConfession(appID string, userID, confessionID uuid.UUID) error {
	var confession Confession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", confessionID).First(&confession).Error; err != nil {
		return err
	}

	// Check if already liked
	var existing ConfessionLike
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND confession_id = ?", userID, confessionID).First(&existing).Error; err == nil {
		// Unlike
		s.db.Delete(&existing)
		s.db.Model(&Confession{}).Where("id = ?", confessionID).
			Update("like_count", gorm.Expr("like_count - 1"))
		return nil
	}

	// Create like
	like := &ConfessionLike{
		AppID:        appID,
		UserID:       userID,
		ConfessionID: confessionID,
	}

	if err := s.db.Create(like).Error; err != nil {
		return err
	}

	s.db.Model(&Confession{}).Where("id = ?", confessionID).
		Update("like_count", gorm.Expr("like_count + 1"))

	return nil
}

func (s *ConfessionService) AddComment(appID string, userID, confessionID uuid.UUID, content string) (*ConfessionComment, error) {
	if len(content) < 1 || len(content) > 500 {
		return nil, errors.New("comment must be 1-500 characters")
	}

	comment := &ConfessionComment{
		AppID:        appID,
		ConfessionID: confessionID,
		UserID:       userID,
		Content:      content,
		IsAnonymous:  true,
	}

	if err := s.db.Create(comment).Error; err != nil {
		return nil, err
	}

	s.db.Model(&Confession{}).Where("id = ?", confessionID).
		Update("comment_count", gorm.Expr("comment_count + 1"))

	return comment, nil
}

func (s *ConfessionService) GetComments(appID string, confessionID uuid.UUID, page, limit int) ([]ConfessionComment, error) {
	var comments []ConfessionComment
	offset := (page - 1) * limit

	err := s.db.Scopes(tenant.ForTenant(appID)).
		Where("confession_id = ?", confessionID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&comments).Error

	return comments, err
}

func (s *ConfessionService) IncrementShare(appID string, confessionID uuid.UUID) error {
	return s.db.Model(&Confession{}).
		Scopes(tenant.ForTenant(appID)).
		Where("id = ?", confessionID).
		Update("share_count", gorm.Expr("share_count + 1")).Error
}

func (s *ConfessionService) GetStats(appID string, userID uuid.UUID) (*ConfessionStreak, error) {
	var streak ConfessionStreak
	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = ConfessionStreak{
			AppID:  appID,
			UserID: userID,
		}
		s.db.Create(&streak)
	}
	return &streak, nil
}

func (s *ConfessionService) GetMyConfessions(appID string, userID uuid.UUID, page, limit int) ([]Confession, int64, error) {
	var confessions []Confession
	var total int64

	offset := (page - 1) * limit

	query := s.db.Model(&Confession{}).Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID)
	query.Count(&total)

	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&confessions).Error

	return confessions, total, err
}

func (s *ConfessionService) DeleteConfession(appID string, userID, confessionID uuid.UUID) error {
	var confession Confession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ? AND user_id = ?", confessionID, userID).First(&confession).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("confession not found or not owned by you")
		}
		return err
	}

	return s.db.Delete(&confession).Error
}

func (s *ConfessionService) GetConfession(appID string, confessionID uuid.UUID) (*Confession, error) {
	var confession Confession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", confessionID).First(&confession).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("confession not found")
		}
		return nil, err
	}

	s.db.Model(&confession).Update("view_count", gorm.Expr("view_count + 1"))

	return &confession, nil
}

func (s *ConfessionService) ReactToConfession(appID string, userID, confessionID uuid.UUID, emoji string) error {
	if emoji == "" {
		return errors.New("emoji is required")
	}

	var confession Confession
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("id = ?", confessionID).First(&confession).Error; err != nil {
		return err
	}

	// Check if user already reacted with this emoji
	var existing ConfessionReaction
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ? AND confession_id = ? AND emoji = ?", userID, confessionID, emoji).First(&existing).Error; err == nil {
		// Remove the reaction (toggle off)
		s.db.Delete(&existing)
		s.db.Model(&Confession{}).Where("id = ?", confessionID).
			Update("reaction_count", gorm.Expr("reaction_count - 1"))
		return nil
	}

	reaction := &ConfessionReaction{
		AppID:        appID,
		UserID:       userID,
		ConfessionID: confessionID,
		Emoji:        emoji,
	}

	if err := s.db.Create(reaction).Error; err != nil {
		return err
	}

	s.db.Model(&Confession{}).Where("id = ?", confessionID).
		Update("reaction_count", gorm.Expr("reaction_count + 1"))

	return nil
}

func (s *ConfessionService) GetReactions(appID string, confessionID uuid.UUID) ([]map[string]interface{}, error) {
	var results []struct {
		Emoji string
		Count int64
	}

	err := s.db.Model(&ConfessionReaction{}).
		Select("emoji, count(*) as count").
		Scopes(tenant.ForTenant(appID)).
		Where("confession_id = ?", confessionID).
		Group("emoji").
		Order("count DESC").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	reactions := make([]map[string]interface{}, len(results))
	for i, r := range results {
		reactions[i] = map[string]interface{}{
			"emoji": r.Emoji,
			"count": r.Count,
		}
	}

	return reactions, nil
}

// GetTrendingFeed retrieves confessions ordered by trending score.
func (s *ConfessionService) GetTrendingFeed(appID string, page, limit int) ([]Confession, int64, error) {
	var confessions []Confession
	var total int64

	offset := (page - 1) * limit

	if err := s.db.Model(&Confession{}).Scopes(tenant.ForTenant(appID)).Where("deleted_at IS NULL").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, app_id, user_id, content, category, mood, is_anonymous, like_count, comment_count, share_count, view_count, reaction_count, created_at, updated_at, deleted_at,
		((like_count * 3) + (reaction_count * 2) + (comment_count * 2) + (view_count * 0.1) - (EXTRACT(EPOCH FROM (NOW() - created_at)) / 3600 * 1.5)) as score
		FROM confessions
		WHERE deleted_at IS NULL AND app_id = ?
		ORDER BY score DESC
		OFFSET ? LIMIT ?
	`

	if err := s.db.Raw(query, appID, offset, limit).Scan(&confessions).Error; err != nil {
		return nil, 0, err
	}

	return confessions, total, nil
}

func (s *ConfessionService) updateStreak(appID string, userID uuid.UUID) {
	var streak ConfessionStreak
	today := time.Now().Truncate(24 * time.Hour)

	err := s.db.Scopes(tenant.ForTenant(appID)).Where("user_id = ?", userID).First(&streak).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		streak = ConfessionStreak{
			AppID:         appID,
			UserID:        userID,
			CurrentStreak: 1,
			LongestStreak: 1,
			TotalPosts:    1,
			LastPostDate:  today,
		}
		s.db.Create(&streak)
		return
	}

	lastPost := streak.LastPostDate.Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	if lastPost.Equal(today) {
		streak.TotalPosts++
	} else if lastPost.Equal(yesterday) {
		streak.CurrentStreak++
		streak.TotalPosts++
		if streak.CurrentStreak > streak.LongestStreak {
			streak.LongestStreak = streak.CurrentStreak
		}
	} else {
		streak.CurrentStreak = 1
		streak.TotalPosts++
	}

	streak.LastPostDate = today
	s.db.Save(&streak)
}
