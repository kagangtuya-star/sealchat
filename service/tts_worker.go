package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/protocol"
	"sealchat/utils"
)

var ttsWake = make(chan struct{}, 1)
var ttsWorkerOnce sync.Once
var ttsTimelines = struct {
	sync.Mutex
	items     map[string]chan struct{}
	synthesis map[string]*ttsSynthesisCancel
}{items: map[string]chan struct{}{}, synthesis: map[string]*ttsSynthesisCancel{}}

type ttsSynthesisCancel struct {
	cancel context.CancelFunc
}

var ttsCallbacks struct {
	sync.RWMutex
	ready   func(model.TTSJob, ttsprovider.Media, string, bool) bool
	invalid func(string)
	live    func(context.Context, model.TTSJob, string) func(bool) bool
}

// The live finish function reports whether a PCM start frame was dispatched,
// i.e. whether the realtime stream already paced the channel lane. ready is
// told so, and reports whether the lane must still wait for the file timeline.
// Neither can know what a browser heard; browsers deduplicate by message.
func TTSSetCallbacks(ready func(model.TTSJob, ttsprovider.Media, string, bool) bool, invalid func(string), live func(context.Context, model.TTSJob, string) func(bool) bool) {
	ttsCallbacks.Lock()
	defer ttsCallbacks.Unlock()
	ttsCallbacks.ready = ready
	ttsCallbacks.invalid = invalid
	ttsCallbacks.live = live
}

type ttsLiveContextKey struct{}

// A message job may wait behind the channel's earlier audio. Once queued it is
// stale only after this bound (restart backlog, abnormal lanes), never merely
// because its predecessors speak for long. Unqueued intents expire separately.
const ttsMessageQueueWait = 15 * time.Minute

func TTSWake() {
	select {
	case ttsWake <- struct{}{}:
	default:
	}
}
func TTSCancelMessage(id string) {
	var jobs []model.TTSJob
	if model.GetDB().Where("message_id = ? AND status = ?", id, "queued").Find(&jobs).Error == nil {
		for _, j := range jobs {
			if ttsReleaseJob(model.GetDB(), j.ID) == nil {
				ttsMessageStatus(&j, "cancelled", nil)
			}
		}
	}
	TTSStopPlayback(id)
}

func TTSClearChannelPending(channelID string) error {
	return model.GetDB().Model(&model.MessageModel{}).Where("channel_id = ? AND tts_status = ? AND deleted_at IS NULL", channelID, "pending").Updates(map[string]any{"tts_status": "cancelled", "tts_intent": ""}).Error
}

func TTSStopPlayback(id string) {
	ttsTimelines.Lock()
	synthesis := ttsTimelines.synthesis[id]
	if stop, ok := ttsTimelines.items[id]; ok {
		close(stop)
		delete(ttsTimelines.items, id)
	}
	ttsTimelines.Unlock()
	if synthesis != nil {
		synthesis.cancel()
	}
	ttsCallbacks.RLock()
	fn := ttsCallbacks.invalid
	ttsCallbacks.RUnlock()
	if fn != nil {
		fn(id)
	}
}

func ttsRegisterSynthesisCancel(messageID string, cancel context.CancelFunc) func() {
	if messageID == "" {
		return func() {}
	}
	entry := &ttsSynthesisCancel{cancel: cancel}
	ttsTimelines.Lock()
	ttsTimelines.synthesis[messageID] = entry
	ttsTimelines.Unlock()
	return func() {
		ttsTimelines.Lock()
		if ttsTimelines.synthesis[messageID] == entry {
			delete(ttsTimelines.synthesis, messageID)
		}
		ttsTimelines.Unlock()
	}
}

