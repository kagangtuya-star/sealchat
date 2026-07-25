package service

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

func ActivateWorldTheater(actorID, worldID, activationCode, configuredCode string) (bool, error) {
	world, _, err := requireTheaterPermission(actorID, worldID, "", TheaterPermissionView)
	if err != nil {
		return false, err
	}
	if world.TheaterActivated {
		return true, nil
	}
	if expected := strings.TrimSpace(configuredCode); expected != "" && activationCode != expected {
		return false, newTheaterError(TheaterErrorActivationRequired, "小剧场需要激活码", fiber.StatusConflict, nil)
	}
	if err := model.GetDB().Model(&model.WorldModel{}).
		Where("id = ? AND status = ?", world.ID, "active").
		Update("theater_activated", true).Error; err != nil {
		return false, err
	}
	return true, nil
}
