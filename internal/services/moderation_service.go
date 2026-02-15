package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/dto"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/models"
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrReportNotFound = errors.New("report not found")
	ErrAlreadyBlocked = errors.New("user already blocked")
	ErrSelfBlock      = errors.New("cannot block yourself")
)

var BannedWords = []string{
	"fuck", "fucking", "fucker", "shit", "shitty", "bullshit",
	"ass", "asshole", "bastard", "bitch", "cunt",
	"nigger", "nigga", "chink", "spic", "kike", "faggot", "fag",
	"retard", "retarded", "tranny",
	"porn", "porno", "nude", "nudes",
	"spam", "scam", "scammer", "phishing", "malware",
}

type ModerationService struct {
	db                  *gorm.DB
	bannedWordRegexps   []*regexp.Regexp
	urlPattern          *regexp.Regexp
	emailPattern        *regexp.Regexp
	phonePattern        *regexp.Regexp
	repeatedCharPattern *regexp.Regexp
	allCapsPattern      *regexp.Regexp
	compiled            bool
	mu                  sync.RWMutex
}

func NewModerationService(db *gorm.DB) *ModerationService {
	ms := &ModerationService{db: db}
	ms.compilePatterns()
	return ms
}

func (ms *ModerationService) compilePatterns() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.compiled {
		return
	}

	ms.bannedWordRegexps = make([]*regexp.Regexp, 0, len(BannedWords))
	for _, word := range BannedWords {
		pattern := `(?i)\b` + regexp.QuoteMeta(word) + `\b`
		re, err := regexp.Compile(pattern)
		if err == nil {
			ms.bannedWordRegexps = append(ms.bannedWordRegexps, re)
		}
	}

	ms.urlPattern = regexp.MustCompile(`(?i)(https?://\S+|www\.\S+\.\S+)`)
	ms.emailPattern = regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	ms.phonePattern = regexp.MustCompile(`\d{3}[-.\s]?\d{3}[-.\s]?\d{4}|\(\d{3}\)\s*\d{3}[-.\s]?\d{4}`)
	ms.repeatedCharPattern = regexp.MustCompile(`(?i)(a{4,}|b{4,}|c{4,}|d{4,}|e{4,}|f{4,}|g{4,}|h{4,}|i{4,}|j{4,}|k{4,}|l{4,}|m{4,}|n{4,}|o{4,}|p{4,}|q{4,}|r{4,}|s{4,}|t{4,}|u{4,}|v{4,}|w{4,}|x{4,}|y{4,}|z{4,}|!{4,}|\?{4,}|\.{4,})`)
	ms.allCapsPattern = regexp.MustCompile(`[A-Z]{5,}`)
	ms.compiled = true
}

func (ms *ModerationService) FilterContent(text string) (bool, string) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if text == "" {
		return true, ""
	}
	for _, re := range ms.bannedWordRegexps {
		if re.MatchString(text) {
			return false, "inappropriate_language"
		}
	}
	if ms.urlPattern.MatchString(text) {
		return false, "url_not_allowed"
	}
	if ms.emailPattern.MatchString(text) {
		return false, "contact_info_not_allowed"
	}
	if ms.phonePattern.MatchString(text) {
		return false, "contact_info_not_allowed"
	}
	if ms.repeatedCharPattern.MatchString(text) {
		return false, "spam_detected"
	}
	capsMatches := ms.allCapsPattern.FindAllString(text, -1)
	if len(capsMatches) > 2 {
		return false, "excessive_caps"
	}
	return true, ""
}

func (ms *ModerationService) ContainsProfanity(text string) bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	for _, re := range ms.bannedWordRegexps {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func (ms *ModerationService) GetRejectionMessage(reason string) string {
	messages := map[string]string{
		"inappropriate_language":   "Your response contains inappropriate language.",
		"url_not_allowed":          "URLs and web links are not allowed.",
		"contact_info_not_allowed": "Contact information is not allowed.",
		"spam_detected":            "Your response appears to be spam.",
		"excessive_caps":           "Please avoid using excessive capital letters.",
	}
	if msg, ok := messages[reason]; ok {
		return msg
	}
	return "Your response does not meet our content guidelines."
}

func (s *ModerationService) CreateReport(appID string, reporterID uuid.UUID, req *dto.CreateReportRequest) (*models.Report, error) {
	validTypes := map[string]bool{"user": true, "post": true, "comment": true}
	if !validTypes[req.ContentType] {
		return nil, errors.New("invalid content_type: must be user, post, or comment")
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, errors.New("reason is required")
	}

	report := models.Report{
		ID:          uuid.New(),
		AppID:       appID,
		ReporterID:  reporterID,
		ContentType: req.ContentType,
		ContentID:   req.ContentID,
		Reason:      req.Reason,
		Status:      "pending",
	}

	if err := s.db.Create(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to create report: %w", err)
	}
	return &report, nil
}

func (s *ModerationService) ListReports(appID string, status string, limit, offset int) ([]models.Report, int64, error) {
	var reports []models.Report
	var total int64

	query := s.db.Model(&models.Report{}).Scopes(tenant.ForTenant(appID))
	if status != "" {
		query = query.Where("status = ?", status)
	}
	query.Count(&total)

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&reports).Error; err != nil {
		return nil, 0, err
	}
	return reports, total, nil
}

func (s *ModerationService) ActionReport(appID string, reportID uuid.UUID, req *dto.ActionReportRequest) error {
	validStatuses := map[string]bool{"reviewed": true, "actioned": true, "dismissed": true}
	if !validStatuses[req.Status] {
		return errors.New("invalid status: must be reviewed, actioned, or dismissed")
	}

	result := s.db.Model(&models.Report{}).
		Scopes(tenant.ForTenant(appID)).
		Where("id = ?", reportID).
		Updates(map[string]interface{}{
			"status":     req.Status,
			"admin_note": req.AdminNote,
		})
	if result.RowsAffected == 0 {
		return ErrReportNotFound
	}
	return result.Error
}

func (s *ModerationService) BlockUser(appID string, blockerID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return ErrSelfBlock
	}

	var existing models.Block
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).First(&existing).Error; err == nil {
		return ErrAlreadyBlocked
	}

	block := models.Block{
		ID:        uuid.New(),
		AppID:     appID,
		BlockerID: blockerID,
		BlockedID: blockedID,
	}
	return s.db.Create(&block).Error
}

func (s *ModerationService) UnblockUser(appID string, blockerID, blockedID uuid.UUID) error {
	return s.db.Scopes(tenant.ForTenant(appID)).
		Where("blocker_id = ? AND blocked_id = ?", blockerID, blockedID).
		Delete(&models.Block{}).Error
}

func (s *ModerationService) GetBlockedIDs(appID string, userID uuid.UUID) ([]uuid.UUID, error) {
	var blocks []models.Block
	if err := s.db.Scopes(tenant.ForTenant(appID)).Where("blocker_id = ?", userID).Find(&blocks).Error; err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(blocks))
	for i, b := range blocks {
		ids[i] = b.BlockedID
	}
	return ids, nil
}
