package api

import (
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sealchat/service"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
)

var (
	usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	passwordPattern = regexp.MustCompile(`[A-Za-z]`)
	digitPattern    = regexp.MustCompile(`[0-9]`)
)

func AdminUserList(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return nil
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))
	keyword := c.Query("keyword", "")
	userType := c.Query("type", "") // "bot", "user", "" (all)

	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := model.GetDB()
	var total int64
	query := db.Model(&model.UserModel{})

	// 搜索过滤
	if keyword != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%")
	}

	// 用户类型过滤
	if userType == "bot" {
		query = query.Where("is_bot = ?", true)
	} else if userType == "user" {
		query = query.Where("is_bot = ?", false)
	}

	query.Count(&total)

	// 获取列表
	var items []*model.UserModel
	offset := (page - 1) * pageSize
	query.Order("created_at desc").
		Offset(offset).Limit(pageSize).
		Find(&items)

	for _, i := range items {
		i.RoleIds, _ = model.UserRoleMappingListByUserID(i.ID, "", "system")
	}

	// 返回JSON响应
	return c.JSON(fiber.Map{
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
		"items":    items,
	})
}

func AdminUserDisable(c *fiber.Ctx) error {
	userId := c.Query("id")
	if userId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID不能为空",
		})
	}

	err := model.UserSetDisable(userId, true)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "禁用用户失败",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "用户已成功禁用",
	})
}

func AdminUserEnable(c *fiber.Ctx) error {
	userId := c.Query("id")
	if userId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID不能为空",
		})
	}

	err := model.UserSetDisable(userId, false)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "启用用户失败",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "用户已成功启用",
	})
}

func AdminUserResetPassword(c *fiber.Ctx) error {
	uid := c.Query("id")
	if uid == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "参数错误",
		})
	}

	err := model.UserUpdatePassword(uid, "123456")
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "重置密码失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "禁用成功",
	})
}

func AdminUserRoleLinkByUserId(c *fiber.Ctx) error {
	type RequestBody struct {
		UserId  string   `json:"userId"`
		RoleIds []string `json:"roleIds"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		// 处理解析错误
		return err
	}

	if body.UserId == "" || len(body.RoleIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID和角色ID不能为空",
		})
	}

	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return nil
	}

	_, err := service.UserRoleLink(body.RoleIds, []string{body.UserId})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "添加用户角色失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "用户角色已添加",
	})
}

func AdminUserRoleUnlinkByUserId(c *fiber.Ctx) error {
	type RequestBody struct {
		UserId  string   `json:"userId"`
		RoleIds []string `json:"roleIds"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		// 处理解析错误
		return err
	}

	if body.UserId == "" || len(body.RoleIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID和角色ID不能为空",
		})
	}

	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return nil
	}

	_, err := service.UserRoleUnlink(body.RoleIds, []string{body.UserId})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "删除用户角色失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "用户角色已成功删除",
	})
}

func validateUserInput(username, nickname, password string) error {
	username = strings.TrimSpace(username)
	nickname = strings.TrimSpace(nickname)

	if username == "" {
		return errors.New("用户名不能为空")
	}
	if len(username) < 2 {
		return errors.New("用户名长度不能小于2位")
	}
	if len(username) > 32 {
		return errors.New("用户名长度不能超过32位")
	}
	if !usernamePattern.MatchString(username) {
		return errors.New("用户名只能包含字母、数字、下划线、点或中划线")
	}

	if nickname == "" {
		return errors.New("昵称不能为空")
	}
	if len(nickname) > 20 {
		return errors.New("昵称不能超过20个字符")
	}
	if strings.ContainsAny(nickname, " \t\n\r") {
		return errors.New("昵称不能包含空格")
	}

	if password == "" {
		return errors.New("密码不能为空")
	}
	if len(password) < 6 {
		return errors.New("密码长度至少6位")
	}
	if !passwordPattern.MatchString(password) || !digitPattern.MatchString(password) {
		return errors.New("密码必须包含字母和数字")
	}

	return nil
}