func StartTTSWorker(ctx context.Context) {
	ttsWorkerOnce.Do(func() {
		go ttsWorker(ctx)
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					ttsMaintainVoices(ctx)
				}
			}
		}()
	})
}
func ttsWorker(ctx context.Context) {
	db := model.GetDB()
	// Crash recovery never resubmits a possibly accepted charged POST.
	_ = db.Model(&model.TTSJob{}).Where("status = ? AND usage_status = ?", "running", "settled").Updates(map[string]any{"status": "storage_pending", "error_code": "archive_interrupted"}).Error
	_ = db.Model(&model.TTSJob{}).Where("status = ? AND usage_status = ?", "running", "cached").Update("status", "cancelled").Error
	_ = db.Model(&model.TTSJob{}).Where("status = ? AND usage_status <> ?", "running", "settled").Updates(map[string]any{"status": "usage_unknown", "usage_status": "unknown", "error_code": "server_restarted"}).Error
	_ = db.Model(&model.TTSJob{}).Where("status = ?", "archiving").Update("status", "storage_pending").Error
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var mu sync.Mutex
	lanes := map[string]bool{}
	running := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ttsWake:
		case <-ticker.C:
		}
		ttsRecoverOutbox()
		// Disabling new charged operations must not suspend archival of already
		// billed audio. Queued HTTP work still checks ttsConfig inside ttsRun.
		var speechConfig *utils.SpeechConfig
		if config := utils.GetConfig(); config != nil {
			speechConfig = config.AI.Speech
		}
		if speechConfig == nil {
			speechConfig = &utils.SpeechConfig{}
		}
		cfg := utils.NormalizeSpeechConfig(speechConfig)
		var jobs []model.TTSJob
		if db.Where("status IN ?", []string{"queued", "storage_pending"}).Order("queue_order ASC, created_at ASC, id ASC").Limit(100).Find(&jobs).Error != nil {
			continue
		}
		for _, j := range jobs {
			lane := j.ChannelID
			if lane == "" {
				lane = "user:" + j.PayerUserID
			}
			mu.Lock()
			if running >= cfg.MaxConcurrent || lanes[lane] {
				mu.Unlock()
				continue
			}
			running++
			lanes[lane] = true
			mu.Unlock()
			go func(job model.TTSJob, lane string) {
				defer func() { mu.Lock(); running--; delete(lanes, lane); mu.Unlock(); TTSWake() }()
				ttsRun(ctx, &job)
			}(j, lane)
		}
	}
}

func ttsRecoverOutbox() {
	db := model.GetDB()
	var messages []model.MessageModel
	if db.Where("tts_status = ? AND tts_intent <> ?", "pending", "").Order("created_at ASC, id ASC").Limit(50).Find(&messages).Error != nil {
		return
	}
	for _, m := range messages {
		status := "queued"
		var s TTSSnapshot
		if m.IsDeleted || m.IsRevoked || m.DeletedAt != nil || time.Since(m.CreatedAt) > 2*time.Minute || json.Unmarshal([]byte(m.TTSIntent), &s) != nil {
			status = "skipped"
		} else {
			j := &model.TTSJob{Operation: "message_synthesis", RequestKey: "message:" + m.ID, PayerUserID: m.UserID, ChannelID: m.ChannelID, MessageID: m.ID, MessageRevision: int64(m.EditCount), Snapshot: m.TTSIntent, VoiceID: s.VoiceID, Deadline: time.Now().Add(ttsMessageQueueWait)}
			if err := ttsReserveSnapshot(j, s); err != nil {
				status = "skipped"
			}
		}
		_ = db.Model(&model.MessageModel{}).Where("id = ? AND tts_status = ? AND edit_count = ?", m.ID, "pending", m.EditCount).Update("tts_status", status).Error
	}
}

func ttsMessageCurrent(job *model.TTSJob, s TTSSnapshot) bool {
	return ttsMessageCurrentWithDB(model.GetDB(), job, s)
}

func TTSJobMessageCurrent(job *model.TTSJob) bool {
	var snapshot TTSSnapshot
	return json.Unmarshal([]byte(job.Snapshot), &snapshot) == nil && ttsMessageCurrent(job, snapshot)
}

func ttsMessageCurrentWithDB(db *gorm.DB, job *model.TTSJob, s TTSSnapshot) bool {
	if job.MessageID == "" {
		return true
	}
	var m model.MessageModel
	if db.Where("id = ? AND is_deleted = ? AND (is_revoked = ? OR is_revoked IS NULL) AND deleted_at IS NULL AND edit_count = ?", job.MessageID, false, false, job.MessageRevision).First(&m).Error != nil {
		return false
	}
	text, err := ttsPlainText(m.Content, ttsMessageSynthesisMaxRunes)
	if m.IsWhisper != (s.Whisper || len(s.Audience) > 0) || (job.ChannelID != "" && m.ChannelID != job.ChannelID) || (job.PayerUserID != "" && m.UserID != job.PayerUserID) {
		return false
	}
	if err != nil || text != s.Input.Text || m.TTSIntent != job.Snapshot {
		return false
	}
	if m.IsWhisper {
		var recipients []model.MessageWhisperRecipientModel
		if db.Where("message_id = ? AND deleted_at IS NULL", m.ID).Find(&recipients).Error != nil {
			return false
		}
		seen := map[string]bool{}
		for _, recipient := range recipients {
			seen[recipient.UserID] = true
		}
		if m.WhisperTo != "" {
			seen[m.WhisperTo] = true
		}
		audience := make([]string, 0, len(seen))
		for userID := range seen {
			audience = append(audience, userID)
		}
		sort.Strings(audience)
		if len(audience) != len(s.Audience) {
			return false
		}
		for i := range audience {
			if audience[i] != s.Audience[i] {
				return false
			}
		}
	}
	return true
}

