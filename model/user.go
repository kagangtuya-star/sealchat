package model

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/blake2s"
	"gorm.io/gorm"

	"sealchat/protocol"
	"sealchat/utils"
)

func resolveAuthTokenMaxAgeDuration() time.Duration {
	maxAgeDays := utils.ResolveAuthSessionMaxAgeDays()
	return time.Duration(maxAgeDays*24) * time.Hour
}

// UserModel 用户表
type UserModel struct {
	StringPKBaseModel
	Nickname  string `gorm:"null" json:"nick"` // 昵称
	Avatar    string `json:"avatar"`           // 头像
	NickColor string `json:"nick_color"`       // 昵称颜色
	Brief     string `json:"brief"`            // 简介
	// Role     string `json:"role"`             // 权限

	Username string `gorm:"index:idx_username,unique;not null" json:"username"` // 用户名，唯一，非空
	Password string `gorm:"not null" json:"-"`                                  // 密码，非空
	Salt     string `gorm:"not null" json:"-"`                                  // 盐，非空
	IsBot    bool   `gorm:"null" json:"is_bot"`                                 // 是否是机器人

	Email           *string    `gorm:"size:254;index:idx_user_email,unique" json:"email,omitempty"`
	EmailVerified   bool       `gorm:"default:false" json:"emailVerified"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt,omitempty"`

	Disabled    bool              `json:"disabled"`
	AccessToken *AccessTokenModel `gorm:"-" json:"-"`

	RoleIds []string `json:"roleIds" gorm:"-"`
	// Token          string `gorm:"index" json:"token"` // 令牌
	// TokenExpiresAt int64  `json:"expiresAt"`
	// RecentSentAt int64 `json:"recentSentAt"` // 最近发送消息的时间
}

func (*UserModel) TableName() string {
	return "users"
}

func (u *UserModel) ToProtocolType() *protocol.User {
	return &protocol.User{
		ID:     u.ID,
		Nick:   u.Nickname,
		Avatar: u.Avatar,
		IsBot:  u.IsBot,
		Name:   u.Username,
	}
}

func (u *UserModel) SaveAvatar() {
	db.Model(u).Update("avatar", u.Avatar)
}

func (u *UserModel) SaveInfo() {
	db.Model(u).Select("nickname", "brief").Updates(u)
}

// UserSetDisable 禁用用户函数
func UserSetDisable(userId string, val bool) error {
	return db.Model(&UserModel{}).Where("id = ?", userId).Update("disabled", val).Error
}

// AccessTokenModel access_token表
type AccessTokenModel struct {
	StringPKBaseModel
	UserID    string    `json:"userID" gorm:"not null"`    // 用户ID，非空
	ExpiredAt time.Time `json:"expiredAt" gorm:"not null"` // 过期时间，非空
}

func (*AccessTokenModel) TableName() string {
	return "access_tokens"
}

// 生成随机盐
func generateSalt() string {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(salt)
}

// 使用盐对密码进行哈希
func hashPassword(password string, salt string) (string, error) {
	// 将密码和盐拼接起来
	saltedPassword := password + salt

	// 计算哈希值
	hashBytes := blake2s.Sum256([]byte(saltedPassword))
	hash := base64.RawStdEncoding.EncodeToString(hashBytes[:])

	return hash, nil
}

func UserCount() int64 {
	var count int64
	db.Select("id").Find(&UserModel{}).Count(&count)
	return count
}

// 创建用户
func UserCreate(username, password string, nickname string) (*UserModel, error) {
	salt := generateSalt()
	hashedPassword, err := hashPassword(password, salt)
	if err != nil {
		return nil, err
	}
	user := &UserModel{
		Username: username,
		Nickname: nickname,
		Password: hashedPassword,
		Salt:     salt,
	}
	user.ID = utils.NewID()
	if err := db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

// 修改密码
func UserUpdatePassword(userID string, newPassword string) error {
	// 查询用户
	var user UserModel
	if err := db.Where("id = ?", userID).First(&user).Error; err != nil {
		return fmt.Errorf("UserModel not found")
	}

	// 更新密码
	salt := generateSalt()
	hashedNewPassword, err := hashPassword(newPassword, salt)
	if err != nil {
		return err
	}
	if err := db.Model(&user).Updates(map[string]interface{}{
		"password": hashedNewPassword,
		"salt":     salt,
	}).Error; err != nil {
		return err
	}
	return nil
}

var (
	ErrInvalidCredentials = errors.New("账号或密码错误")
	ErrNicknameNotUnique  = errors.New("昵称不唯一，请使用用户名")
	ErrEmailAlreadyUsed   = errors.New("该邮箱已被使用")
)

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") {
		return true
	}
	if strings.Contains(msg, "Error 1062") || strings.Contains(msg, "Duplicate entry") {
		return true
	}
	if strings.Contains(msg, "SQLSTATE 23505") || strings.Contains(msg, "duplicate key value") {
		return true
	}
	return false
}

