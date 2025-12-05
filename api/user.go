package api

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func getCurUser(c *fiber.Ctx) *model.UserModel {
	if c.Locals("user") == nil {
		return nil
	}
	return c.Locals("user").(*model.UserModel)
}

func validateCaptchaForScene(scene utils.CaptchaScene, captchaId, captchaValue, token, remoteIP string) (bool, string) {
	conf := appConfig.Captcha.Target(scene)
	switch conf.Mode {
	case utils.CaptchaModeLocal:
		if !model.CaptchaVerify(captchaId, captchaValue) {
			return false, "验证码错误或已过期"
		}
	case utils.CaptchaModeTurnstile:
		trimmed := strings.TrimSpace(token)
		if trimmed == "" {
			return false, "请完成人机验证"
		}
		ok, err := model.TurnstileVerify(trimmed, conf.Turnstile.SecretKey, remoteIP)
		if err != nil {
			log.Printf("Turnstile verify failed: %v", err)
		}
		if !ok {
			return false, "人机验证失败"
		}
	}
	return true, ""
}

// UserSignup 注册接口
func UserSignup(c *fiber.Ctx) error {
	type RequestBody struct {
		Username       string `json:"username" form:"username" binding:"required"`
		Password       string `json:"password" form:"password" binding:"required"`
		Nickname       string `json:"nickname" form:"nickname" binding:"required"`
		CaptchaId      string `json:"captchaId" form:"captchaId"`
		CaptchaValue   string `json:"captchaValue" form:"captchaValue"`
		TurnstileToken string `json:"turnstileToken" form:"turnstileToken"`
	}

	var requestBody RequestBody
	if err := c.BodyParser(&requestBody); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}

	username := requestBody.Username
	password := requestBody.Password

	if username == "" || password == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "用户名或密码不能为空",
		})
	}

	if len(username) < 2 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "用户名长度不能小于2位",
		})
	}

	if ok, _ := regexp.MatchString(`^[A-Za-z0-9_.-]+$`, username); !ok {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "用户名只能包含字母、数字、下划线、点或中划线",
		})
	}

	if len(password) < 3 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "密码长度不能小于3位",
		})
	}

	requestBody.Nickname = strings.TrimSpace(requestBody.Nickname)
	if requestBody.Nickname == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "昵称不能为空",
		})
	}

	if ok, msg := validateCaptchaForScene(utils.CaptchaSceneSignup, requestBody.CaptchaId, requestBody.CaptchaValue, requestBody.TurnstileToken, c.IP()); !ok {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": msg,
		})
	}

	count := model.UserCount()

	user, err := model.UserCreate(username, password, requestBody.Nickname)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	if count == 0 {
		// 首个用户，设置为管理员
		_, _ = service.UserRoleLink([]string{"sys-admin"}, []string{user.ID})
		if _, err := service.BootstrapDefaultWorldForOwner(user.ID); err != nil {
			log.Printf("初始化默认世界失败: %v", err)
		}
	} else {
		_, _ = service.UserRoleLink([]string{"sys-user"}, []string{user.ID})
		if world, err := service.GetOrCreateDefaultWorld(); err == nil {
			_, _ = service.WorldJoin(world.ID, user.ID, model.WorldRoleMember)
		}
	}

	token, err := model.UserGenerateAccessToken(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "生成token失败",
		})
	}

	model.TimelineUpdate(user.ID)

	return c.JSON(fiber.Map{
		"message": "注册成功",
		"token":   token,
	})
}

// 登录接口
func UserSignin(c *fiber.Ctx) error {
	type RequestBody struct {
		Username       string `json:"username" form:"username" binding:"required"`
		Password       string `json:"password" form:"password" binding:"required"`
		CaptchaId      string `json:"captchaId" form:"captchaId"`
		CaptchaValue   string `json:"captchaValue" form:"captchaValue"`
		TurnstileToken string `json:"turnstileToken" form:"turnstileToken"`
	}

	var requestBody RequestBody
	if err := c.BodyParser(&requestBody); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}

	username := requestBody.Username
	password := requestBody.Password
	if username == "" || password == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "用户名或密码不能为空",
		})
	}

	if len(password) < 3 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "密码长度不能小于3位",
		})
	}

	if ok, msg := validateCaptchaForScene(utils.CaptchaSceneSignin, requestBody.CaptchaId, requestBody.CaptchaValue, requestBody.TurnstileToken, c.IP()); !ok {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": msg,
		})
	}

	user, err := model.UserAuthenticate(username, password)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}
	token, err := model.UserGenerateAccessToken(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "生成token失败",
		})
	}
	return c.JSON(fiber.Map{
		"message": "登录成功",
		"token":   token,
	})
}

// UserChangePassword 修改密码接口
func UserChangePassword(c *fiber.Ctx) error {
	tokenString := c.Get("Authorization")
	if tokenString == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "未提供token",
		})
	}
	user, err := model.UserVerifyAccessToken(tokenString)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	var formData struct {
		Password    string `form:"password" json:"password" binding:"required"`
		PasswordNew string `form:"passwordNew" json:"passwordNew" binding:"required"`
	}
	if err := c.BodyParser(&formData); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}

	oldPassword := formData.Password
	newPassword := formData.PasswordNew

	if oldPassword == "" || newPassword == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "旧密码或新密码不能为空",
		})
	}
	if _, err := model.UserAuthenticate(user.Username, oldPassword); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "旧密码错误",
		})
	}
	if err := model.UserUpdatePassword(user.ID, newPassword); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "修改密码失败",
		})
	}

	err = model.AcessTokenDeleteAllByUserID(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "删除用户凭证失败",
		})
	}

	token, err := model.UserGenerateAccessToken(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "生成token失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "修改密码成功",
		"token":   token,
	})
}

func UserInfo(c *fiber.Ctx) error {
	u := getCurUser(c)
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"user":    u,
		"permSys": pm.GetAllSysPermByUid(u.ID),
	})
}

func UserInfoUpdate(c *fiber.Ctx) error {
	type RequestBody struct {
		Nickname string `json:"nick" form:"nick"`
		Brief    string `json:"brief" form:"brief"`
	}

	var data RequestBody
	err := c.BodyParser(&data)
	if err != nil {
		return err
	}
	data.Nickname = strings.TrimSpace(data.Nickname)
	if len(data.Nickname) > 20 {
		return c.JSON(fiber.Map{
			"message": "昵称不能超过20个字符",
		})
	}
	if len(data.Nickname) < 1 {
		return c.JSON(fiber.Map{
			"message": "昵称不能为空",
		})
	}
	if m, _ := regexp.MatchString(`\s`, data.Nickname); m {
		c.Status(http.StatusBadRequest)
		return c.JSON(fiber.Map{
			"message": "昵称不能包含空格",
		})
	}

	u := getCurUser(c)
	db := model.GetDB()
	u2 := &model.UserModel{}
	db.Select("id").Where("nickname = ? and id != ?", data.Nickname, u.ID).First(&u2)
	if u2.ID != "" {
		c.Status(http.StatusBadRequest)
		return c.JSON(fiber.Map{
			"message": "昵称已被占用",
		})
	}

	u.Nickname = data.Nickname
	u.Brief = data.Brief
	u.SaveInfo()

	return c.JSON(fiber.Map{
		"user": u,
	})
}