// A ticket must also match the whisper audience captured at send time. This
// denies newly added recipients even if a separate message-edit path omitted a
// revision increment; already delivered bytes cannot be recalled.
func TTSMessageAudioCurrent(message *model.MessageModel) bool {
	data := message.ValidTTS()
	if data == nil || data.Status != "ready" || data.AudioResourceID == "" {
		return false
	}
	var job model.TTSJob
	if model.GetDB().Where("message_id = ? AND resource_id = ? AND message_revision = ? AND deleted_at IS NULL", message.ID, data.AudioResourceID, data.MessageRevision).First(&job).Error != nil {
		return false
	}
	var snapshot TTSSnapshot
	return json.Unmarshal([]byte(job.Snapshot), &snapshot) == nil && ttsMessageCurrent(&job, snapshot)
}

func ttsRun(parent context.Context, j *model.TTSJob) {
	db := model.GetDB()
	var s TTSSnapshot
	if json.Unmarshal([]byte(j.Snapshot), &s) != nil {
		ttsReleaseMessageJob(db, j, "failed")
		return
	}
	if j.Status == "storage_pending" {
		r := db.Model(&model.TTSJob{}).Where("id = ? AND status = ?", j.ID, "storage_pending").Update("status", "archiving")
		if r.Error == nil && r.RowsAffected == 1 {
			ttsArchive(parent, j, s)
		}
		return
	}
	if time.Now().After(j.Deadline) || !ttsMessageCurrent(j, s) {
		ttsReleaseMessageJob(db, j, "skipped")
		return
	}
	cfg, err := ttsConfig()
	if err != nil {
		ttsReleaseMessageJob(db, j, "unavailable")
		return
	}
	var provider *utils.SpeechProviderConfig
	for _, p := range cfg.Providers {
		if p.ID == s.Provider.ID && p.Enabled && p.CredentialScope == s.Provider.CredentialScope && p.Region == s.Provider.Region && p.Workspace == s.Provider.Workspace && p.Model == s.Provider.Model && p.SynthesisEndpoint == s.Provider.SynthesisEndpoint && p.VoiceEndpoint == s.Provider.VoiceEndpoint {
			cp := p
			provider = &cp
			break
		}
	}
	if provider == nil {
		ttsReleaseMessageJob(db, j, "unavailable")
		return
	}
	if s.VoiceID != "" {
		if _, err = ttsAccessibleVoice(db, j.PayerUserID, s.VoiceID); err != nil {
			ttsReleaseMessageJob(db, j, "unavailable")
			return
		}
	}
	if j.UsageStatus == "cached" {
		r := db.Model(&model.TTSJob{}).Where("id = ? AND status = ? AND usage_status = ?", j.ID, "queued", "cached").Update("status", "running")
		if r.Error != nil || r.RowsAffected != 1 {
			return
		}
		var resource model.AttachmentModel
		var media ttsprovider.Media
		if db.Where("id = ? AND deleted_at IS NULL", j.ResourceID).First(&resource).Error != nil || json.Unmarshal([]byte(j.MediaJSON), &media) != nil {
			_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "failed", "error_code": "cache_unavailable"}).Error
			ttsMessageStatus(j, "failed", nil)
			return
		}
		if err := db.Model(&model.TTSJob{}).Where("id = ? AND status = ?", j.ID, "running").Update("status", "succeeded").Error; err != nil {
			return
		}
		ttsPublishMessage(parent, j, s, &resource, media)
		return
	}
	dir, err := TTSSpoolDir()
	if err != nil {
		return
	}
	f, err := os.CreateTemp(dir, "synthesis-*")
	if err != nil {
		return
	}
	path := f.Name()
	ctx, cancel := context.WithTimeout(parent, time.Duration(cfg.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	cleanupSynthesisCancel := func() {}
	if j.Operation == "message_synthesis" {
		cleanupSynthesisCancel = ttsRegisterSynthesisCancel(j.MessageID, cancel)
	}
	defer cleanupSynthesisCancel()
	r := db.Model(&model.TTSJob{}).Where("id = ? AND status = ?", j.ID, "queued").Updates(map[string]any{"status": "running", "spool_path": path})
	if r.Error != nil || r.RowsAffected != 1 {
		f.Close()
		os.Remove(path)
		return
	}
	j.SpoolPath = path
	stopCurrentWatch := make(chan struct{})
	defer close(stopCurrentWatch)
	if j.Operation == "message_synthesis" && j.MessageID != "" {
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-stopCurrentWatch:
					return
				case <-ticker.C:
					if !ttsMessageCurrent(j, s) {
						cancel()
						return
					}
				}
			}
		}()
	}
	var units int64
	var synthesisUsage *ttsprovider.Result
	confirmed := false
	requestID := ""
	audioURL := ""
	sseWAV := false
	if j.Operation == "design" || j.Operation == "clone" {
		var result ttsprovider.VoiceResult
		result, err = ttsProviderCreateVoice(ctx, *provider, j, s)
		requestID = result.RequestID
		confirmed = err == nil && result.Usage.Count != nil && *result.Usage.Count == 1
		units = 1
		if err == nil && result.Output.VoiceID == "" {
			err = TTSValidationError("供应商未返回音色 ID")
		}
		if result.Output.VoiceID != "" {
			updates := map[string]any{"provider_voice_id": result.Output.VoiceID, "name": s.Name, "description": s.Description, "source_resource_id": s.SourceResourceID, "provider_status": "DEPLOYING"}
			if result.Output.TargetModel != "" && result.Output.TargetModel != s.Provider.Model {
				err = ErrTTSConflict
			}
			_ = db.Model(&model.TTSVoice{}).Where("id = ?", j.VoiceID).Updates(updates).Error
			if result.Output.PreviewAudio != nil {
				data, e := base64.StdEncoding.DecodeString(result.Output.PreviewAudio.Data)
				if e == nil && len(data) <= ttsprovider.MaxAudioBytes {
					_, e = f.Write(data)
				}
				if e != nil {
					err = e
				}
			}
		}
	} else {
		var finish func(bool) bool
		if j.MessageID != "" && s.Input.Format == "wav" {
			ttsCallbacks.RLock()
			live := ttsCallbacks.live
			ttsCallbacks.RUnlock()
			if live != nil {
				finish = live(parent, *j, path)
			}
		}
		result, e := ttsProviderSynthesize(ctx, *provider, j, s, f)
		if finish != nil {
			// Provider stream audio feeds realtime playback. When the HTTP path
			// names a finished file, archive validation applies to that file
			// after the live reader has closed the spool. Failure cancels
			// already sent PCM; it never retries a charged request.
			ok := e == nil && result.Complete
			if ok && result.AudioURL == "" {
				// Read-only check on a copy; the spool is sealed after Close.
				b, readErr := os.ReadFile(path)
				_, mediaErr := ttsprovider.FinalizePCM16WAV(b)
				ok = readErr == nil && mediaErr == nil
			}
			if finish(ok) {
				parent = context.WithValue(parent, ttsLiveContextKey{}, true)
			}
		}
		err = e
		units = result.Characters
		confirmed = ttsSynthesisUsageConfirmed(s.Provider, result)
		synthesisUsage = &result
		requestID = result.RequestID
		if e == nil && result.Complete {
			audioURL = result.AudioURL
			sseWAV = audioURL == "" && s.Input.Format == "wav"
		}
	}
	closeErr := f.Close()
	var providerError *ttsprovider.ProviderError
	if requestID == "" && errors.As(err, &providerError) {
		requestID = providerError.RequestID
	}
	if err == nil {
		err = closeErr
	}
	// audio.url is not persisted, so the complete file must replace the SSE
	// spool before settlement: crash recovery turns running+settled into
	// storage_pending and archives j.SpoolPath as is. A failed fetch still
	// settles confirmed usage and never resubmits the charged POST.
	canonicalCode := ""
	if err == nil && audioURL != "" {
		dctx, dcancel := context.WithTimeout(parent, time.Duration(cfg.RequestTimeoutSeconds)*time.Second)
		canonicalCode = ttsProviderAdoptAudio(dctx, *provider, j.SpoolPath, audioURL)
		dcancel()
	}
	if err == nil && sseWAV {
		// A complete SSE WAV may still carry streaming header lengths. Seal it
		// before settlement, since recovery archives the spool as is; a spool
		// that cannot be sealed stays untouched and fails strict archiving.
		_ = ttsFinalizeWAVFile(j.SpoolPath)
	}
	_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Update("provider_request_id", requestID).Error
	if confirmed {
		if e := ttsSettleUsageJob(db, j.ID, units, synthesisUsage, time.Now()); e != nil {
			ttsUnknown(j, "settlement_pending")
			return
		}
	} else {
		ttsUnknown(j, ttsProviderErrorCode(err, "provider_usage_unknown"))
		return
	}
	if err != nil {
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "failed", "error_code": ttsProviderErrorCode(err, "provider_or_spool_failed")}).Error
		_ = os.Remove(j.SpoolPath)
		ttsMessageStatus(j, "failed", nil)
		return
	}
	if j.Operation == "clone" {
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Update("status", "succeeded").Error
		_ = os.Remove(j.SpoolPath)
		return
	}
	if canonicalCode != "" {
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "failed", "error_code": canonicalCode}).Error
		_ = os.Remove(j.SpoolPath)
		ttsMessageStatus(j, "failed", nil)
		return
	}
	ttsArchive(parent, j, s)
}

