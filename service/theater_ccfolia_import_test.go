package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

type visualMediaTestRunner struct{ fail error }

func (r visualMediaTestRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	return nil, os.WriteFile(args[len(args)-1], []byte("test-webm"), 0o600)
}

func ccfoliaEffectFixture(t *testing.T, shared bool) (string, string) {
	t.Helper()
	data := makeTestAPNG(t, 2, 1, 2, 0)
	hash := sha256.Sum256(data)
	ref := hex.EncodeToString(hash[:]) + ".png"
	items := map[string]any{}
	if shared {
		items["shared"] = map[string]any{"imageUrl": ref, "width": 4, "height": 3, "visible": true}
	}
	raw := worldTheaterPayload(t, map[string]any{
		"meta": map[string]any{"version": ccfoliaBackupVersion},
		"entities": map[string]any{
			"room":  map[string]any{"fieldWidth": 30, "fieldHeight": 18, "fieldObjectFit": "cover", "gridSize": 1},
			"items": items, "scenes": map[string]any{},
			"effects": map[string]any{
				"later": map[string]any{"name": "Later", "imageUrl": ref, "soundRef": nil, "order": 2, "active": false, "playTime": 1786895287260},
				"first": map[string]any{"name": "First", "imageUrl": ref, "soundRef": nil, "order": 1, "active": false, "playTime": 1786895287260},
			},
		},
		"resources": map[string]any{ref: map[string]any{"type": "image/png"}},
	})
	root := t.TempDir()
	for name, content := range map[string][]byte{"__data.json": raw, ref: data} {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, ref
}

func TestCCFOLIAEffectsNativeAnimationWithoutEncoder(t *testing.T) {
	root, ref := ccfoliaEffectFixture(t, false)
	backup, err := loadCCFOLIABackup(root)
	if err != nil {
		t.Fatal(err)
	}
	resources, targets, animated, warnings, err := loadCCFOLIAResources(root, &backup)
	if err != nil || len(warnings) != 0 || animated != 1 || !targets[ref].OriginalOnly {
		t.Fatalf("load: animated=%d warnings=%v targets=%v err=%v", animated, warnings, targets, err)
	}
	for _, toolchain := range []MediaToolchain{{}, {FFmpegPath: "missing-vp9-encoder"}} {
		processor := NewVisualMediaProcessor(utils.TheaterMediaConfig{}, toolchain, visualMediaTestRunner{fail: errors.New("effect must not invoke ffmpeg")})
		if err := prepareCCFOLIAAnimatedResources(t.Context(), root, resources, targets, processor); err != nil {
			t.Fatal(err)
		}
	}
	conversion, err := convertCCFOLIABackup(backup, "world", targets)
	if err != nil {
		t.Fatal(err)
	}
	if conversion.Summary.Effects != 2 || len(conversion.Summary.UnsupportedEntities) != 0 || len(conversion.Summary.Warnings) != 0 {
		t.Fatalf("summary = %#v", conversion.Summary)
	}
	if err := validateTheaterSharedSnapshot(conversion.Snapshot); err != nil {
		t.Fatal(err)
	}
	for _, object := range conversion.Snapshot.PersistentObjects {
		if object.Kind != "effect" || object.SceneID != nil || !object.Visible {
			t.Fatalf("effect must be globally playable: %#v", object)
		}
		if err := validateTheaterEffectContent(object.Content); err != nil {
			t.Fatal(err)
		}
		var content struct {
			Effect struct {
				DurationMS int64    `json:"durationMs"`
				Keywords   []string `json:"keywords"`
				Media      struct {
					URL      string `json:"url"`
					MimeType string `json:"mimeType"`
				} `json:"media"`
			} `json:"effect"`
		}
		if err := json.Unmarshal(object.Content, &content); err != nil {
			t.Fatal(err)
		}
		if content.Effect.DurationMS != 1000 || len(content.Effect.Keywords) != 0 || content.Effect.Media.MimeType != "image/apng" || strings.Contains(content.Effect.Media.URL, "/variants/") {
			t.Fatalf("native effect content = %s", object.Content)
		}
		if object.Name == "First" && object.OrderKey != "1" || object.Name == "Later" && object.OrderKey != "2" {
			t.Fatalf("effect source order lost: %#v", object)
		}
		if !strings.Contains(string(object.Metadata), "1786895287260") || !strings.Contains(string(object.Metadata), `\"active\":false`) {
			t.Fatalf("source playback metadata lost: %s", object.Metadata)
		}
	}
}

func TestCCFOLIAEffectSharedImageUsesOriginalAnimatedMedia(t *testing.T) {
	root, ref := ccfoliaEffectFixture(t, true)
	backup, err := loadCCFOLIABackup(root)
	if err != nil {
		t.Fatal(err)
	}
	resources, targets, _, _, err := loadCCFOLIAResources(root, &backup)
	if err != nil {
		t.Fatal(err)
	}
	if targets[ref].OriginalOnly {
		t.Fatal("shared stage image must retain its processing path")
	}
	processor := NewVisualMediaProcessor(utils.TheaterMediaConfig{}, MediaToolchain{FFmpegPath: "fake-ffmpeg"}, visualMediaTestRunner{})
	if err := prepareCCFOLIAAnimatedResources(t.Context(), root, resources, targets, processor); err != nil {
		t.Fatal(err)
	}
	conversion, err := convertCCFOLIABackup(backup, "world", targets)
	if err != nil {
		t.Fatal(err)
	}
	effects := 0
	for _, object := range conversion.Snapshot.PersistentObjects {
		if object.Kind != "effect" {
			continue
		}
		effects++
		content := string(object.Content)
		if strings.Contains(content, "video/webm") || strings.Contains(content, "/variants/display/content") || !strings.Contains(content, "image/apng") {
			t.Fatalf("shared animated media must use original image: kind=%s content=%s", object.Kind, object.Content)
		}
	}
	if effects != 2 {
		t.Fatalf("effect count = %d, want 2", effects)
	}
}

func TestCCFOLIAStageAnimationWithoutEncoderUsesOriginalOnce(t *testing.T) {
	root, ref := ccfoliaEffectFixture(t, true)
	backup, err := loadCCFOLIABackup(root)
	if err != nil {
		t.Fatal(err)
	}
	resources, targets, _, _, err := loadCCFOLIAResources(root, &backup)
	if err != nil {
		t.Fatal(err)
	}
	processor := NewVisualMediaProcessor(utils.TheaterMediaConfig{}, MediaToolchain{}, nil)
	if err := prepareCCFOLIAAnimatedResources(t.Context(), root, resources, targets, processor); err != nil {
		t.Fatal(err)
	}
	if len(resources[0].Variants) != 0 || targets[ref].PlaybackVariant != "" || targets[ref].MimeType != "image/apng" {
		t.Fatalf("original must not be imported twice: resource=%#v target=%#v", resources[0], targets[ref])
	}
}

func TestCCFOLIAEffectResourceRecoveryAndMissingReference(t *testing.T) {
	root, ref := ccfoliaEffectFixture(t, false)
	backup, err := loadCCFOLIABackup(root)
	if err != nil {
		t.Fatal(err)
	}
	delete(backup.Resources, ref)
	_, _, _, warnings, err := loadCCFOLIAResources(root, &backup)
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "已按文件恢复") {
		t.Fatalf("effect resource recovery: warnings=%v err=%v", warnings, err)
	}
	missing := "missing.png"
	backup.Entities.Effects["first"] = ccfoliaEffect{ImageURL: &missing}
	if _, _, _, _, err := loadCCFOLIAResources(root, &backup); err == nil || !strings.Contains(err.Error(), "entities.effects.first.imageUrl") {
		t.Fatalf("missing effect asset accepted: %v", err)
	}
}

