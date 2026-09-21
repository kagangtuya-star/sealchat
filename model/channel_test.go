package model

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sealchat/utils"
)

func TestChannelToProtocolTypeIncludesBotCommandPrefixes(t *testing.T) {
	channel := (&ChannelModel{
		StringPKBaseModel: StringPKBaseModel{ID: "channel-1"},
		WorldID:           "world-1",
		Name:              "测试频道",
		DefaultDiceExpr:   "d20",
	}).ToProtocolType()

	if channel == nil {
		t.Fatalf("expected protocol channel")
	}
	if len(channel.BotCommandPrefixes) == 0 {
		t.Fatalf("expected bot command prefixes to be exposed")
	}
	if channel.BotCommandPrefixes[0] != "." {
		t.Fatalf("expected first bot command prefix to be '.', got %#v", channel.BotCommandPrefixes)
	}
}

func initChannelLatestReadTestDB(t *testing.T) {
	t.Helper()
	DBInit(&utils.AppConfig{
		DSN: fmt.Sprintf("file:model-channel-latest-read-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	})
}

func createChannelLatestReadForTest(t *testing.T, channelID, userID string, messageTime int64, mentionTime *int64) ChannelLatestReadModel {
	t.Helper()
	record := ChannelLatestReadModel{
		ChannelId:         channelID,
		UserId:            userID,
		MessageTime:       messageTime,
		LatestMentionTime: mentionTime,
	}
	if err := GetDB().Create(&record).Error; err != nil {
		t.Fatalf("create channel latest read: %v", err)
	}
	return record
}

func loadChannelLatestReadForTest(t *testing.T, channelID, userID string) ChannelLatestReadModel {
	t.Helper()
	var record ChannelLatestReadModel
	if err := GetDB().Where("channel_id = ? AND user_id = ?", channelID, userID).First(&record).Error; err != nil {
		t.Fatalf("load channel latest read: %v", err)
	}
	return record
}

func TestChannelLatestReadMentionWatermark(t *testing.T) {
	initChannelLatestReadTestDB(t)

	t.Run("read init leaves mention watermark pending", func(t *testing.T) {
		channelID, userID := "channel-init", "user-init"
		if err := ChannelReadInit(channelID, userID); err != nil {
			t.Fatal(err)
		}
		record := loadChannelLatestReadForTest(t, channelID, userID)
		if record.LatestMentionTime != nil {
			t.Fatalf("LatestMentionTime = %v, want nil", record.LatestMentionTime)
		}
	})

	t.Run("batch read init leaves mention watermark pending", func(t *testing.T) {
		channelID := "channel-init-batch"
		userIDs := []string{"user-init-batch-1", "user-init-batch-2"}
		if err := ChannelReadInitInBatches(channelID, userIDs); err != nil {
			t.Fatal(err)
		}
		for _, userID := range userIDs {
			record := loadChannelLatestReadForTest(t, channelID, userID)
			if record.LatestMentionTime != nil {
				t.Fatalf("LatestMentionTime for %s = %v, want nil", userID, record.LatestMentionTime)
			}
		}
	})

	t.Run("read set creates initialized mention watermark", func(t *testing.T) {
		channelID, userID := "channel-read-set-init", "user-read-set-init"
		if err := ChannelReadSet(channelID, userID); err != nil {
			t.Fatal(err)
		}
		record := loadChannelLatestReadForTest(t, channelID, userID)
		if record.LatestMentionTime == nil || *record.LatestMentionTime != 0 {
			t.Fatalf("LatestMentionTime = %v, want pointer to 0", record.LatestMentionTime)
		}
	})

	t.Run("watermark comparison and read progression", func(t *testing.T) {
		unreadChannel := "channel-watermark-unread"
		readChannel := "channel-watermark-read"
		userID := "user-watermark"
		createChannelLatestReadForTest(t, unreadChannel, userID, 100, int64Ptr(200))
		createChannelLatestReadForTest(t, readChannel, userID, 200, int64Ptr(200))

		state, err := ChannelUnreadStateFetch([]string{unreadChannel, readChannel}, userID)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Mentions[unreadChannel] {
			t.Fatalf("expected %s to have an unread mention", unreadChannel)
		}
		if state.Mentions[readChannel] {
			t.Fatalf("did not expect %s to have an unread mention", readChannel)
		}

		if err := ChannelReadSet(unreadChannel, userID); err != nil {
			t.Fatal(err)
		}
		record := loadChannelLatestReadForTest(t, unreadChannel, userID)
		if record.LatestMentionTime == nil || *record.LatestMentionTime != 200 {
			t.Fatalf("ChannelReadSet changed mention watermark to %v", record.LatestMentionTime)
		}
		state, err = ChannelUnreadStateFetch([]string{unreadChannel}, userID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Mentions[unreadChannel] {
			t.Fatal("mention remained unread after ChannelReadSet advanced MessageTime")
		}
	})

	t.Run("advance selected users", func(t *testing.T) {
		channelID, otherChannelID := "channel-users", "channel-users-other"
		createChannelLatestReadForTest(t, channelID, "sender-users", 0, int64Ptr(0))
		createChannelLatestReadForTest(t, channelID, "target-users", 0, int64Ptr(0))
		createChannelLatestReadForTest(t, channelID, "other-users", 0, int64Ptr(0))
		createChannelLatestReadForTest(t, otherChannelID, "target-users", 0, int64Ptr(0))

		if err := ChannelMentionAdvanceForUsers(channelID, "sender-users", []string{"sender-users", "target-users"}, 200); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct {
			channelID string
			userID    string
			want      int64
		}{
			{channelID, "sender-users", 0},
			{channelID, "target-users", 200},
			{channelID, "other-users", 0},
			{otherChannelID, "target-users", 0},
		} {
			record := loadChannelLatestReadForTest(t, test.channelID, test.userID)
			if record.LatestMentionTime == nil || *record.LatestMentionTime != test.want {
				t.Fatalf("watermark for %s/%s = %v, want %d", test.channelID, test.userID, record.LatestMentionTime, test.want)
			}
		}
	})

	t.Run("advance channel excludes sender and is monotonic", func(t *testing.T) {
		channelID := "channel-all"
		createChannelLatestReadForTest(t, channelID, "sender-all", 0, int64Ptr(0))
		createChannelLatestReadForTest(t, channelID, "target-all-1", 0, int64Ptr(0))
		createChannelLatestReadForTest(t, channelID, "target-all-2", 0, int64Ptr(0))

		if err := ChannelMentionAdvanceForChannel(channelID, "sender-all", 200); err != nil {
			t.Fatal(err)
		}
		if err := ChannelMentionAdvanceForChannel(channelID, "sender-all", 100); err != nil {
			t.Fatal(err)
		}
		for _, userID := range []string{"target-all-1", "target-all-2"} {
			record := loadChannelLatestReadForTest(t, channelID, userID)
			if record.LatestMentionTime == nil || *record.LatestMentionTime != 200 {
				t.Fatalf("watermark for %s = %v, want 200", userID, record.LatestMentionTime)
			}
		}
		sender := loadChannelLatestReadForTest(t, channelID, "sender-all")
		if sender.LatestMentionTime == nil || *sender.LatestMentionTime != 0 {
			t.Fatalf("sender watermark = %v, want 0", sender.LatestMentionTime)
		}
	})

	t.Run("legacy null falls back once and backfills", func(t *testing.T) {
		channelID, userID := "channel-legacy", "user-legacy"
		record := createChannelLatestReadForTest(t, channelID, userID, 100, nil)
		message := MessageModel{
			StringPKBaseModel: StringPKBaseModel{ID: "message-legacy", CreatedAt: time.UnixMilli(150)},
			ChannelID:         channelID,
			UserID:            "sender-legacy",
			Content:           "legacy mention",
		}
		if err := GetDB().Create(&message).Error; err != nil {
			t.Fatal(err)
		}
		mention := MentionModel{
			ReceiverId:  userID,
			SenderId:    message.UserID,
			LocPostType: "channel",
			LocPostID:   channelID,
			RelatedType: "message",
			RelatedID:   message.ID,
		}
		if err := GetDB().Create(&mention).Error; err != nil {
			t.Fatal(err)
		}

		state, err := ChannelUnreadStateFetch([]string{channelID}, userID)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Mentions[channelID] {
			t.Fatal("legacy unread mention was not found")
		}
		stored := loadChannelLatestReadForTest(t, channelID, userID)
		if stored.LatestMentionTime == nil || *stored.LatestMentionTime != record.MessageTime+1 {
			t.Fatalf("backfilled watermark = %v, want %d", stored.LatestMentionTime, record.MessageTime+1)
		}

		if err := GetDB().Where("id = ?", mention.ID).Delete(&MentionModel{}).Error; err != nil {
			t.Fatal(err)
		}
		state, err = ChannelUnreadStateFetch([]string{channelID}, userID)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Mentions[channelID] {
			t.Fatal("initialized watermark unexpectedly fell back to the deleted mention row")
		}
	})

	t.Run("historical mention survives first read init", func(t *testing.T) {
		channelID, userID := "channel-first-init", "user-first-init"
		message := MessageModel{
			StringPKBaseModel: StringPKBaseModel{ID: "message-first-init", CreatedAt: time.UnixMilli(150)},
			ChannelID:         channelID,
			UserID:            "sender-first-init",
			Content:           "historical mention",
		}
		if err := GetDB().Create(&message).Error; err != nil {
			t.Fatal(err)
		}
		mention := MentionModel{
			ReceiverId:  userID,
			SenderId:    message.UserID,
			LocPostType: "channel",
			LocPostID:   channelID,
			RelatedType: "message",
			RelatedID:   message.ID,
		}
		if err := GetDB().Create(&mention).Error; err != nil {
			t.Fatal(err)
		}
		if err := ChannelReadInit(channelID, userID); err != nil {
			t.Fatal(err)
		}
		before := loadChannelLatestReadForTest(t, channelID, userID)
		if before.LatestMentionTime != nil {
			t.Fatalf("watermark before first fetch = %v, want nil", before.LatestMentionTime)
		}

		state, err := ChannelUnreadStateFetch([]string{channelID}, userID)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Mentions[channelID] {
			t.Fatal("historical mention was lost after first read init")
		}
		after := loadChannelLatestReadForTest(t, channelID, userID)
		if after.LatestMentionTime == nil {
			t.Fatal("first fetch did not initialize mention watermark")
		}
	})

	t.Run("legacy backfill does not overwrite concurrent advance", func(t *testing.T) {
		channelID, userID := "channel-race", "user-race"
		record := createChannelLatestReadForTest(t, channelID, userID, 100, nil)
		if err := ChannelMentionAdvanceForUsers(channelID, "sender-race", []string{userID}, 300); err != nil {
			t.Fatal(err)
		}
		updated, err := channelMentionBackfillIfNull(record.ID, record.MentionStateVersion, 0)
		if err != nil {
			t.Fatal(err)
		}
		if updated {
			t.Fatal("legacy backfill overwrote a concurrently advanced watermark")
		}
		stored := loadChannelLatestReadForTest(t, channelID, userID)
		if stored.LatestMentionTime == nil || *stored.LatestMentionTime != 300 {
			t.Fatalf("watermark after competing updates = %v, want 300", stored.LatestMentionTime)
		}
	})

	t.Run("legacy backfill does not overwrite invalidation revision", func(t *testing.T) {
		channelID, userID := "channel-invalidation-race", "user-invalidation-race"
		record := createChannelLatestReadForTest(t, channelID, userID, 100, nil)
		if err := GetDB().Model(&ChannelLatestReadModel{}).
			Where("id = ?", record.ID).
			Update("mention_state_version", 3).Error; err != nil {
			t.Fatal(err)
		}
		record = loadChannelLatestReadForTest(t, channelID, userID)
		if err := ChannelMentionInvalidateForUsers(channelID, "sender-invalidation-race", []string{userID}); err != nil {
			t.Fatal(err)
		}
		invalidated := loadChannelLatestReadForTest(t, channelID, userID)
		if invalidated.LatestMentionTime != nil || invalidated.MentionStateVersion != 4 {
			t.Fatalf("invalidated state = watermark %v version %d, want nil/4", invalidated.LatestMentionTime, invalidated.MentionStateVersion)
		}

		updated, err := channelMentionBackfillIfNull(record.ID, record.MentionStateVersion, record.MessageTime+1)
		if err != nil {
			t.Fatal(err)
		}
		if updated {
			t.Fatal("legacy backfill overwrote a newer invalidation revision")
		}
		stored := loadChannelLatestReadForTest(t, channelID, userID)
		if stored.LatestMentionTime != nil || stored.MentionStateVersion != 4 {
			t.Fatalf("state after stale backfill = watermark %v version %d, want nil/4", stored.LatestMentionTime, stored.MentionStateVersion)
		}
	})

	t.Run("legacy backfill accepts current revision", func(t *testing.T) {
		channelID, userID := "channel-current-revision", "user-current-revision"
		record := createChannelLatestReadForTest(t, channelID, userID, 100, nil)
		updated, err := channelMentionBackfillIfNull(record.ID, record.MentionStateVersion, 101)
		if err != nil {
			t.Fatal(err)
		}
		if !updated {
			t.Fatal("legacy backfill rejected the current revision")
		}
		stored := loadChannelLatestReadForTest(t, channelID, userID)
		if stored.LatestMentionTime == nil || *stored.LatestMentionTime != 101 {
			t.Fatalf("watermark after current revision backfill = %v, want 101", stored.LatestMentionTime)
		}
	})

	t.Run("invalidate leaves already read watermark initialized", func(t *testing.T) {
		channelID, userID := "channel-read-invalidate", "user-read-invalidate"
		createChannelLatestReadForTest(t, channelID, userID, 300, int64Ptr(300))
		if err := ChannelMentionInvalidateForUsers(channelID, "sender-read-invalidate", []string{userID}); err != nil {
			t.Fatal(err)
		}
		stored := loadChannelLatestReadForTest(t, channelID, userID)
		if stored.LatestMentionTime == nil || *stored.LatestMentionTime != 300 || stored.MentionStateVersion != 0 {
			t.Fatalf("read state changed by invalidation: watermark %v version %d", stored.LatestMentionTime, stored.MentionStateVersion)
		}
	})

	t.Run("invalidate resets watermark and deleted mention falls back to read", func(t *testing.T) {
		channelID, userID := "channel-invalidate", "user-invalidate"
		createChannelLatestReadForTest(t, channelID, userID, 100, int64Ptr(300))
		message := MessageModel{
			StringPKBaseModel: StringPKBaseModel{ID: "message-invalidate", CreatedAt: time.UnixMilli(200)},
			ChannelID:         channelID,
			UserID:            "sender-invalidate",
			Content:           "deleted mention",
		}
		if err := GetDB().Create(&message).Error; err != nil {
			t.Fatal(err)
		}
		if err := GetDB().Create(&MentionModel{
			ReceiverId:  userID,
			SenderId:    message.UserID,
			LocPostType: "channel",
			LocPostID:   channelID,
			RelatedType: "message",
			RelatedID:   message.ID,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := GetDB().Model(&message).Update("is_deleted", true).Error; err != nil {
			t.Fatal(err)
		}
		if err := ChannelMentionInvalidateForUsers(channelID, message.UserID, []string{userID}); err != nil {
			t.Fatal(err)
		}
		invalidated := loadChannelLatestReadForTest(t, channelID, userID)
		if invalidated.LatestMentionTime != nil || invalidated.MentionStateVersion != 1 {
			t.Fatalf("invalidated state = watermark %v version %d, want nil/1", invalidated.LatestMentionTime, invalidated.MentionStateVersion)
		}

		state, err := ChannelUnreadStateFetch([]string{channelID}, userID)
		if err != nil {
			t.Fatal(err)
		}
		if state.Mentions[channelID] {
			t.Fatal("deleted mention remained unread after invalidation fallback")
		}
		backfilled := loadChannelLatestReadForTest(t, channelID, userID)
		if backfilled.LatestMentionTime == nil || *backfilled.LatestMentionTime != 0 || backfilled.MentionStateVersion != 1 {
			t.Fatalf("backfilled state = watermark %v version %d, want 0/1", backfilled.LatestMentionTime, backfilled.MentionStateVersion)
		}
	})
}

func TestChannelMentionMigrationSingleflight(t *testing.T) {
	initChannelLatestReadTestDB(t)
	channelID, userID := "channel-singleflight", "user-singleflight"
	createChannelLatestReadForTest(t, channelID, userID, 0, nil)

	originalFetch := channelUnreadMentionFetchFunc
	channelMentionMigrationCalls = sync.Map{}
	t.Cleanup(func() {
		channelUnreadMentionFetchFunc = originalFetch
		channelMentionMigrationCalls = sync.Map{}
	})

	const workers = 10
	var calls atomic.Int32
	entered := make(chan struct{}, workers)
	release := make(chan struct{})
	channelUnreadMentionFetchFunc = func(channelIDs []string, updateTimes []time.Time, gotUserID string) (map[string]bool, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return map[string]bool{channelID: true}, nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ChannelUnreadStateFetch([]string{channelID}, userID)
			errs <- err
		}()
	}

	<-entered
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent unread fetch failed: %v", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("fallback calls = %d, want 1", got)
	}
	stored := loadChannelLatestReadForTest(t, channelID, userID)
	if stored.LatestMentionTime == nil {
		t.Fatal("singleflight migration did not initialize watermark")
	}
}