func verifyUserPassword(user *UserModel, password string) error {
	hashedPassword, err := hashPassword(password, user.Salt)
	if err != nil {
		return err
	}
	if hashedPassword != user.Password {
		return ErrInvalidCredentials
	}
	return nil
}

// 登录认证（仅用户名）
func UserAuthenticate(username, password string) (*UserModel, error) {
	var user UserModel
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if err := verifyUserPassword(&user, password); err != nil {
		return nil, err
	}
	return &user, nil
}

// 登录认证（用户名/昵称/邮箱）
func UserAuthenticateByAccount(account, password string) (*UserModel, error) {
	var user UserModel
	if err := db.Where("username = ?", account).First(&user).Error; err == nil {
		if err := verifyUserPassword(&user, password); err != nil {
			return nil, err
		}
		return &user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if strings.Contains(account, "@") {
		email := strings.ToLower(account)
		if err := db.Where("email = ?", email).First(&user).Error; err == nil {
			if err := verifyUserPassword(&user, password); err != nil {
				return nil, err
			}
			return &user, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	var users []UserModel
	if err := db.Where("nickname = ?", account).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, ErrInvalidCredentials
	}
	if len(users) > 1 {
		return nil, ErrNicknameNotUnique
	}
	if err := verifyUserPassword(&users[0], password); err != nil {
		return nil, err
	}
	return &users[0], nil
}

func AcessTokenDeleteAllByUserID(userID string) error {
	return db.Where("user_id = ?", userID).Delete(&AccessTokenModel{}).Error
}

// UserGenerateAccessToken 生成 access_token
func UserGenerateAccessToken(userID string) (string, error) {
	expiredAt := time.Now().Add(resolveAuthTokenMaxAgeDuration())

	token := utils.NewID()
	accessToken := &AccessTokenModel{
		UserID:    userID,
		ExpiredAt: expiredAt,
	}

	accessToken.ID = token
	signedToken := TokenSign(accessToken.ID, expiredAt)
	if err := db.Create(accessToken).Error; err != nil {
		return "", err
	}
	return signedToken, nil
}

// UserVerifyAccessToken 验证 access_token 是否有效
func UserVerifyAccessToken(tokenString string) (*UserModel, error) {
	// 解析 token
	ret := TokenCheck(tokenString)

	if !ret.HashValid {
		return nil, ErrInvalidToken
	}

	if !ret.TimeValid {
		return nil, ErrTokenExpired
	}

	var accessToken AccessTokenModel
	if err := db.Where("id = ?", ret.Token).Limit(1).Find(&accessToken).Error; err != nil {
		return nil, ErrInvalidToken
	}

	if accessToken.ID == "" {
		return nil, ErrInvalidToken
	}

	now := time.Now()
	if accessToken.ExpiredAt.Compare(now) <= 0 {
		// 二次过期时间校验
		return nil, ErrInvalidToken
	}

	// 查询用户
	var user UserModel
	if err := db.Where("id = ?", accessToken.UserID).Limit(1).Find(&user).Error; err != nil {
		return nil, fmt.Errorf("user not found")
	}

	user.AccessToken = &accessToken
	return &user, nil
}

// UserRefreshAccessToken 刷新 access_token
func UserRefreshAccessToken(tokenID string) (string, error) {
	expiredAt := time.Now().Add(resolveAuthTokenMaxAgeDuration())

	var accessToken AccessTokenModel
	if err := db.Where("id = ?", tokenID).First(&accessToken).Error; err != nil {
		return "", ErrInvalidToken
	}

	if err := db.Model(&AccessTokenModel{}).Update("expired_at", expiredAt).Error; err != nil {
		return "", fmt.Errorf("update failed")
	}

	signedToken := TokenSign(accessToken.ID, expiredAt)
	return signedToken, nil
}

// UserGetEx 获取用户信息
func UserGetEx(id string) (*UserModel, error) {
	var user UserModel
	result := db.Where("id = ?", id).Limit(1).Find(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("获取用户信息失败: %v", result.Error)
	}
	return &user, nil
}

// UserGetByUsername 通过用户名获取用户信息
func UserGetByUsername(username string) (*UserModel, error) {
	trimmed := strings.TrimSpace(username)
	if trimmed == "" {
		return nil, fmt.Errorf("用户名不能为空")
	}
	var user UserModel
	result := db.Where("username = ?", trimmed).Limit(1).Find(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("用户不存在")
		}
		return nil, fmt.Errorf("获取用户信息失败: %v", result.Error)
	}
	if user.ID == "" {
		return nil, fmt.Errorf("用户不存在")
	}
	return &user, nil
}

func UserGet(id string) *UserModel {
	r, _ := UserGetEx(id)
	return r
}

// UserBotList 查询所有启用的机器人用户
func UserBotList() ([]*UserModel, error) {
	var bots []*UserModel
	err := db.Where("disabled = ? AND is_bot = ?", false, true).Find(&bots).Error
	if err != nil {
		return nil, err
	}
	return bots, nil
}

// UserExistsByUsername 检查用户名是否已存在
func UserExistsByUsername(username string) bool {
	var count int64
	db.Model(&UserModel{}).Where("username = ?", username).Count(&count)
	return count > 0
}

// UserExistsByUsernames 批量检查用户名是否已存在，返回已存在的用户名集合
func UserExistsByUsernames(usernames []string) map[string]bool {
	if len(usernames) == 0 {
		return make(map[string]bool)
	}
	var existing []string
	db.Model(&UserModel{}).Where("username IN ?", usernames).Pluck("username", &existing)
	result := make(map[string]bool)
	for _, u := range existing {
		result[u] = true
	}
	return result
}

// UsersDuplicateRemove 删除重复的用户，只保留最早的一个
func UsersDuplicateRemove() error {
	var users []UserModel
	// 查找所有重复的用户名
	if err := db.Select("username, MIN(created_at) as min_created_at").
		Group("username").
		Having("COUNT(*) > 1").
		Find(&users).Error; err != nil {
		return err
	}
	// 对每个重复的用户名进行处理
	for _, user := range users {
		// 删除除最早创建的记录之外的所有记录
		if err := db.Unscoped().Where("username = ? AND created_at > ?", user.Username, user.CreatedAt).
			Delete(&UserModel{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// UserGetByEmail 通过邮箱查询用户
func UserGetByEmail(email string) (*UserModel, error) {
	var user UserModel
	if err := db.Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// UserExistsByEmail 检查邮箱是否已被使用
func UserExistsByEmail(email string) (bool, error) {
	var count int64
	if err := db.Model(&UserModel{}).Where("email = ?", email).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UserExistsByEmailExcludingUser 检查邮箱是否被其他用户使用
func UserExistsByEmailExcludingUser(email, excludeUserID string) (bool, error) {
	var count int64
	if err := db.Model(&UserModel{}).Where("email = ? AND id != ?", email, excludeUserID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UserBindEmail 为用户绑定邮箱
func UserBindEmail(userID, email string) error {
	now := time.Now()
	err := db.Model(&UserModel{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"email":             email,
		"email_verified":    true,
		"email_verified_at": now,
	}).Error
	if err == nil {
		return nil
	}
	if isUniqueConstraintError(err) {
		return ErrEmailAlreadyUsed
	}
	return err
}

// UserCreateWithEmail 创建带邮箱的用户
func UserCreateWithEmail(username, password, nickname, email string) (*UserModel, error) {
	salt := generateSalt()
	hashedPassword, err := hashPassword(password, salt)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user := &UserModel{
		Username:        username,
		Nickname:        nickname,
		Password:        hashedPassword,
		Salt:            salt,
		Email:           &email,
		EmailVerified:   true,
		EmailVerifiedAt: &now,
	}
	user.ID = utils.NewID()
	if err := db.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

// UserGetByEmailOrUsername 通过邮箱或用户名查询用户
func UserGetByEmailOrUsername(account string) (*UserModel, error) {
	var user UserModel
	if err := db.Where("username = ? OR email = ?", account, account).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}
