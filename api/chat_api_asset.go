package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/spf13/afero"
	"modernc.org/libc/limits"

	"sealchat/model"
	"sealchat/service"
)

const assetIDHexLength = 64

func apiAssetUpload(ctx *ChatContext, data *struct {
	AssetID     string `json:"asset_id"`
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
	Data        string `json:"data"`
}) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, errors.New("unauthorized")
	}
	if !ctx.User.IsBot {
		return nil, errors.New("asset.upload requires bot")
	}

	assetID := strings.ToLower(strings.TrimSpace(data.AssetID))
	if len(assetID) != assetIDHexLength {
		return nil, errors.New("invalid asset_id")
	}
	if _, err := hex.DecodeString(assetID); err != nil {
		return nil, errors.New("invalid asset_id")
	}

	encoded := stripBase64Whitespace(data.Data)
	if encoded == "" {
		return nil, errors.New("missing asset data")
	}

	limit := appConfig.ImageSizeLimit * 1024
	if limit == 0 {
		limit = limits.INT_MAX
	}
	estimatedSize := int64(base64.StdEncoding.DecodedLen(len(encoded)))
	if estimatedSize > limit {
		return nil, ErrFileTooLarge
	}

	decoded, err := decodeBase64Payload(encoded)
	if err != nil {
		return nil, errors.New("invalid base64 data")
	}
	if int64(len(decoded)) > limit {
		return nil, ErrFileTooLarge
	}

	sum := sha256.Sum256(decoded)
	if hex.EncodeToString(sum[:]) != assetID {
		return nil, errors.New("asset_id mismatch")
	}

	var existing model.AttachmentModel
	if err := model.GetDB().Where("id = ?", assetID).Limit(1).Find(&existing).Error; err != nil {
		return nil, err
	}
	if existing.ID != "" {
		return map[string]any{
			"ok":       true,
			"asset_id": assetID,
			"existed":  true,
		}, nil
	}

	contentType := strings.TrimSpace(data.ContentType)
	if contentType == "" && len(decoded) > 0 {
		contentType = http.DetectContentType(decoded)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	tmpDir := appConfig.Storage.Local.TempDir
	if strings.TrimSpace(tmpDir) == "" {
		tmpDir = "./data/temp/"
	}
	_ = appFs.MkdirAll(tmpDir, 0755)
	tempFile, err := afero.TempFile(appFs, tmpDir, "*.asset")
	if err != nil {
		return nil, err
	}
	tempPath := tempFile.Name()
	if _, err := tempFile.Write(decoded); err != nil {
		_ = tempFile.Close()
		_ = appFs.Remove(tempPath)
		return nil, err
	}
	_ = tempFile.Close()

	location, err := service.PersistAttachmentFile(sum[:], int64(len(decoded)), tempPath, contentType)
	if err != nil {
		_ = appFs.Remove(tempPath)
		return nil, err
	}

	filename := strings.TrimSpace(data.Filename)
	if filename == "" {
		filename = assetID
	}

	_, newItem := model.AttachmentCreate(&model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: assetID},
		Filename:    filename,
		Size:        int64(len(decoded)),
		Hash:        sum[:],
		MimeType:    contentType,
		UserID:      ctx.User.ID,
		StorageType: location.StorageType,
		ObjectKey:   location.ObjectKey,
		ExternalURL: location.ExternalURL,
	})

	return map[string]any{
		"ok":       true,
		"asset_id": newItem.ID,
		"existed":  false,
	}, nil
}

func stripBase64Whitespace(value string) string {
	if value == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(value))
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch ch {
		case ' ', '\n', '\r', '\t':
			continue
		default:
			sb.WriteByte(ch)
		}
	}
	return sb.String()
}

func decodeBase64Payload(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("empty base64")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(encoded)
}
