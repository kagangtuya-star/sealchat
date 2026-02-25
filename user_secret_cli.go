package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"sealchat/model"
)

const defaultResetSecret = "123456"

func handleUserSecret(action string, usernames []string, adminOnly bool, yes bool) error {
	if err := ensureUserSecretCLITables(); err != nil {
		return err
	}

	switch action {
	case "list":
		return handleUserSecretList()
	case "reset":
		return handleUserSecretReset(usernames, adminOnly, yes)
	default:
		return fmt.Errorf("不支持的 user-secret 动作: %s", action)
	}
}

func ensureUserSecretCLITables() error {
	db := model.GetDB()
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	return db.AutoMigrate(&model.UserModel{}, &model.UserRoleMappingModel{})
}

func handleUserSecretList() error {
	adminIDs, err := model.UserRoleMappingUserIdListByRoleId("sys-admin")
	if err != nil {
		return err
	}

	if len(adminIDs) == 0 {
		fmt.Println("未找到平台管理员用户")
		return nil
	}

	var users []model.UserModel
	if err := model.GetDB().
		Where("id IN ?", adminIDs).
		Order("username ASC").
		Find(&users).Error; err != nil {
		return err
	}

	if len(users) == 0 {
		fmt.Println("未找到平台管理员用户（角色映射存在但用户不存在）")
		return nil
	}

	fmt.Println("平台管理员用户列表：")
	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Printf("%-24s %-20s %-10s %-8s\n", "ID", "用户名", "禁用", "机器人")
	fmt.Println("────────────────────────────────────────────────────────────")
	for _, user := range users {
		fmt.Printf("%-24s %-20s %-10v %-8v\n", user.ID, user.Username, user.Disabled, user.IsBot)
	}
	fmt.Println("────────────────────────────────────────────────────────────")
	fmt.Printf("共 %d 个平台管理员\n", len(users))
	return nil
}

func handleUserSecretReset(usernames []string, adminOnly bool, yes bool) error {
	targetNames := normalizeUsernames(usernames)
	if len(targetNames) == 0 {
		return fmt.Errorf("reset 动作需要至少一个 --username")
	}

	var users []model.UserModel
	if err := model.GetDB().
		Where("username IN ?", targetNames).
		Find(&users).Error; err != nil {
		return err
	}

	userByName := make(map[string]*model.UserModel, len(users))
	for i := range users {
		userByName[users[i].Username] = &users[i]
	}

	var missing []string
	for _, name := range targetNames {
		if _, ok := userByName[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("以下用户名不存在: %s", strings.Join(missing, ", "))
	}

	adminSet, err := loadAdminUserIDSet()
	if err != nil {
		return err
	}

	if adminOnly {
		var nonAdmin []string
		for _, name := range targetNames {
			u := userByName[name]
			if _, ok := adminSet[u.ID]; !ok {
				nonAdmin = append(nonAdmin, name)
			}
		}
		if len(nonAdmin) > 0 {
			sort.Strings(nonAdmin)
			return fmt.Errorf("--admin-only 已启用，以下用户不是平台管理员: %s", strings.Join(nonAdmin, ", "))
		}
	}

	if !yes {
		fmt.Println("即将重置以下用户的密码为 123456：")
		for _, name := range targetNames {
			u := userByName[name]
			roleType := "普通用户"
			if _, ok := adminSet[u.ID]; ok {
				roleType = "平台管理员"
			}
			fmt.Printf("- %s (%s)\n", u.Username, roleType)
		}
		fmt.Print("确认执行？(y/N): ")
		if !readConfirmYes() {
			fmt.Println("已取消")
			return nil
		}
	}

	var okCount int
	var failed []string
	for _, name := range targetNames {
		u := userByName[name]
		if err := model.UserUpdatePassword(u.ID, defaultResetSecret); err != nil {
			failed = append(failed, fmt.Sprintf("%s(%v)", u.Username, err))
			continue
		}
		okCount++
	}

	fmt.Printf("重置完成：成功 %d，失败 %d\n", okCount, len(failed))
	if len(failed) > 0 {
		fmt.Printf("失败列表：%s\n", strings.Join(failed, ", "))
		return fmt.Errorf("存在重置失败用户")
	}
	return nil
}

func normalizeUsernames(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}

func loadAdminUserIDSet() (map[string]struct{}, error) {
	ids, err := model.UserRoleMappingUserIdListByRoleId("sys-admin")
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}
	return result, nil
}

func readConfirmYes() bool {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(line), "y")
}
