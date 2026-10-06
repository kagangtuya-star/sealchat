package protocol

import (
	"errors"
	"fmt"
	"math"
)

// MediaFxSpec mirrors the frontend Media FX v1 spec (ui/src/features/media-fx/media-fx.ts).
// It is persisted inside theater presentation layers and styles, so it only carries
// whitelisted presets and bounded numbers; renderers never receive raw CSS.
// Keep the ranges below in sync with media-fx.ts and media-fx-schema.ts.
const MediaFxVersion = 1

type MediaFxMotionPreset string

const (
	MediaFxMotionNone    MediaFxMotionPreset = "none"
	MediaFxMotionShake   MediaFxMotionPreset = "shake"
	MediaFxMotionShakeX  MediaFxMotionPreset = "shake-x"
	MediaFxMotionShakeY  MediaFxMotionPreset = "shake-y"
	MediaFxMotionBreathe MediaFxMotionPreset = "breathe"
	MediaFxMotionFloat   MediaFxMotionPreset = "float"
	MediaFxMotionDrift   MediaFxMotionPreset = "drift"
	MediaFxMotionZoom    MediaFxMotionPreset = "zoom"
)

const (
	minMediaFxDurationMS int64 = 300
	maxMediaFxDurationMS int64 = 20_000
	// Matches the frontend FILTER_EPSILON used to decide "no change".
	mediaFxFilterEpsilon = 0.001
)

type MediaFxMotion struct {
	Preset     MediaFxMotionPreset `json:"preset"`
	Intensity  float64             `json:"intensity"`
	DurationMS int64               `json:"durationMs"`
	Loop       bool                `json:"loop"`
}

type MediaFxFilter struct {
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Grayscale  float64 `json:"grayscale"`
	Sepia      float64 `json:"sepia"`
	HueRotate  float64 `json:"hueRotate"`
	BlurPx     float64 `json:"blurPx"`
}

type MediaFxSpec struct {
	Version int           `json:"version"`
	Motion  MediaFxMotion `json:"motion"`
	Filter  MediaFxFilter `json:"filter"`
}

type mediaFxFilterField struct {
	name                    string
	value, minimum, maximum float64
	defaultValue            float64
}

func (filter MediaFxFilter) fields() []mediaFxFilterField {
	return []mediaFxFilterField{
		{"brightness", filter.Brightness, 0.25, 2, 1},
		{"contrast", filter.Contrast, 0.25, 2, 1},
		{"saturation", filter.Saturation, 0, 2, 1},
		{"grayscale", filter.Grayscale, 0, 1, 0},
		{"sepia", filter.Sepia, 0, 1, 0},
		{"hueRotate", filter.HueRotate, -180, 180, 0},
		{"blurPx", filter.BlurPx, 0, 24, 0},
	}
}

func validMediaFxMotionPreset(preset MediaFxMotionPreset) bool {
	switch preset {
	case MediaFxMotionNone, MediaFxMotionShake, MediaFxMotionShakeX, MediaFxMotionShakeY,
		MediaFxMotionBreathe, MediaFxMotionFloat, MediaFxMotionDrift, MediaFxMotionZoom:
		return true
	}
	return false
}

// validateMediaFx accepts nil (no effect) and otherwise requires a complete v1 spec.
func validateMediaFx(spec *MediaFxSpec, path string) error {
	if spec == nil {
		return nil
	}
	var problems []error
	if spec.Version != MediaFxVersion {
		problems = append(problems, fmt.Errorf("%s.version must be %d", path, MediaFxVersion))
	}
	if !validMediaFxMotionPreset(spec.Motion.Preset) {
		problems = append(problems, fmt.Errorf("%s.motion.preset is invalid", path))
	}
	if !finiteInRange(spec.Motion.Intensity, 0, 1) {
		problems = append(problems, fmt.Errorf("%s.motion.intensity must be finite and between 0 and 1", path))
	}
	if spec.Motion.DurationMS < minMediaFxDurationMS || spec.Motion.DurationMS > maxMediaFxDurationMS {
		problems = append(problems, fmt.Errorf("%s.motion.durationMs must be between %d and %d", path, minMediaFxDurationMS, maxMediaFxDurationMS))
	}
	for _, field := range spec.Filter.fields() {
		if !finiteInRange(field.value, field.minimum, field.maximum) {
			problems = append(problems, fmt.Errorf("%s.filter.%s must be finite and between %g and %g", path, field.name, field.minimum, field.maximum))
		}
	}
	return errors.Join(problems...)
}

func mediaFxHasContent(spec *MediaFxSpec) bool {
	if spec == nil {
		return false
	}
	if spec.Motion.Preset != MediaFxMotionNone && spec.Motion.Intensity > 0 {
		return true
	}
	for _, field := range spec.Filter.fields() {
		if math.Abs(field.value-field.defaultValue) >= mediaFxFilterEpsilon {
			return true
		}
	}
	return false
}

// mediaFxEqual compares effect content, never pointer identity. A missing spec and an
// all-default spec both mean "no effect" and are therefore equal.
func mediaFxEqual(left, right *MediaFxSpec) bool {
	leftActive, rightActive := mediaFxHasContent(left), mediaFxHasContent(right)
	if !leftActive || !rightActive {
		return leftActive == rightActive
	}
	return *left == *right
}

func cloneMediaFx(spec *MediaFxSpec) *MediaFxSpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	return &clone
}