// Keep only a bounded machine code for internal diagnostics, never error text.
func ttsProviderErrorCode(err error, fallback string) string {
	var providerError *ttsprovider.ProviderError
	if !errors.As(err, &providerError) {
		return fallback
	}
	var code strings.Builder
	for _, r := range providerError.Code {
		if code.Len() == 64 {
			break
		}
		switch {
		case r >= 'A' && r <= 'Z':
			code.WriteByte(byte(r + ('a' - 'A')))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			code.WriteByte(byte(r))
		default:
			code.WriteByte('_')
		}
	}
	if normalized := strings.Trim(code.String(), "_"); normalized != "" {
		return "provider_" + normalized
	}
	return fallback
}

// ttsAdoptProviderAudio replaces the realtime SSE spool with the provider's
// complete file only after that file passes strict media validation. The
// candidate is written beside the spool and never left behind on failure.
func ttsAdoptProviderAudio(ctx context.Context, client *ttsprovider.Client, spoolPath, audioURL string) string {
	archivePath := spoolPath + ".archive"
	f, err := os.OpenFile(archivePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "spool_unavailable"
	}
	_, err = client.DownloadAudio(ctx, audioURL, f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(archivePath)
		return "provider_audio_download_failed"
	}
	b, err := os.ReadFile(archivePath)
	if err != nil {
		_ = os.Remove(archivePath)
		return "spool_unavailable"
	}
	if _, err = ttsprovider.InspectMedia(b); err != nil {
		// Provider WAV headers may carry estimated lengths; only a complete
		// PCM16 RIFF/WAVE can be sealed, and it must pass strict validation.
		var finalized []byte
		if finalized, err = ttsprovider.FinalizePCM16WAV(b); err == nil {
			if err = os.WriteFile(archivePath, finalized, 0600); err != nil {
				_ = os.Remove(archivePath)
				return "spool_unavailable"
			}
			_, err = ttsprovider.InspectMedia(finalized)
		}
	}
	if err != nil {
		_ = os.Remove(archivePath)
		return "provider_audio_invalid_media"
	}
	if err = os.Rename(archivePath, spoolPath); err != nil {
		_ = os.Remove(archivePath)
		return "spool_unavailable"
	}
	return ""
}