// The user's private reproduction package is optional; deterministic fixtures
// above cover the same contracts when the archive is not available in CI.
func TestCCFOLIAGoldenRecordImportCompatibility(t *testing.T) {
	source := filepath.Join("..", "docs", "最后的黄金唱片ccf.zip")
	if _, err := os.Stat(source); os.IsNotExist(err) {
		t.Skip("private CCFOLIA reproduction package is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	cases := map[string]MediaToolchain{"without-ffmpeg": {}}
	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		cases["with-ffmpeg"] = MediaToolchain{FFmpegPath: ffmpeg}
	}
	for name, toolchain := range cases {
		t.Run(name, func(t *testing.T) {
			actorID, worldID, _ := initWorldTheaterServiceTest(t)
			previousMedia := theaterMedia
			theaterMedia = &theaterMediaService{toolchain: toolchain}
			t.Cleanup(func() { theaterMedia = previousMedia })
			theaterPackageWorkerState.Lock()
			previousConfig := theaterPackageWorkerState.config
			theaterPackageWorkerState.config.StorageDir = t.TempDir()
			theaterPackageWorkerState.Unlock()
			t.Cleanup(func() {
				theaterPackageWorkerState.Lock()
				theaterPackageWorkerState.config = previousConfig
				theaterPackageWorkerState.Unlock()
			})
			if _, err := InitStorageManager(utils.StorageConfig{Mode: utils.StorageModeLocal, Local: utils.LocalStorageConfig{UploadDir: t.TempDir(), TempDir: t.TempDir()}}); err != nil {
				t.Fatal(err)
			}
			job := &model.TheaterPackageJobModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
				Type:              model.TheaterPackageJobTypeImportCCFOLIA, Status: model.TheaterPackageJobStatusRunning,
				ActorUserID: actorID, TargetWorldID: worldID, InputFilePath: source, OriginalName: filepath.Base(source),
			}
			if err := model.GetDB().Create(job).Error; err != nil {
				t.Fatal(err)
			}
			summary, err := importCCFOLIATheaterPackage(t.Context(), job)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Effects != 6 || summary.Resources != 17 || summary.Objects != 15 || summary.AnimatedResources != 9 || len(summary.Warnings) != 0 {
				t.Fatalf("golden record summary = %#v", summary)
			}
			var effects []model.TheaterObjectModel
			if err := model.GetDB().Where("kind = ?", "effect").Find(&effects).Error; err != nil {
				t.Fatal(err)
			}
			if len(effects) != 6 {
				t.Fatalf("persisted effects = %d", len(effects))
			}
			for _, effect := range effects {
				if !effect.Visible || effect.SceneID != "" || !strings.Contains(effect.ContentJSON, `"durationMs":4950`) || !strings.Contains(effect.ContentJSON, `"mimeType":"image/apng"`) || strings.Contains(effect.ContentJSON, "/variants/") {
					t.Fatalf("persisted native effect = %#v", effect)
				}
				var content map[string]any
				if err := json.Unmarshal([]byte(effect.ContentJSON), &content); err != nil {
					t.Fatal(err)
				}
				media := content["effect"].(map[string]any)["media"].(map[string]any)
				var resource model.TheaterResourceModel
				if err := model.GetDB().First(&resource, "id = ?", media["resourceId"]).Error; err != nil {
					t.Fatal(err)
				}
				if resource.ReferenceCount != 1 || resource.Status != "ready" {
					t.Fatalf("effect resource is not held and ready: %#v", resource)
				}
			}
			if _, err := importCCFOLIATheaterPackage(t.Context(), job); err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := model.GetDB().Model(&model.TheaterObjectModel{}).Where("kind = ?", "effect").Count(&count).Error; err != nil || count != 6 {
				t.Fatalf("repeated job duplicated effects: count=%d err=%v", count, err)
			}
		})
	}
}
