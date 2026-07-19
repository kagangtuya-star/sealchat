package protocol

import (
	"errors"
	"strings"
)

type StickyNoteAppearance struct {
	Version    int                        `json:"version"`
	Background *StickyNoteBackgroundImage `json:"background,omitempty"`
}

type StickyNoteBackgroundImage struct {
	Kind               string  `json:"kind"`
	AttachmentID       string  `json:"attachmentId"`
	Opacity            float64 `json:"opacity"`
	Fit                string  `json:"fit"`
	PositionX          float64 `json:"positionX"`
	PositionY          float64 `json:"positionY"`
	ContentWashOpacity float64 `json:"contentWashOpacity"`
}

func ValidateStickyNoteAppearance(value *StickyNoteAppearance) error {
	if value == nil {
		return nil
	}
	if value.Version != 1 {
		return errors.New("便签背景版本无效")
	}
	if value.Background == nil {
		return nil
	}
	background := value.Background
	if background.Kind != "image" || strings.TrimSpace(background.AttachmentID) == "" {
		return errors.New("便签背景图片无效")
	}
	switch background.Fit {
	case "cover", "contain", "stretch", "tile":
	default:
		return errors.New("便签背景填充方式无效")
	}
	if background.Opacity < 0 || background.Opacity > 1 ||
		background.PositionX < 0 || background.PositionX > 100 ||
		background.PositionY < 0 || background.PositionY > 100 ||
		background.ContentWashOpacity < 0 || background.ContentWashOpacity > 1 {
		return errors.New("便签背景参数超出范围")
	}
	return nil
}