func AdminUserCreate(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "权限不足",
		})
	}

	type RequestBody struct {
		Username string   `json:"username"`
		Nickname string   `json:"nickname"`
		Password string   `json:"password"`
		RoleIds  []string `json:"roleIds"`
		Disabled bool     `json:"disabled"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}

	body.Username = strings.TrimSpace(body.Username)
	body.Nickname = strings.TrimSpace(body.Nickname)

	if err := validateUserInput(body.Username, body.Nickname, body.Password); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	if model.UserExistsByUsername(body.Username) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "用户名已存在",
		})
	}

	user, err := model.UserCreate(body.Username, body.Password, body.Nickname)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "创建用户失败: " + err.Error(),
		})
	}

	if body.Disabled {
		_ = model.UserSetDisable(user.ID, true)
		user.Disabled = true
	}

	roleIds := body.RoleIds
	if len(roleIds) == 0 {
		roleIds = []string{"sys-user"}
	}
	_, _ = service.UserRoleLink(roleIds, []string{user.ID})
	user.RoleIds = roleIds

	// 加入默认公共世界
	if world, err := service.GetOrCreateDefaultWorld(); err == nil {
		_, _ = service.WorldJoin(world.ID, user.ID, model.WorldRoleMember)
	}

	return c.JSON(fiber.Map{
		"message": "用户创建成功",
		"user":    user,
	})
}

func AdminCheckUsername(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "权限不足",
		})
	}

	username := strings.TrimSpace(c.Query("username"))
	if username == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message":   "用户名不能为空",
			"available": false,
		})
	}

	available := !model.UserExistsByUsername(username)
	return c.JSON(fiber.Map{
		"available": available,
	})
}

func AdminUserImportTemplate(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "权限不足",
		})
	}

	c.Set("Content-Type", "text/csv; charset=utf-8")
	c.Set("Content-Disposition", "attachment; filename=user_import_template.csv")

	template := "\xEF\xBB\xBFusername,nickname,password\nexample_user,示例用户,Password123\n"
	return c.SendString(template)
}

type BatchImportError struct {
	Row      int    `json:"row"`
	Username string `json:"username"`
	Error    string `json:"error"`
}

func AdminUserBatchCreate(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "权限不足",
		})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "请上传CSV文件",
		})
	}

	const maxFileSize = 2 * 1024 * 1024
	if file.Size > maxFileSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "文件大小不能超过2MB",
		})
	}

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "无法打开文件",
		})
	}
	defer f.Close()

	reader := csv.NewReader(bufio.NewReader(f))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "无法读取CSV文件",
		})
	}

	colMap := make(map[string]int)
	for i, col := range header {
		col = strings.TrimSpace(col)
		col = strings.TrimPrefix(col, "\ufeff")
		col = strings.ToLower(col)
		colMap[col] = i
	}

	usernameIdx, ok1 := colMap["username"]
	nicknameIdx, ok2 := colMap["nickname"]
	passwordIdx, ok3 := colMap["password"]
	if !ok1 || !ok2 || !ok3 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "CSV格式错误，必须包含 username, nickname, password 列",
		})
	}

	type userRow struct {
		Row      int
		Username string
		Nickname string
		Password string
	}
	var rows []userRow
	var importErrors []BatchImportError
	rowNum := 1
	totalRows := 0

	const maxRows = 500
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if err != nil {
			importErrors = append(importErrors, BatchImportError{
				Row:   rowNum,
				Error: "解析错误: " + err.Error(),
			})
			continue
		}

		username := ""
		nickname := ""
		password := ""
		if usernameIdx < len(record) {
			username = strings.TrimSpace(record[usernameIdx])
		}
		if nicknameIdx < len(record) {
			nickname = strings.TrimSpace(record[nicknameIdx])
		}
		if passwordIdx < len(record) {
			password = strings.TrimSpace(record[passwordIdx])
		}

		if username == "" && nickname == "" && password == "" {
			continue
		}

		totalRows++
		if totalRows > maxRows {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "行数超过限制，最多支持500行",
			})
		}

		if err := validateUserInput(username, nickname, password); err != nil {
			importErrors = append(importErrors, BatchImportError{
				Row:      rowNum,
				Username: username,
				Error:    err.Error(),
			})
			continue
		}

		rows = append(rows, userRow{
			Row:      rowNum,
			Username: username,
			Nickname: nickname,
			Password: password,
		})
	}

	if len(rows) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "没有有效的用户数据",
			"errors":  importErrors,
		})
	}

	seen := make(map[string]int)
	var validRows []userRow
	for _, r := range rows {
		if prevRow, exists := seen[r.Username]; exists {
			importErrors = append(importErrors, BatchImportError{
				Row:      r.Row,
				Username: r.Username,
				Error:    "用户名与第" + strconv.Itoa(prevRow) + "行重复",
			})
			continue
		}
		seen[r.Username] = r.Row
		validRows = append(validRows, r)
	}

	usernames := make([]string, len(validRows))
	for i, r := range validRows {
		usernames[i] = r.Username
	}
	existingMap := model.UserExistsByUsernames(usernames)

	var toCreate []userRow
	for _, r := range validRows {
		if existingMap[r.Username] {
			importErrors = append(importErrors, BatchImportError{
				Row:      r.Row,
				Username: r.Username,
				Error:    "用户名已存在",
			})
			continue
		}
		toCreate = append(toCreate, r)
	}

	createdCount := 0
	// 预先获取默认世界
	defaultWorld, _ := service.GetOrCreateDefaultWorld()
	for _, r := range toCreate {
		user, err := model.UserCreate(r.Username, r.Password, r.Nickname)
		if err != nil {
			importErrors = append(importErrors, BatchImportError{
				Row:      r.Row,
				Username: r.Username,
				Error:    "创建失败: " + err.Error(),
			})
			continue
		}
		if _, err := service.UserRoleLink([]string{"sys-user"}, []string{user.ID}); err != nil {
			importErrors = append(importErrors, BatchImportError{
				Row:      r.Row,
				Username: r.Username,
				Error:    "创建后绑定角色失败: " + err.Error(),
			})
			continue
		}
		// 加入默认公共世界
		if defaultWorld != nil {
			if _, err := service.WorldJoin(defaultWorld.ID, user.ID, model.WorldRoleMember); err != nil {
				importErrors = append(importErrors, BatchImportError{
					Row:      r.Row,
					Username: r.Username,
					Error:    "创建后加入默认世界失败: " + err.Error(),
				})
				continue
			}
		}
		createdCount++
	}

	success := len(importErrors) == 0
	message := "批量导入完成"
	if !success {
		message = "批量导入完成，部分失败"
	}

	return c.JSON(fiber.Map{
		"success": success,
		"message": message,
		"stats": fiber.Map{
			"total":   totalRows,
			"created": createdCount,
			"failed":  len(importErrors),
		},
		"errors": importErrors,
	})
}
