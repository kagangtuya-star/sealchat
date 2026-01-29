package api

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func isValidEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	localPart := parts[0]
	domain := parts[1]
	if len(localPart) == 0 || len(localPart) > 64 {
		return false
	}
	if len(domain) == 0 || len(domain) > 255 {
		return false
	}
	return emailRegex.MatchString(email)
}

func getClientIP(c *fiber.Ctx) string {
	ip := c.Get("X-Real-IP")
	if ip == "" {
		ip = c.Get("X-Forwarded-For")
		if ip != "" {
			ip = strings.Split(ip, ",")[0]
		}
	}
	if ip == "" {
		ip = c.IP()
	}
	return strings.TrimSpace(ip)
}

func EmailAuthSignupCodeSend(c *fiber.Ctx) error {
	var req struct {
		Email          string `json:"email"`
		CaptchaId      string `json:"captchaId"`
		CaptchaValue   string `json:"captchaValue"`
		TurnstileToken string `json:"turnstileToken"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !isValidEmail(req.Email) {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "邮箱格式无效"})
	}

	cfg := utils.GetConfig()
	captchaCfg := cfg.Captcha.Target(utils.CaptchaSceneSignup)
	if err := verifyCaptchaByMode(captchaCfg, req.CaptchaId, req.CaptchaValue, req.TurnstileToken); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	if err := svc.SendSignupCode(req.Email, getClientIP(c), c.Get("User-Agent")); err != nil {
		if err == service.ErrEmailAlreadyUsed {
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "该邮箱已被注册"})
		}
		if err == model.ErrEmailRateLimited {
			return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{"error": "发送频率过高，请稍后再试"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "发送失败，请稍后再试"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "验证码已发送"})
}

// maskEmail 脱敏邮箱地址，例如 test@example.com -> te**@example.com
func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	domain := parts[1]
	if len(local) <= 2 {
		return local[:1] + "**@" + domain
	}
	return local[:2] + strings.Repeat("*", len(local)-2) + "@" + domain
}

// EmailAuthPasswordResetVerify 验证身份（步骤1）：验证验证码，返回用户名和脱敏邮箱
func EmailAuthPasswordResetVerify(c *fiber.Ctx) error {
	var req struct {
		Account        string `json:"account"`
		CaptchaId      string `json:"captchaId"`
		CaptchaValue   string `json:"captchaValue"`
		TurnstileToken string `json:"turnstileToken"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Account = strings.TrimSpace(req.Account)
	if req.Account == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请输入用户名或邮箱"})
	}

	cfg := utils.GetConfig()
	captchaCfg := cfg.Captcha.Target(utils.CaptchaScenePasswordReset)
	if captchaCfg.Mode != utils.CaptchaModeOff {
		if err := verifyCaptchaByMode(captchaCfg, req.CaptchaId, req.CaptchaValue, req.TurnstileToken); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	user, err := model.UserGetByEmailOrUsername(req.Account)
	if err != nil || user == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "用户不存在"})
	}

	userEmail := ""
	if user.Email != nil {
		userEmail = *user.Email
	}
	if userEmail == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "该账户未绑定邮箱，请联系管理员"})
	}

	// 判断用户是否直接输入了邮箱
	inputIsEmail := isValidEmail(req.Account) && strings.EqualFold(req.Account, userEmail)

	// 根据输入类型决定是否显示完整邮箱和是否需要补全
	maskedEmail := maskEmail(userEmail)
	needEmailConfirm := true // 默认需要补全邮箱
	if inputIsEmail {
		// 如果输入的是邮箱且匹配，显示完整邮箱，不需要补全
		maskedEmail = userEmail
		needEmailConfirm = false
	}

	return c.JSON(fiber.Map{
		"success":          true,
		"username":         user.Username,
		"email":            userEmail,        // 完整邮箱，用于后续发送验证码
		"maskedEmail":      maskedEmail,      // 脱敏邮箱，用于前端显示
		"needEmailConfirm": needEmailConfirm, // 是否需要补全邮箱
	})
}

