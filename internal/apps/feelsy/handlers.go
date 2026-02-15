package feelsy

import (
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// FeelHandler handles HTTP requests for feel check-ins.
type FeelHandler struct {
	service *FeelService
}

// NewFeelHandler creates a new FeelHandler.
func NewFeelHandler(service *FeelService) *FeelHandler {
	return &FeelHandler{service: service}
}

// CreateFeelCheck handles POST /api/feels
func (h *FeelHandler) CreateFeelCheck(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	var req struct {
		MoodScore    int    `json:"mood_score"`
		EnergyScore  int    `json:"energy_score"`
		MoodEmoji    string `json:"mood_emoji"`
		Note         string `json:"note"`
		JournalEntry string `json:"journal_entry"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	check, err := h.service.CreateFeelCheck(appID, userID, req.MoodScore, req.EnergyScore, req.MoodEmoji, req.Note, req.JournalEntry)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(check)
}

// GetTodayCheck handles GET /api/feels/today
func (h *FeelHandler) GetTodayCheck(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	check, err := h.service.GetTodayCheck(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true, "message": "No check-in today",
		})
	}

	return c.JSON(check)
}

// GetFeelHistory handles GET /api/feels/history
func (h *FeelHandler) GetFeelHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit > 100 {
		limit = 100
	}

	checks, total, err := h.service.GetFeelHistory(appID, userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch history",
		})
	}

	return c.JSON(fiber.Map{
		"data": checks, "total": total,
		"limit": limit, "offset": offset,
	})
}

// GetFeelStats handles GET /api/feels/stats
func (h *FeelHandler) GetFeelStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	stats, err := h.service.GetFeelStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch stats",
		})
	}

	return c.JSON(stats)
}

// SendGoodVibe handles POST /api/feels/vibe
func (h *FeelHandler) SendGoodVibe(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	var req struct {
		ReceiverID string `json:"receiver_id"`
		Message    string `json:"message"`
		VibeType   string `json:"vibe_type"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	receiverID, err := uuid.Parse(req.ReceiverID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid receiver ID",
		})
	}

	vibe, err := h.service.SendGoodVibe(appID, userID, receiverID, req.Message, req.VibeType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(vibe)
}

// GetReceivedVibes handles GET /api/feels/vibes
func (h *FeelHandler) GetReceivedVibes(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit > 50 {
		limit = 50
	}

	vibes, err := h.service.GetReceivedVibes(appID, userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch vibes",
		})
	}

	return c.JSON(fiber.Map{"data": vibes})
}

// GetFriendFeels handles GET /api/feels/friends
func (h *FeelHandler) GetFriendFeels(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	feels, err := h.service.GetFriendFeels(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch friend feels",
		})
	}

	return c.JSON(fiber.Map{"data": feels})
}

// SendFriendRequest handles POST /api/feels/friends/add
func (h *FeelHandler) SendFriendRequest(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	var req struct {
		FriendEmail string `json:"friend_email"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	if req.FriendEmail == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "friend_email is required",
		})
	}

	request, err := h.service.SendFriendRequest(appID, userID, req.FriendEmail)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": request})
}

// AcceptFriendRequest handles PUT /api/feels/friends/:id/accept
func (h *FeelHandler) AcceptFriendRequest(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	requestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request ID",
		})
	}

	if err := h.service.AcceptFriendRequest(appID, userID, requestID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Friend request accepted"})
}

// RejectFriendRequest handles DELETE /api/feels/friends/:id/reject
func (h *FeelHandler) RejectFriendRequest(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	requestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request ID",
		})
	}

	if err := h.service.RejectFriendRequest(appID, userID, requestID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Friend request rejected"})
}

// ListFriendRequests handles GET /api/feels/friends/requests
func (h *FeelHandler) ListFriendRequests(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	requests, err := h.service.ListFriendRequests(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch friend requests",
		})
	}

	return c.JSON(fiber.Map{"data": requests})
}

// ListFriends handles GET /api/feels/friends/list
func (h *FeelHandler) ListFriends(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	friends, err := h.service.ListFriends(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch friends",
		})
	}

	return c.JSON(fiber.Map{"data": friends})
}

// RemoveFriend handles DELETE /api/feels/friends/:id
func (h *FeelHandler) RemoveFriend(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	friendshipID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid friendship ID",
		})
	}

	if err := h.service.RemoveFriend(appID, userID, friendshipID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "Friend removed"})
}

// GetWeeklyInsights handles GET /api/feels/insights
func (h *FeelHandler) GetWeeklyInsights(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	insights, err := h.service.GetWeeklyInsights(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to fetch weekly insights",
		})
	}

	return c.JSON(insights)
}

// UpdateJournal handles PUT /api/feels/:id/journal
func (h *FeelHandler) UpdateJournal(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	checkID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid check-in ID",
		})
	}

	var req struct {
		JournalEntry string `json:"journal_entry"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	check, err := h.service.UpdateJournalEntry(appID, userID, checkID, req.JournalEntry)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.JSON(check)
}

// GetWeeklyRecap handles GET /api/feels/recap
func (h *FeelHandler) GetWeeklyRecap(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	recap, err := h.service.GetWeeklyRecap(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": true, "message": "No data for recap",
		})
	}

	return c.JSON(recap)
}
