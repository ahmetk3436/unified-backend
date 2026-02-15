package aurascan

import (
	"encoding/base64"
	"io"
	"strconv"
	"strings"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// =============================================================================
// AuraHandler
// =============================================================================

type AuraHandler struct {
	auraService *AuraService
}

func NewAuraHandler(auraService *AuraService) *AuraHandler {
	return &AuraHandler{auraService: auraService}
}

func (h *AuraHandler) CheckScanEligibility(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	isSubscribed := h.auraService.IsSubscribed(appID, userID)
	allowed, remaining, err := h.auraService.CanScan(appID, userID, isSubscribed)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to check eligibility"})
	}

	return c.JSON(ScanEligibilityResponse{CanScan: allowed, Remaining: remaining, IsSubscribed: isSubscribed})
}

func (h *AuraHandler) Scan(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	isSubscribed := h.auraService.IsSubscribed(appID, userID)
	allowed, _, err := h.auraService.CanScan(appID, userID, isSubscribed)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to verify scan eligibility"})
	}
	if !allowed {
		return c.Status(429).JSON(fiber.Map{"error": true, "message": "Daily scan limit reached. Upgrade to Premium for unlimited scans."})
	}

	var req CreateAuraRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid request body"})
	}

	if req.ImageData != "" && len(req.ImageData) > 3*1024*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Image data too large. Maximum 3MB base64."})
	}
	if req.ImageData == "" && req.ImageURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Either image_data or image_url is required"})
	}

	reading, err := h.auraService.Create(appID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(reading)
}

func (h *AuraHandler) ScanWithUpload(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	isSubscribed := h.auraService.IsSubscribed(appID, userID)
	allowed, _, err := h.auraService.CanScan(appID, userID, isSubscribed)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to verify scan eligibility"})
	}
	if !allowed {
		return c.Status(429).JSON(fiber.Map{"error": true, "message": "Daily scan limit reached. Upgrade to Premium for unlimited scans."})
	}

	file, err := c.FormFile("image")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Image file is required"})
	}

	contentType := file.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/jpeg") && !strings.HasPrefix(contentType, "image/png") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Only JPEG and PNG images are supported"})
	}

	if file.Size > 4*1024*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Image too large. Maximum 4MB."})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to read image"})
	}
	defer f.Close()

	fileBytes, err := io.ReadAll(f)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to read image data"})
	}

	b64Data := base64.StdEncoding.EncodeToString(fileBytes)
	req := CreateAuraRequest{ImageData: b64Data}

	reading, err := h.auraService.Create(appID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(reading)
}

func (h *AuraHandler) GetByID(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	readingID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid reading ID"})
	}

	reading, err := h.auraService.GetByID(appID, userID, readingID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": "Reading not found"})
	}

	return c.JSON(reading)
}

func (h *AuraHandler) List(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))

	readings, total, err := h.auraService.List(appID, userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to fetch readings"})
	}

	items := make([]AuraReadingResponse, 0, len(readings))
	for _, r := range readings {
		items = append(items, AuraReadingResponse{
			ID: r.ID, UserID: r.UserID, AuraColor: r.AuraColor,
			SecondaryColor: r.SecondaryColor, EnergyLevel: r.EnergyLevel,
			MoodScore: r.MoodScore, Personality: r.Personality,
			Strengths: r.Strengths, Challenges: r.Challenges,
			DailyAdvice: r.DailyAdvice, ImageURL: r.ImageURL,
			AnalyzedAt: r.AnalyzedAt, CreatedAt: r.CreatedAt,
		})
	}

	return c.JSON(AuraListResponse{Data: items, Page: page, PageSize: pageSize, TotalCount: total})
}

func (h *AuraHandler) Stats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	stats, err := h.auraService.GetStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to fetch stats"})
	}

	return c.JSON(stats)
}

// =============================================================================
// AuraMatchHandler
// =============================================================================

type AuraMatchHandler struct {
	matchService *AuraMatchService
}

func NewAuraMatchHandler(matchService *AuraMatchService) *AuraMatchHandler {
	return &AuraMatchHandler{matchService: matchService}
}

func (h *AuraMatchHandler) CreateMatch(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	var req CreateMatchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid request body"})
	}

	match, err := h.matchService.Create(appID, userID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(match)
}

func (h *AuraMatchHandler) GetMatches(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	matches, err := h.matchService.List(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to fetch matches"})
	}

	return c.JSON(fiber.Map{"data": matches})
}

func (h *AuraMatchHandler) GetMatchByFriend(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	friendID, err := uuid.Parse(c.Params("friend_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": true, "message": "Invalid friend ID"})
	}

	match, err := h.matchService.GetByFriend(appID, userID, friendID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": "No match found with this friend"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to fetch match"})
	}

	return c.JSON(match)
}

// =============================================================================
// StreakHandler
// =============================================================================

type StreakHandler struct {
	streakService *StreakService
}

func NewStreakHandler(streakService *StreakService) *StreakHandler {
	return &StreakHandler{streakService: streakService}
}

func (h *StreakHandler) GetStreak(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	streak, err := h.streakService.Get(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to fetch streak"})
	}

	return c.JSON(streak)
}

func (h *StreakHandler) UpdateStreak(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": true, "message": "Invalid user ID"})
	}

	result, err := h.streakService.Update(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": true, "message": "Failed to update streak"})
	}

	return c.JSON(result)
}
