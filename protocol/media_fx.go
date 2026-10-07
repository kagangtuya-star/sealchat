package protocol

import (
	"errors"
	"fmt"
	"math"
)

// MediaFxSpec mirrors the frontend Media FX spec (ui/src/features/media-fx/media-fx.ts).
// It is persisted inside theater presentation layers and styles, so it only carries
// whitelisted presets and bounded numbers; renderers never receive raw CSS or shaders.
// Keep the ranges below in sync with media-fx.ts and media-fx-schema.ts.
//
// v2 adds the required `advanced` object. Stored v1 specs stay valid as they are and
// are upgraded lazily by clients on their next save; there is no data migration.
const (
	MediaFxVersion       = 2
	LegacyMediaFxVersion = 1
)

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

// MediaFxAdvanced holds normalized 0..1 strengths; renderers map them to pixels.
type MediaFxAdvanced struct {
	Pixelate float64 `json:"pixelate"`
	RGBSplit float64 `json:"rgbSplit"`
	Scanline float64 `json:"scanline"`
}

// Advanced is a pointer on purpose: v1 specs must not carry it, while v2 specs must,
// so the validator can tell a v1 document from an incomplete v2 one.
type MediaFxSpec struct {
	Version  int              `json:"version"`
	Motion   MediaFxMotion    `json:"motion"`
	Filter   MediaFxFilter    `json:"filter"`
	Advanced *MediaFxAdvanced `json:"advanced,omitempty"`
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

func (advanced MediaFxAdvanced) fields() []mediaFxFilterField {
	return []mediaFxFilterField{
		{"pixelate", advanced.Pixelate, 0, 1, 0},
		{"rgbSplit", advanced.RGBSplit, 0, 1, 0},
		{"scanline", advanced.Scanline, 0, 1, 0},
	}
}

// normalizedAdvanced treats a missing (v1) advanced as all zero.
func (spec *MediaFxSpec) normalizedAdvanced() MediaFxAdvanced {
	if spec == nil || spec.Advanced == nil {
		return MediaFxAdvanced{}
	}
	return *spec.Advanced
}

func validMediaFxMotionPreset(preset MediaFxMotionPreset) bool {
	switch preset {
	case MediaFxMotionNone, MediaFxMotionShake, MediaFxMotionShakeX, MediaFxMotionShakeY,
		MediaFxMotionBreathe, MediaFxMotionFloat, MediaFxMotionDrift, MediaFxMotionZoom:
		return true
	}
	return false
}

// validateMediaFx accepts nil (no effect) and otherwise requires a complete v1 spec
// (without advanced) or a complete v2 spec (with advanced).
func validateMediaFx(spec *MediaFxSpec, path string) error {
	if spec == nil {
		return nil
	}
	var problems []error
	switch spec.Version {
	case LegacyMediaFxVersion:
		if spec.Advanced != nil {
			problems = append(problems, fmt.Errorf("%s.advanced is not allowed in version %d", path, LegacyMediaFxVersion))
		}
	case MediaFxVersion:
		if spec.Advanced == nil {
			problems = append(problems, fmt.Errorf("%s.advanced is required in version %d", path, MediaFxVersion))
		} else {
			for _, field := range spec.Advanced.fields() {
				if !finiteInRange(field.value, field.minimum, field.maximum) {
					problems = append(problems, fmt.Errorf("%s.advanced.%s must be finite and between %g and %g", path, field.name, field.minimum, field.maximum))
				}
			}
		}
	default:
		problems = append(problems, fmt.Errorf("%s.version must be %d or %d", path, LegacyMediaFxVersion, MediaFxVersion))
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

// ValidateMediaFx exposes the single Media FX validator to other packages that embed
// MediaFxSpec in their own documents (e.g. theater stage surface styles).
func ValidateMediaFx(spec *MediaFxSpec) error {
	return validateMediaFx(spec, "mediaFx")
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
	for _, field := range spec.normalizedAdvanced().fields() {
		if math.Abs(field.value-field.defaultValue) >= mediaFxFilterEpsilon {
			return true
		}
	}
	return false
}

// mediaFxEqual compares effect content, never pointer identity or version. A missing
// spec and an all-default spec both mean "no effect", and a v1 spec equals the v2 spec
// with the same motion / filter and an all-zero advanced.
func mediaFxEqual(left, right *MediaFxSpec) bool {
	leftActive, rightActive := mediaFxHasContent(left), mediaFxHasContent(right)
	if !leftActive || !rightActive {
		return leftActive == rightActive
	}
	return left.Motion == right.Motion && left.Filter == right.Filter &&
		left.normalizedAdvanced() == right.normalizedAdvanced()
}

func cloneMediaFx(spec *MediaFxSpec) *MediaFxSpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	if spec.Advanced != nil {
		advanced := *spec.Advanced
		clone.Advanced = &advanced
	}
	return &clone
}
