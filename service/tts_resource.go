package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/service/storage"
	"sealchat/utils"
)

func TTSSpoolDir() (string, error) {
	m := GetStorageManager()
	if m == nil {
		return "", fmt.Errorf("存储尚未初始化")
	}
	p, err := m.ResolveLocalPath("tts-private/spool")
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(p, 0700); err != nil {
		return "", err
	}
	return p, nil
}

func TTSPersistAudio(userID, channelID, jobID, kind, path string, media ttsprovider.Media) (*model.AttachmentModel, error) {
	m := GetStorageManager()
	if m == nil {
		return nil, fmt.Errorf("存储尚未初始化")
	}
	// Copies keep the validated spool intact if moving/uploading or DB persistence
	// fails. The caller may retry storage without another synthesis request.
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dir, err := TTSSpoolDir()
	if err != nil {
		return nil, err
	}
	copyFile, err := os.CreateTemp(dir, "archive-*")
	if err != nil {
		return nil, err
	}
	copyPath := copyFile.Name()
	defer os.Remove(copyPath)
	if _, err = copyFile.Write(b); err != nil {
		copyFile.Close()
		return nil, err
	}
	if err = copyFile.Close(); err != nil {
		return nil, err
	}
	id := utils.NewID()
	key := "tts-private/resources/" + id
	ctx := context.Background()
	input := storage.UploadInput{ObjectKey: key, LocalPath: copyPath, ContentType: ttsMIME(media.Container)}
	var result *storage.UploadResult
	if kind == "job_audio" {
		result, err = m.UploadTTS(ctx, input)
	} else {
		// Clone samples and unknown kinds never opt into remote storage.
		result, err = m.UploadWithBackend(ctx, storage.BackendLocal, input)
	}
	if err != nil {
		return nil, err
	}
	meta, _ := json.Marshal(media)
	storageType := model.StorageLocal
	if result.Backend == storage.BackendS3 {
		storageType = model.StorageS3
	}
	a := &model.AttachmentModel{StringPKBaseModel: model.StringPKBaseModel{ID: id}, UserID: userID, ChannelID: channelID, RootIDType: "tts", RootID: jobID, ParentIDType: kind, StorageType: storageType, ObjectKey: result.ObjectKey, Size: int64(len(b)), MimeType: ttsMIME(media.Container), Filename: "speech." + media.Container, Note: string(meta)}
	if err = model.GetDB().Create(a).Error; err != nil {
		// This object was created by this attempt and has no durable reference.
		// Remove the actual stored object; preserve the synthesis spool for retry.
		_ = m.Delete(ctx, result.Backend, result.ObjectKey)
		return nil, err
	}
	return a, nil
}
func ttsMIME(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}
func TTSResourcePath(a *model.AttachmentModel) (string, error) {
	if a == nil || !a.IsTTSManaged() || a.DeletedAt != nil || a.StorageType != model.StorageLocal || !strings.HasPrefix(a.ObjectKey, "tts-private/resources/") {
		return "", ErrTTSDenied
	}
	manager := GetStorageManager()
	if manager == nil {
		return "", ErrTTSDisabled
	}
	return manager.ResolveLocalPath(a.ObjectKey)
}
func TTSValidateSpool(path string) bool {
	dir, err := TTSSpoolDir()
	return err == nil && filepath.Dir(path) == dir && strings.HasPrefix(filepath.Base(path), "synthesis-")
}

func ttsResourceBackend(a *model.AttachmentModel) (storage.BackendType, error) {
	if a == nil || !a.IsTTSManaged() || a.DeletedAt != nil || !strings.HasPrefix(a.ObjectKey, "tts-private/resources/") {
		return "", ErrTTSDenied
	}
	switch a.StorageType {
	case model.StorageLocal:
		return storage.BackendLocal, nil
	case model.StorageS3:
		return storage.BackendS3, nil
	default:
		return "", ErrTTSDenied
	}
}

func TTSResourceExists(ctx context.Context, a *model.AttachmentModel) (bool, error) {
	backend, err := ttsResourceBackend(a)
	if err != nil {
		return false, err
	}
	m := GetStorageManager()
	if m == nil {
		return false, ErrTTSDisabled
	}
	return m.Exists(ctx, backend, a.ObjectKey)
}

// TTSOpenResource is used only after ticket/resource authorization.
func TTSOpenResource(ctx context.Context, a *model.AttachmentModel) (io.ReadCloser, error) {
	backend, err := ttsResourceBackend(a)
	if err != nil {
		return nil, err
	}
	m := GetStorageManager()
	if m == nil {
		return nil, ErrTTSDisabled
	}
	return m.OpenRead(ctx, backend, a.ObjectKey)
}