func EmailAuthPasswordResetRequest(c *fiber.Ctx) error {
	var req struct {
		Account        string `json:"account"`
		CaptchaId      string `json:"captchaId"`
		CaptchaValue   string `json:"captchaValue"`
		TurnstileToken string `json:"turnstileToken"`
		Verified       bool   `json:"verified"` // 是否已通过身份验证（步骤1）
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Account = strings.TrimSpace(req.Account)
	if req.Account == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请输入用户名或邮箱"})
	}

	// 如果未通过身份验证（旧流程或直接调用），需要验证验证码
	if !req.Verified {
		cfg := utils.GetConfig()
		captchaCfg := cfg.Captcha.Target(utils.CaptchaScenePasswordReset)
		if captchaCfg.Mode != utils.CaptchaModeOff {
			if err := verifyCaptchaByMode(captchaCfg, req.CaptchaId, req.CaptchaValue, req.TurnstileToken); err != nil {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
		}
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	_ = svc.SendPasswordResetCode(req.Account, getClientIP(c), c.Get("User-Agent"))

	return c.JSON(fiber.Map{"success": true, "message": "如果该账户存在且绑定了邮箱，验证码已发送"})
}

func EmailAuthPasswordResetConfirm(c *fiber.Ctx) error {
	var req struct {
		Account     string `json:"account"`
		Code        string `json:"code"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Account = strings.TrimSpace(req.Account)
	req.Code = strings.TrimSpace(req.Code)

	if req.Account == "" || req.Code == "" || req.NewPassword == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少必要参数"})
	}

	if len(req.NewPassword) < 6 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "密码长度至少为 6 位"})
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	if err := svc.VerifyAndResetPassword(req.Account, req.Code, req.NewPassword); err != nil {
		switch err {
		case model.ErrCodeNotFound, model.ErrCodeInvalid:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码无效"})
		case model.ErrCodeExpired:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码已过期"})
		case model.ErrCodeMaxAttempts:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码尝试次数过多"})
		case service.ErrUserNotFound:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "用户不存在"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "重置失败，请稍后再试"})
		}
	}

	return c.JSON(fiber.Map{"success": true, "message": "密码重置成功"})
}

func EmailAuthBindCodeSend(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未登录"})
	}

	var req struct {
		Email          string `json:"email"`
		CaptchaId      string `json:"captchaId"`
		CaptchaValue   string `json:"captchaValue"`
		TurnstileToken string `json:"turnstileToken"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !isValidEmail(req.Email) {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "邮箱格式无效"})
	}

	cfg := utils.GetConfig()
	captchaCfg := cfg.Captcha.Target(utils.CaptchaSceneSignup)
	if err := verifyCaptchaByMode(captchaCfg, req.CaptchaId, req.CaptchaValue, req.TurnstileToken); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	if err := svc.SendBindCode(user.ID, req.Email, getClientIP(c), c.Get("User-Agent")); err != nil {
		if err == service.ErrEmailAlreadyUsed {
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "该邮箱已被使用"})
		}
		if err == model.ErrEmailRateLimited {
			return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{"error": "发送频率过高，请稍后再试"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "发送失败，请稍后再试"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "验证码已发送"})
}

func EmailAuthBindConfirm(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未登录"})
	}

	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	if req.Email == "" || req.Code == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少必要参数"})
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	if err := svc.VerifyAndBindEmail(user.ID, req.Email, req.Code); err != nil {
		switch err {
		case model.ErrCodeNotFound, model.ErrCodeInvalid:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码无效"})
		case model.ErrCodeExpired:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码已过期"})
		case model.ErrCodeMaxAttempts:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码尝试次数过多"})
		case service.ErrEmailAlreadyUsed, model.ErrEmailAlreadyUsed:
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "该邮箱已被使用"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "绑定失败，请稍后再试"})
		}
	}

	return c.JSON(fiber.Map{"success": true, "message": "邮箱绑定成功"})
}

func EmailAuthSignupWithCode(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
		Code     string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	req.Nickname = strings.TrimSpace(req.Nickname)

	if req.Username == "" || req.Password == "" || req.Email == "" || req.Code == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少必要参数"})
	}

	if len(req.Password) < 6 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "密码长度至少为 6 位"})
	}

	cfg := utils.GetConfig()
	if !cfg.RegisterOpen {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "注册已关闭"})
	}

	svc := service.NewEmailAuthService()
	if svc == nil || !svc.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"error": "邮箱认证功能未启用"})
	}

	if err := svc.VerifySignupCode(req.Email, req.Code); err != nil {
		switch err {
		case model.ErrCodeNotFound, model.ErrCodeInvalid:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码无效"})
		case model.ErrCodeExpired:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码已过期"})
		case model.ErrCodeMaxAttempts:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "验证码尝试次数过多"})
		default:
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "验证失败，请稍后再试"})
		}
	}

	if model.UserExistsByUsername(req.Username) {
		return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "用户名已存在"})
	}

	exists, err := model.UserExistsByEmail(req.Email)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "邮箱检查失败，请稍后再试"})
	}
	if exists {
		return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "该邮箱已被注册"})
	}

	nickname := req.Nickname
	if nickname == "" {
		nickname = req.Username
	}

	count := model.UserCount()
	user, err := model.UserCreateWithEmail(req.Username, req.Password, nickname, req.Email)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "注册失败，请稍后再试"})
	}

	if count == 0 {
		// 首个用户，设置为管理员
		_, _ = service.UserRoleLink([]string{"sys-admin"}, []string{user.ID})
		user.RoleIds = []string{"sys-admin"}
		if _, err := service.BootstrapDefaultWorldForOwner(user.ID); err != nil {
			log.Printf("初始化默认世界失败: %v", err)
		}
	} else {
		_, _ = service.UserRoleLink([]string{"sys-user"}, []string{user.ID})
		user.RoleIds = []string{"sys-user"}
		if world, err := service.GetOrCreateDefaultWorld(); err == nil {
			_, _ = service.WorldJoin(world.ID, user.ID, model.WorldRoleMember)
		}
	}

	token, err := model.UserGenerateAccessToken(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "生成令牌失败"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"user":    user,
		"token":   token,
	})
}

func verifyCaptchaByMode(cfg utils.CaptchaTargetConfig, captchaId, captchaValue, turnstileToken string) error {
	switch cfg.Mode {
	case utils.CaptchaModeLocal:
		if captchaId == "" || captchaValue == "" {
			return fiber.NewError(http.StatusBadRequest, "请完成验证码验证")
		}
		if !model.CaptchaVerify(captchaId, captchaValue) {
			return fiber.NewError(http.StatusBadRequest, "验证码错误")
		}
	case utils.CaptchaModeTurnstile:
		if turnstileToken == "" {
			return fiber.NewError(http.StatusBadRequest, "请完成人机验证")
		}
		ok, _ := model.TurnstileVerify(turnstileToken, cfg.Turnstile.SecretKey, "")
		if !ok {
			return fiber.NewError(http.StatusBadRequest, "人机验证失败")
		}
	}
	return nil
}
