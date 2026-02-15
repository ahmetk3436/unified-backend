package handlers

import (
	"github.com/ahmetcoskunkizilkaya/unified-backend/internal/tenant"
	"github.com/gofiber/fiber/v2"
)

type LegalHandler struct {
	registry *tenant.Registry
}

func NewLegalHandler(registry *tenant.Registry) *LegalHandler {
	return &LegalHandler{registry: registry}
}

func (h *LegalHandler) PrivacyPolicy(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	appCfg := h.registry.Get(appID)
	appName := "Our App"
	if appCfg != nil {
		appName = appCfg.AppName
	}

	return c.Type("html").SendString(`<!DOCTYPE html>
<html><head><title>Privacy Policy - ` + appName + `</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;max-width:800px;margin:0 auto;padding:20px;color:#333}h1{color:#1a1a1a}h2{color:#444;margin-top:30px}</style>
</head><body>
<h1>Privacy Policy</h1>
<p>Last updated: February 2026</p>
<h2>Information We Collect</h2>
<p>We collect your email address and app usage data to provide our services. If you sign in with Apple, we receive your Apple ID identifier.</p>
<h2>How We Use Your Information</h2>
<p>Your data is used solely to operate ` + appName + `, authenticate your account, and improve our services.</p>
<h2>Data Storage</h2>
<p>Your data is stored securely on encrypted servers. We do not sell your personal information to third parties.</p>
<h2>Account Deletion</h2>
<p>You can delete your account and all associated data at any time from the app settings.</p>
<h2>Contact</h2>
<p>For questions about this policy, contact us at support@` + appID + `.app</p>
</body></html>`)
}

func (h *LegalHandler) TermsOfService(c *fiber.Ctx) error {
	appID := tenant.GetAppID(c)
	appCfg := h.registry.Get(appID)
	appName := "Our App"
	if appCfg != nil {
		appName = appCfg.AppName
	}

	return c.Type("html").SendString(`<!DOCTYPE html>
<html><head><title>Terms of Service - ` + appName + `</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>body{font-family:-apple-system,BlinkMacSystemFont,sans-serif;max-width:800px;margin:0 auto;padding:20px;color:#333}h1{color:#1a1a1a}h2{color:#444;margin-top:30px}</style>
</head><body>
<h1>Terms of Service</h1>
<p>Last updated: February 2026</p>
<h2>Acceptance</h2>
<p>By using ` + appName + `, you agree to these terms.</p>
<h2>User Conduct</h2>
<p>You agree not to post offensive, illegal, or harmful content. We reserve the right to moderate and remove content that violates our guidelines.</p>
<h2>Subscriptions</h2>
<p>Premium features require an active subscription managed through the App Store. Subscriptions auto-renew unless cancelled 24 hours before the end of the current period.</p>
<h2>Termination</h2>
<p>We may suspend or terminate accounts that violate these terms.</p>
<h2>Contact</h2>
<p>For questions, contact us at support@` + appID + `.app</p>
</body></html>`)
}
