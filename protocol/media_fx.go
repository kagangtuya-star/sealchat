package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// MediaFxSpec mirrors the frontend Media FX spec (ui/src/features/media-fx/media-fx.ts).
// It is persisted inside theater presentation layers and styles, so it only carries
// whitelisted presets and bounded numbers; renderers never receive raw CSS or shaders.
// Keep the ranges below in sync with media-fx.ts and media-fx-schema.ts.
//
// v2 adds the required `advanced` object with pixelate / rgbSplit / scanline; v3 adds
// six more static effects to it; v4 adds the required `temporal` object (time-driven
// pixel effects). Stored v1 / v2 / v3 specs stay valid as they are and are upgraded
// lazily by clients on their next save; there is no data migration.
const (
	MediaFxVersion       = 4
	MediaFxV3Version     = 3
	MediaFxV2Version     = 2
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
//
// The v3-only effects are pointers so the strict decoder records their presence: a v2
// spec must not carry them and a v3 spec must carry all of them, while the v2 fields
// keep their plain shape. Compare through fields() / mediaFxAdvancedEqual, never the
// struct itself; a missing v3 effect always reads as 0 (off).
type MediaFxAdvanced struct {
	Pixelate float64 `json:"pixelate"`
	RGBSplit float64 `json:"rgbSplit"`
	Scanline float64 `json:"scanline"`

	Vignette  *float64 `json:"vignette,omitempty"`
	Grain     *float64 `json:"grain,omitempty"`
	Posterize *float64 `json:"posterize,omitempty"`
	Negative  *float64 `json:"negative,omitempty"`
	Sharpen   *float64 `json:"sharpen,omitempty"`
	Edge      *float64 `json:"edge,omitempty"`

	v3NullPresence uint8
	unknownField   string
}

const (
	mediaFxAdvancedVignettePresent uint8 = 1 << iota
	mediaFxAdvancedGrainPresent
	mediaFxAdvancedPosterizePresent
	mediaFxAdvancedNegativePresent
	mediaFxAdvancedSharpenPresent
	mediaFxAdvancedEdgePresent
)

// UnmarshalJSON only remembers v3-only fields that were explicitly null. A non-null
// field already has a non-nil *float64, while an omitted field stays nil; this tiny bit
// of transient state closes the version-2 `grain:null` loophole without changing the
// shape or equality of valid decoded specs. Unknown fields are recorded for the shared
// validator instead of making ordinary json.Unmarshal stricter than before.
func (advanced *MediaFxAdvanced) UnmarshalJSON(data []byte) error {
	type mediaFxAdvancedAlias MediaFxAdvanced
	var decoded mediaFxAdvancedAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*advanced = MediaFxAdvanced(decoded)
	for key, raw := range fields {
		null := bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		switch key {
		case "pixelate", "rgbSplit", "scanline":
		case "vignette":
			if null {
				advanced.v3NullPresence |= mediaFxAdvancedVignettePresent
			}
		case "grain":
			if null {
				advanced.v3NullPresence |= mediaFxAdvancedGrainPresent
			}
		case "posterize":
			if null {
				advanced.v3NullPresence |= mediaFxAdvancedPosterizePresent
			}
		case "negative":
			if null {
				advanced.v3NullPresence |= mediaFxAdvancedNegativePresent
			}
		case "sharpen":
			if null {
				advanced.v3NullPresence |= mediaFxAdvancedSharpenPresent
			}
		case "edge":
			if null {
				advanced.v3NullPresence |= mediaFxAdvancedEdgePresent
			}
		default:
			if advanced.unknownField == "" {
				advanced.unknownField = key
			}
		}
	}
	return nil
}

// MediaFxTemporal holds the v4 time-driven effects: four normalized 0..1 strengths
// (0 = off) and a speed multiplier that means nothing while every effect is off.
// Every field is a pointer so the validator can require all of them in v4; unknown
// fields are recorded for the shared validator like MediaFxAdvanced does.
type MediaFxTemporal struct {
	Grain        *float64 `json:"grain"`
	Flicker      *float64 `json:"flicker"`
	Glitch       *float64 `json:"glitch"`
	ScanlineRoll *float64 `json:"scanlineRoll"`
	Speed        *float64 `json:"speed"`

	unknownField string
}

func (temporal *MediaFxTemporal) UnmarshalJSON(data []byte) error {
	type mediaFxTemporalAlias MediaFxTemporal
	var decoded mediaFxTemporalAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*temporal = MediaFxTemporal(decoded)
	for key := range fields {
		switch key {
		case "grain", "flicker", "glitch", "scanlineRoll", "speed":
		default:
			if temporal.unknownField == "" {
				temporal.unknownField = key
			}
		}
	}
	return nil
}

const (
	minMediaFxTemporalSpeed     = 0.25
	maxMediaFxTemporalSpeed     = 3.0
	defaultMediaFxTemporalSpeed = 1.0
)

type mediaFxTemporalField struct {
	name             string
	value            *float64
	minimum, maximum float64
	defaultValue     float64
	effect           bool
}

func (temporal MediaFxTemporal) fields() []mediaFxTemporalField {
	return []mediaFxTemporalField{
		{"grain", temporal.Grain, 0, 1, 0, true},
		{"flicker", temporal.Flicker, 0, 1, 0, true},
		{"glitch", temporal.Glitch, 0, 1, 0, true},
		{"scanlineRoll", temporal.ScanlineRoll, 0, 1, 0, true},
		{"speed", temporal.Speed, minMediaFxTemporalSpeed, maxMediaFxTemporalSpeed, defaultMediaFxTemporalSpeed, false},
	}
}

// current reads a field with a missing value as its default (off / 1x).
func (field mediaFxTemporalField) current() float64 {
	if field.value == nil {
		return field.defaultValue
	}
	return *field.value
}

// mediaFxTemporalHasContent only looks at the four effects; speed alone is no effect.
func mediaFxTemporalHasContent(temporal *MediaFxTemporal) bool {
	if temporal == nil {
		return false
	}
	for _, field := range temporal.fields() {
		if field.effect && math.Abs(field.current()-field.defaultValue) >= mediaFxFilterEpsilon {
			return true
		}
	}
	return false
}

// mediaFxTemporalEqual treats every temporal without an effect as equal, whatever its
// speed, so a v1 / v2 / v3 spec equals the v4 spec with temporal off.
func mediaFxTemporalEqual(left, right *MediaFxTemporal) bool {
	leftActive, rightActive := mediaFxTemporalHasContent(left), mediaFxTemporalHasContent(right)
	if !leftActive || !rightActive {
		return leftActive == rightActive
	}
	leftFields, rightFields := left.fields(), right.fields()
	for index := range leftFields {
		if leftFields[index].current() != rightFields[index].current() {
			return false
		}
	}
	return true
}

// Advanced is a pointer on purpose: v1 specs must not carry it, while v2 / v3 / v4 specs
// must, so the validator can tell a v1 document from an incomplete v2 one. Temporal
// follows the same rule: only v4 carries it, and v4 must. `temporalNullPresence` remembers
// an explicit top-level temporal:null just long enough for old-version strict validation;
// valid decoded specs keep no transient presence state.
type MediaFxSpec struct {
	Version  int              `json:"version"`
	Motion   MediaFxMotion    `json:"motion"`
	Filter   MediaFxFilter    `json:"filter"`
	Advanced *MediaFxAdvanced `json:"advanced,omitempty"`
	Temporal *MediaFxTemporal `json:"temporal,omitempty"`

	temporalNullPresence bool
	unknownField         string
	unknownFieldPath     string
}

// UnmarshalJSON preserves the distinction between an omitted temporal field and an
// explicit null without changing the public spec shape. Unknown fields are recorded for
// the shared validator, matching MediaFxAdvanced / MediaFxTemporal strict validation.
func (spec *MediaFxSpec) UnmarshalJSON(data []byte) error {
	type mediaFxSpecAlias MediaFxSpec
	var decoded mediaFxSpecAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*spec = MediaFxSpec(decoded)
	for key, raw := range fields {
		switch key {
		case "version", "advanced":
		case "motion":
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(raw, &nested); err == nil {
				for nestedKey := range nested {
					switch nestedKey {
					case "preset", "intensity", "durationMs", "loop":
					default:
						if spec.unknownField == "" {
							spec.unknownField = nestedKey
							spec.unknownFieldPath = ".motion"
						}
					}
				}
			}
		case "filter":
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(raw, &nested); err == nil {
				for nestedKey := range nested {
					switch nestedKey {
					case "brightness", "contrast", "saturation", "grayscale", "sepia", "hueRotate", "blurPx":
					default:
						if spec.unknownField == "" {
							spec.unknownField = nestedKey
							spec.unknownFieldPath = ".filter"
						}
					}
				}
			}
		case "temporal":
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				spec.temporalNullPresence = true
			}
		default:
			if spec.unknownField == "" {
				spec.unknownField = key
			}
		}
	}
	return nil
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

// mediaFxAdvancedV3Field describes one v3-only effect; value is nil when absent.
type mediaFxAdvancedV3Field struct {
	name    string
	value   *float64
	present bool
}

func (advanced MediaFxAdvanced) v3Fields() []mediaFxAdvancedV3Field {
	return []mediaFxAdvancedV3Field{
		{"vignette", advanced.Vignette, advanced.Vignette != nil || advanced.v3NullPresence&mediaFxAdvancedVignettePresent != 0},
		{"grain", advanced.Grain, advanced.Grain != nil || advanced.v3NullPresence&mediaFxAdvancedGrainPresent != 0},
		{"posterize", advanced.Posterize, advanced.Posterize != nil || advanced.v3NullPresence&mediaFxAdvancedPosterizePresent != 0},
		{"negative", advanced.Negative, advanced.Negative != nil || advanced.v3NullPresence&mediaFxAdvancedNegativePresent != 0},
		{"sharpen", advanced.Sharpen, advanced.Sharpen != nil || advanced.v3NullPresence&mediaFxAdvancedSharpenPresent != 0},
		{"edge", advanced.Edge, advanced.Edge != nil || advanced.v3NullPresence&mediaFxAdvancedEdgePresent != 0},
	}
}

// fields lists every advanced effect with a missing v3 effect read as 0 (off).
func (advanced MediaFxAdvanced) fields() []mediaFxFilterField {
	fields := []mediaFxFilterField{
		{"pixelate", advanced.Pixelate, 0, 1, 0},
		{"rgbSplit", advanced.RGBSplit, 0, 1, 0},
		{"scanline", advanced.Scanline, 0, 1, 0},
	}
	for _, field := range advanced.v3Fields() {
		value := 0.0
		if field.value != nil {
			value = *field.value
		}
		fields = append(fields, mediaFxFilterField{field.name, value, 0, 1, 0})
	}
	return fields
}

// normalizedAdvanced treats a missing (v1) advanced as all zero.
func (spec *MediaFxSpec) normalizedAdvanced() MediaFxAdvanced {
	if spec == nil || spec.Advanced == nil {
		return MediaFxAdvanced{}
	}
	return *spec.Advanced
}

// mediaFxAdvancedEqual compares effect values, so an absent v3 effect equals 0.
func mediaFxAdvancedEqual(left, right MediaFxAdvanced) bool {
	leftFields, rightFields := left.fields(), right.fields()
	for index := range leftFields {
		if leftFields[index].value != rightFields[index].value {
			return false
		}
	}
	return true
}

func cloneMediaFxFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
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
// (without advanced), a complete v2 spec (advanced without v3-only effects), a
// complete v3 spec (advanced with every effect present) or a complete v4 spec (a v3
// advanced plus a temporal with every field present). Only v4 may carry temporal.
func validateMediaFx(spec *MediaFxSpec, path string) error {
	if spec == nil {
		return nil
	}
	var problems []error
	if spec.unknownField != "" {
		problems = append(problems, fmt.Errorf("%s%s: json: unknown field %q", path, spec.unknownFieldPath, spec.unknownField))
	}
	if spec.Version != MediaFxVersion && (spec.Temporal != nil || spec.temporalNullPresence) {
		problems = append(problems, fmt.Errorf("%s.temporal is not allowed in version %d", path, spec.Version))
	}
	switch spec.Version {
	case LegacyMediaFxVersion:
		if spec.Advanced != nil {
			problems = append(problems, fmt.Errorf("%s.advanced is not allowed in version %d", path, LegacyMediaFxVersion))
		}
	case MediaFxV2Version, MediaFxV3Version, MediaFxVersion:
		if spec.Advanced == nil {
			problems = append(problems, fmt.Errorf("%s.advanced is required in version %d", path, spec.Version))
			break
		}
		if spec.Advanced.unknownField != "" {
			problems = append(problems, fmt.Errorf("%s.advanced: json: unknown field %q", path, spec.Advanced.unknownField))
		}
		for _, field := range spec.Advanced.v3Fields() {
			if spec.Version == MediaFxV2Version && field.present {
				problems = append(problems, fmt.Errorf("%s.advanced.%s is not allowed in version %d", path, field.name, MediaFxV2Version))
			}
			if spec.Version != MediaFxV2Version && (!field.present || field.value == nil) {
				problems = append(problems, fmt.Errorf("%s.advanced.%s is required in version %d", path, field.name, spec.Version))
			}
		}
		for _, field := range spec.Advanced.fields() {
			if !finiteInRange(field.value, field.minimum, field.maximum) {
				problems = append(problems, fmt.Errorf("%s.advanced.%s must be finite and between %g and %g", path, field.name, field.minimum, field.maximum))
			}
		}
	default:
		problems = append(problems, fmt.Errorf("%s.version must be %d, %d, %d or %d", path, LegacyMediaFxVersion, MediaFxV2Version, MediaFxV3Version, MediaFxVersion))
	}
	if spec.Version == MediaFxVersion {
		problems = append(problems, validateMediaFxTemporal(spec.Temporal, path+".temporal")...)
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

func validateMediaFxTemporal(temporal *MediaFxTemporal, path string) []error {
	if temporal == nil {
		return []error{fmt.Errorf("%s is required in version %d", path, MediaFxVersion)}
	}
	var problems []error
	if temporal.unknownField != "" {
		problems = append(problems, fmt.Errorf("%s: json: unknown field %q", path, temporal.unknownField))
	}
	for _, field := range temporal.fields() {
		if field.value == nil {
			problems = append(problems, fmt.Errorf("%s.%s is required in version %d", path, field.name, MediaFxVersion))
			continue
		}
		if !finiteInRange(*field.value, field.minimum, field.maximum) {
			problems = append(problems, fmt.Errorf("%s.%s must be finite and between %g and %g", path, field.name, field.minimum, field.maximum))
		}
	}
	return problems
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
	return mediaFxTemporalHasContent(spec.Temporal)
}

// mediaFxEqual compares effect content, never pointer identity or version. A missing
// spec and an all-default spec both mean "no effect"; a v1 / v2 / v3 spec equals the v4
// spec with the same motion / filter / advanced values, the effects it lacks at 0 and
// temporal off (any speed).
func mediaFxEqual(left, right *MediaFxSpec) bool {
	leftActive, rightActive := mediaFxHasContent(left), mediaFxHasContent(right)
	if !leftActive || !rightActive {
		return leftActive == rightActive
	}
	return left.Motion == right.Motion && left.Filter == right.Filter &&
		mediaFxAdvancedEqual(left.normalizedAdvanced(), right.normalizedAdvanced()) &&
		mediaFxTemporalEqual(left.Temporal, right.Temporal)
}

func cloneMediaFx(spec *MediaFxSpec) *MediaFxSpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	if spec.Advanced != nil {
		advanced := *spec.Advanced
		advanced.Vignette = cloneMediaFxFloat(advanced.Vignette)
		advanced.Grain = cloneMediaFxFloat(advanced.Grain)
		advanced.Posterize = cloneMediaFxFloat(advanced.Posterize)
		advanced.Negative = cloneMediaFxFloat(advanced.Negative)
		advanced.Sharpen = cloneMediaFxFloat(advanced.Sharpen)
		advanced.Edge = cloneMediaFxFloat(advanced.Edge)
		clone.Advanced = &advanced
	}
	if spec.Temporal != nil {
		temporal := *spec.Temporal
		temporal.Grain = cloneMediaFxFloat(temporal.Grain)
		temporal.Flicker = cloneMediaFxFloat(temporal.Flicker)
		temporal.Glitch = cloneMediaFxFloat(temporal.Glitch)
		temporal.ScanlineRoll = cloneMediaFxFloat(temporal.ScanlineRoll)
		temporal.Speed = cloneMediaFxFloat(temporal.Speed)
		clone.Temporal = &temporal
	}
	return &clone
}
