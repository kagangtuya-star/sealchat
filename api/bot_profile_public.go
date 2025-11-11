package api

import (
	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

func BotProfileOptions(c *fiber.Ctx) error {
	items, err := model.BotProfileList()
	if err != nil {
		return err
	}
	type option struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AvatarURL   string `json:"avatarUrl"`
		ConnMode    string `json:"connMode"`
		UserID      string `json:"userId"`
		Description string `json:"description,omitempty"`
	}
	opts := make([]option, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		opts = append(opts, option{
			ID:        item.ID,
			Name:      item.Name,
			AvatarURL: item.AvatarURL,
			ConnMode:  string(item.ConnMode),
			UserID:    item.UserID,
		})
	}
	return c.JSON(fiber.Map{
		"items": opts,
	})
}