// ttsFinalizeWAVFile seals a complete PCM16 WAV spool in place through a
// temporary file and rename. It never touches a file it cannot seal.
func ttsFinalizeWAVFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err = ttsprovider.InspectMedia(b); err == nil {
		return nil
	}
	finalized, err := ttsprovider.FinalizePCM16WAV(b)
	if err != nil {
		return err
	}
	tmp := path + ".finalize"
	if err = os.WriteFile(tmp, finalized, 0600); err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

func ttsUnknown(j *model.TTSJob, code string) {
	_ = model.GetDB().Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "usage_unknown", "usage_status": "unknown", "error_code": code}).Error
	ttsMessageStatus(j, "usage_unknown", nil)
}
func ttsArchive(ctx context.Context, j *model.TTSJob, s TTSSnapshot) {
	db := model.GetDB()
	if !TTSValidateSpool(j.SpoolPath) {
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "failed", "error_code": "invalid_spool"}).Error
		ttsMessageStatus(j, "failed", nil)
		return
	}
	b, err := os.ReadFile(j.SpoolPath)
	if err != nil {
		status := "storage_pending"
		if os.IsNotExist(err) {
			status = "failed"
		}
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": status, "error_code": "spool_unavailable"}).Error
		ttsMessageStatus(j, status, nil)
		return
	}
	media, err := ttsprovider.InspectMedia(b)
	if err != nil {
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "failed", "error_code": "unsupported_or_incomplete_media"}).Error
		_ = os.Remove(j.SpoolPath)
		ttsMessageStatus(j, "failed", nil)
		return
	}
	a, err := TTSPersistAudio(j.PayerUserID, j.ChannelID, j.ID, "job_audio", j.SpoolPath, media)
	if err != nil {
		_ = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "storage_pending", "error_code": "storage_failed"}).Error
		return
	}
	meta, _ := json.Marshal(media)
	j.ResourceID = a.ID
	j.MediaJSON = string(meta)
	if err = db.Model(&model.TTSJob{}).Where("id = ?", j.ID).Updates(map[string]any{"status": "succeeded", "resource_id": a.ID, "media_json": string(meta), "error_code": ""}).Error; err != nil {
		return
	}
	if j.Operation == "design" {
		_ = db.Model(&model.TTSVoice{}).Where("id = ?", j.VoiceID).Update("preview_resource_id", a.ID).Error
	}
	if j.Operation == "audition" || j.Operation == "message_synthesis" {
		cache := model.TTSCache{Fingerprint: s.Fingerprint, ResourceID: a.ID, Metadata: string(meta), Complete: true, ExpiresAt: time.Now().Add(30 * time.Minute)}
		// Expiry drops reuse eligibility, never the durable attachment reference.
		_ = db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "fingerprint"}}, DoUpdates: clause.AssignmentColumns([]string{"resource_id", "metadata", "complete", "expires_at"})}).Create(&cache).Error
	}
	ttsPublishMessage(ctx, j, s, a, media)
	_ = os.Remove(j.SpoolPath)
}

