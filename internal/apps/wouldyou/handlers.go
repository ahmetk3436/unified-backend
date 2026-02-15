package wouldyou

import (
	"strconv"

	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ChallengeHandler handles HTTP requests for WouldYouRather challenges.
type ChallengeHandler struct {
	service           *ChallengeService
	questionGenerator *QuestionGeneratorService
}

// NewChallengeHandler creates a new ChallengeHandler.
func NewChallengeHandler(service *ChallengeService, qg *QuestionGeneratorService) *ChallengeHandler {
	return &ChallengeHandler{
		service:           service,
		questionGenerator: qg,
	}
}

// GetDailyChallenge handles GET /api/challenges/daily
func (h *ChallengeHandler) GetDailyChallenge(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)

	challenge, err := h.service.GetDailyChallenge(appID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to get challenge",
		})
	}

	userID, guestID := extractIdentity(c)

	userChoice := ""
	if userID != uuid.Nil {
		vote, _ := h.service.GetUserVote(appID, userID, challenge.ID)
		if vote != nil {
			userChoice = vote.Choice
		}
	} else if guestID != "" {
		vote, _ := h.service.GetGuestVote(appID, guestID, challenge.ID)
		if vote != nil {
			userChoice = vote.Choice
		}
	}

	total := challenge.VotesA + challenge.VotesB
	percentA, percentB := 0, 0
	if total > 0 {
		percentA = (challenge.VotesA * 100) / total
		percentB = (challenge.VotesB * 100) / total
	}

	return c.JSON(fiber.Map{
		"challenge": challenge, "user_choice": userChoice,
		"user_voted": userChoice != "",
		"percent_a": percentA, "percent_b": percentB,
		"total_votes": total,
	})
}

// Vote handles POST /api/challenges/vote
func (h *ChallengeHandler) Vote(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, guestID := extractIdentity(c)

	if userID == uuid.Nil && guestID == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Authentication required. Sign up or use guest mode.",
		})
	}

	var req struct {
		ChallengeID string `json:"challenge_id"`
		Choice      string `json:"choice"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	challengeID, err := uuid.Parse(req.ChallengeID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid challenge ID",
		})
	}

	vote, err := h.service.Vote(appID, userID, guestID, challengeID, req.Choice)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(vote)
}

// GetStats handles GET /api/challenges/stats (requires authenticated user)
func (h *ChallengeHandler) GetStats(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	stats, err := h.service.GetStats(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to get stats",
		})
	}

	return c.JSON(stats)
}

// GetHistory handles GET /api/challenges/history (requires authenticated user)
func (h *ChallengeHandler) GetHistory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, err := tenant.GetUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": true, "message": "Unauthorized",
		})
	}

	history, err := h.service.GetChallengeHistory(appID, userID, 20)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to get history",
		})
	}

	return c.JSON(fiber.Map{"data": history})
}

// GetRandom handles GET /api/challenges/random
func (h *ChallengeHandler) GetRandom(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	userID, _ := extractIdentity(c)

	challenge, err := h.service.GetRandomChallenge(appID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "No challenges available",
		})
	}

	total := challenge.VotesA + challenge.VotesB
	percentA, percentB := 0, 0
	if total > 0 {
		percentA = (challenge.VotesA * 100) / total
		percentB = (challenge.VotesB * 100) / total
	}

	return c.JSON(fiber.Map{
		"challenge": challenge, "user_choice": "",
		"percent_a": percentA, "percent_b": percentB,
		"total_votes": total,
	})
}

// GetByCategory handles GET /api/challenges/category/:category
func (h *ChallengeHandler) GetByCategory(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	category := c.Params("category")
	if category == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Category is required",
		})
	}

	userID, _ := extractIdentity(c)

	challenges, err := h.service.GetChallengesByCategory(appID, category, userID, 20)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to get challenges",
		})
	}

	return c.JSON(fiber.Map{"data": challenges, "total": len(challenges)})
}

// GenerateQuestions handles POST /api/admin/challenges/generate (admin only)
func (h *ChallengeHandler) GenerateQuestions(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)

	var req struct {
		Category string `json:"category"`
		Count    int    `json:"count"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Invalid request body",
		})
	}

	if req.Category == "" {
		req.Category = "general"
	}
	if req.Count <= 0 {
		req.Count = 10
	}
	if req.Count > 50 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, "message": "Count cannot exceed 50",
		})
	}

	if !h.questionGenerator.IsAvailable() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "message": "AI question generation is not configured",
			"hint":  "Set GLM_API_KEY environment variable",
		})
	}

	challenges, err := h.questionGenerator.GenerateBatch(appID, req.Category, req.Count)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to generate questions",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Questions generated successfully",
		"count":   len(challenges),
		"data":    challenges,
	})
}

// GenerateAllCategories handles POST /api/admin/challenges/generate-all
func (h *ChallengeHandler) GenerateAllCategories(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)

	countStr := c.Query("count", "10")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		count = 10
	}
	if count > 20 {
		count = 20
	}

	if !h.questionGenerator.IsAvailable() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, "message": "AI question generation is not configured",
		})
	}

	results, err := h.questionGenerator.GenerateForAllCategories(appID, count)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, "message": "Failed to generate questions",
			"details": err.Error(),
		})
	}

	totalCount := 0
	categoryCounts := make(map[string]int)
	for cat, challenges := range results {
		cnt := len(challenges)
		categoryCounts[cat] = cnt
		totalCount += cnt
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":         "Questions generated for all categories",
		"total_count":     totalCount,
		"category_counts": categoryCounts,
		"data":            results,
	})
}

// extractIdentity gets userID and guestID from context.
func extractIdentity(c *fiber.Ctx) (uuid.UUID, string) {
	userID := uuid.Nil
	guestID := ""

	if uid, ok := c.Locals("userID").(uuid.UUID); ok {
		userID = uid
	} else if token, ok := c.Locals("user").(*jwt.Token); ok {
		claims := token.Claims.(jwt.MapClaims)
		if sub, ok := claims["sub"].(string); ok {
			userID, _ = uuid.Parse(sub)
		}
	}

	if gid, ok := c.Locals("guestID").(string); ok {
		guestID = gid
	}

	return userID, guestID
}
