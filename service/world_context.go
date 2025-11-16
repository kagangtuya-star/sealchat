package service

import (
	"fmt"
	"strings"
	"sync"

	"sealchat/model"
)

var (
	worldInitOnce    sync.Once
	worldInitErr     error
	defaultWorldLock sync.RWMutex
	defaultWorld     *model.WorldModel
)

// InitWorldContext 初始化世界上下文，确保默认世界存在
func InitWorldContext() error {
	worldInitOnce.Do(func() {
		worldInitErr = ensureDefaultWorld()
	})
	return worldInitErr
}

func ensureDefaultWorld() error {
	var world model.WorldModel
	if err := model.GetDB().
		Order("created_at asc").
		Limit(1).
		Find(&world).Error; err != nil {
		return err
	}
	if world.ID == "" {
		created, err := CreateWorld(CreateWorldOptions{
			Name:        "默认世界",
			Description: "系统自动创建的默认世界",
			Visibility:  model.WorldVisibilityPublic,
			JoinPolicy:  model.WorldJoinPolicyOpen,
		})
		if err != nil {
			return err
		}
		setDefaultWorld(created)
		return nil
	}
	setDefaultWorld(&world)
	return nil
}

func setDefaultWorld(world *model.WorldModel) {
	if world == nil {
		return
	}
	defaultWorldLock.Lock()
	defer defaultWorldLock.Unlock()
	if defaultWorld == nil {
		defaultWorld = world
	}
}

// DefaultWorld 获取当前默认世界
func DefaultWorld() *model.WorldModel {
	_ = InitWorldContext()
	defaultWorldLock.RLock()
	defer defaultWorldLock.RUnlock()
	return defaultWorld
}

// DefaultWorldID 返回默认世界ID
func DefaultWorldID() string {
	world := DefaultWorld()
	if world == nil || world.ID == "" {
		panic("默认世界尚未初始化")
	}
	return world.ID
}

// ResolveWorldID 使用显式传入或回落到默认世界
func ResolveWorldID(candidate string) (string, error) {
	return resolveWorldID(strings.TrimSpace(candidate))
}

func resolveWorldID(candidate string) (string, error) {
	if candidate != "" {
		return candidate, nil
	}
	world := DefaultWorld()
	if world == nil || world.ID == "" {
		return "", fmt.Errorf("缺少世界上下文")
	}
	return world.ID, nil
}