func ttsPublishMessage(ctx context.Context, j *model.TTSJob, s TTSSnapshot, a *model.AttachmentModel, media ttsprovider.Media) {
	if j.MessageID != "" && ttsMessageCurrent(j, s) {
		if !ttsMessageStatus(j, "ready", &protocol.MessageTTS{Status: "ready", AudioResourceID: a.ID, DurationMS: media.DurationMS, Format: media.Container, MessageRevision: int(j.MessageRevision)}) {
			return
		}
		if j.Status == "storage_pending" {
			return // Never announce while recovering storage; clients poll the state.
		}
		path, e := TTSResourcePath(a)
		stop := make(chan struct{})
		ttsTimelines.Lock()
		ttsTimelines.items[j.MessageID] = stop
		ttsTimelines.Unlock()
		// A lane includes its audio timeline, not just upstream generation time.
		// A realtime stream already spent it; then the archive is only announced.
		wait := time.Duration(0)
		if e == nil {
			ttsCallbacks.RLock()
			fn := ttsCallbacks.ready
			ttsCallbacks.RUnlock()
			if fn != nil && fn(*j, media, path, ctx.Value(ttsLiveContextKey{}) == true) {
				wait = time.Duration(media.DurationMS+300) * time.Millisecond
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		case <-stop:
			timer.Stop()
		}
		ttsTimelines.Lock()
		if ttsTimelines.items[j.MessageID] == stop {
			delete(ttsTimelines.items, j.MessageID)
		}
		ttsTimelines.Unlock()
	}
}
func ttsReleaseMessageJob(db *gorm.DB, j *model.TTSJob, status string) {
	if ttsReleaseJob(db, j.ID) == nil {
		ttsMessageStatusWithDB(db, j, status, nil)
	}
}
func ttsMessageStatus(j *model.TTSJob, status string, data *protocol.MessageTTS) bool {
	return ttsMessageStatusWithDB(model.GetDB(), j, status, data)
}
func ttsMessageStatusWithDB(db *gorm.DB, j *model.TTSJob, status string, data *protocol.MessageTTS) bool {
	if j.MessageID == "" {
		return false
	}
	updates := map[string]any{"tts_status": status}
	if data != nil {
		b, _ := json.Marshal(data)
		updates["tts_data"] = string(b)
	}
	r := db.Model(&model.MessageModel{}).Where("id = ? AND edit_count = ? AND tts_intent = ? AND deleted_at IS NULL AND is_deleted = ? AND (is_revoked = ? OR is_revoked IS NULL)", j.MessageID, j.MessageRevision, j.Snapshot, false, false).Updates(updates)
	return r.Error == nil && r.RowsAffected == 1
}
