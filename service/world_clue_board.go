package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
)

const WorldClueBoardMaxDocumentBytes = 8 << 20
const WorldClueBoardRealtimeEventMaxBytes = 256 << 10

type WorldClueBoardOperationInput struct {
	Scope     string                           `json:"scope"`
	ClientID  string                           `json:"clientId"`
	Operation protocol.WorldClueBoardOperation `json:"operation"`
}

type WorldClueBoardOperationResult struct {
	protocol.WorldClueBoardWriteResult
	Changed         bool                             `json:"changed"`
	RemovedRelation *protocol.WorldClueBoardRelation `json:"-"`
}

type worldClueBoardQuickdrawDiff struct {
	Added   map[string]json.RawMessage   `json:"added"`
	Removed map[string]json.RawMessage   `json:"removed"`
	Updated map[string][]json.RawMessage `json:"updated"`
}

func applyWorldClueBoardQuickdrawDiff(document *protocol.WorldClueBoardDocument, raw json.RawMessage) error {
	var diff worldClueBoardQuickdrawDiff
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &diff) != nil {
		return ErrWorldClueBoardInvalid
	}
	if diff.Added == nil || diff.Removed == nil || diff.Updated == nil {
		return ErrWorldClueBoardInvalid
	}
	for id := range diff.Added {
		if _, exists := diff.Removed[id]; exists {
			return ErrWorldClueBoardInvalid
		}
		if _, exists := diff.Updated[id]; exists {
			return ErrWorldClueBoardInvalid
		}
	}
	for id := range diff.Removed {
		if _, exists := diff.Updated[id]; exists {
			return ErrWorldClueBoardInvalid
		}
	}
	for _, pair := range diff.Updated {
		if len(pair) != 2 {
			return ErrWorldClueBoardInvalid
		}
	}
	if document.Quickdraw == nil {
		document.Quickdraw = &protocol.WorldClueBoardQuickdraw{EngineVersion: protocol.WorldClueBoardQuickdrawV020, Snapshot: json.RawMessage(`{"document":{"store":{}}}`)}
	}
	var snapshot map[string]json.RawMessage
	var drawing map[string]json.RawMessage
	var records map[string]json.RawMessage
	if json.Unmarshal(document.Quickdraw.Snapshot, &snapshot) != nil || json.Unmarshal(snapshot["document"], &drawing) != nil || json.Unmarshal(drawing["store"], &records) != nil || records == nil {
		return ErrWorldClueBoardUnsupported
	}
	// Validate even removed/old records, so malformed diffs cannot be broadcast.
	validateRecord := func(id string, record json.RawMessage) error {
		store, err := json.Marshal(map[string]json.RawMessage{id: record})
		if err != nil {
			return ErrWorldClueBoardInvalid
		}
		return validateQuickdrawSnapshot(&protocol.WorldClueBoardQuickdraw{EngineVersion: protocol.WorldClueBoardQuickdrawV020, Snapshot: json.RawMessage(`{"document":{"store":` + string(store) + `}}`)})
	}
	for id, record := range diff.Removed {
		if err := validateRecord(id, record); err != nil {
			return err
		}
		delete(records, id)
	}
	for id, pair := range diff.Updated {
		for _, record := range pair {
			if err := validateRecord(id, record); err != nil {
				return err
			}
		}
		records[id] = pair[1]
	}
	for id, record := range diff.Added {
		if err := validateRecord(id, record); err != nil {
			return err
		}
		records[id] = record
	}
	drawing["store"], _ = json.Marshal(records)
	snapshot["document"], _ = json.Marshal(drawing)
	document.Quickdraw.Snapshot, _ = json.Marshal(snapshot)
	return nil
}

func applyWorldClueBoardOperation(document *protocol.WorldClueBoardDocument, operation protocol.WorldClueBoardOperation, visible map[string]bool) (*protocol.WorldClueBoardRelation, error) {
	if err := validateBoardText(operation.ID, "operation id", 160, true); err != nil {
		return nil, err
	}
	switch operation.Type {
	case protocol.WorldClueBoardPlacementsPut:
		if len(operation.Placements) == 0 || operation.Relation != nil || operation.RelationID != "" || len(operation.QuickdrawDiff) != 0 {
			return nil, ErrWorldClueBoardInvalid
		}
		for id, placement := range operation.Placements {
			if visible != nil && !visible[id] {
				return nil, ErrWorldClueBoardDenied
			}
			document.Placements[id] = placement
		}
	case protocol.WorldClueBoardRelationPut:
		if operation.Relation == nil || len(operation.Placements) != 0 || operation.RelationID != "" || len(operation.QuickdrawDiff) != 0 {
			return nil, ErrWorldClueBoardInvalid
		}
		if !worldClueBoardRelationVisible(*operation.Relation, visible) {
			return nil, ErrWorldClueBoardDenied
		}
		for index, relation := range document.Relations {
			if relation.ID == operation.Relation.ID {
				// Replacing a guessed hidden relation ID must not overwrite it either.
				if !worldClueBoardRelationVisible(relation, visible) {
					return nil, ErrWorldClueBoardDenied
				}
				document.Relations[index] = *operation.Relation
				return nil, nil
			}
		}
		document.Relations = append(document.Relations, *operation.Relation)
	case protocol.WorldClueBoardRelationRemove:
		if err := validateBoardText(operation.RelationID, "relation id", 160, true); err != nil {
			return nil, err
		}
		if operation.Relation != nil || len(operation.Placements) != 0 || len(operation.QuickdrawDiff) != 0 {
			return nil, ErrWorldClueBoardInvalid
		}
		for index, relation := range document.Relations {
			if relation.ID == operation.RelationID {
				if !worldClueBoardRelationVisible(relation, visible) {
					return nil, ErrWorldClueBoardDenied
				}
				document.Relations = append(document.Relations[:index], document.Relations[index+1:]...)
				return &relation, nil
			}
		}
	case protocol.WorldClueBoardQuickdrawDiff:
		if operation.Relation != nil || operation.RelationID != "" || len(operation.Placements) != 0 {
			return nil, ErrWorldClueBoardInvalid
		}
		return nil, applyWorldClueBoardQuickdrawDiff(document, operation.QuickdrawDiff)
	default:
		return nil, ErrWorldClueBoardInvalid
	}
	return nil, nil
}

func WorldClueBoardApplyOperation(worldID, boardKey, actorID string, input WorldClueBoardOperationInput) (*WorldClueBoardOperationResult, error) {
	if err := validateWorldClueBoardScope(boardKey); err != nil {
		return nil, err
	}
	if input.Scope != protocol.WorldClueBoardScopeShared {
		return nil, ErrWorldClueBoardInvalid
	}
	_, canWrite, err := worldClueBoardAccess(worldID, actorID, input.Scope, false)
	if err != nil {
		return nil, err
	}
	if !canWrite {
		return nil, ErrWorldClueBoardDenied
	}
	if err := validateBoardText(input.ClientID, "client id", 160, true); err != nil {
		return nil, err
	}
	needsClueVisibility := false
	switch input.Operation.Type {
	case protocol.WorldClueBoardPlacementsPut, protocol.WorldClueBoardRelationPut, protocol.WorldClueBoardRelationRemove:
		needsClueVisibility = true
	case protocol.WorldClueBoardQuickdrawDiff:
	default:
		return nil, ErrWorldClueBoardInvalid
	}
	db := model.GetDB()
	for attempt := 0; attempt < 5; attempt++ {
		var row model.WorldClueBoardModel
		if err := db.Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ?", worldID, boardKey, input.Scope, "").Limit(1).Find(&row).Error; err != nil {
			return nil, err
		}
		document := EmptyWorldClueBoardDocument()
		currentCanonical, _ := json.Marshal(document)
		if row.ID != "" {
			document, currentCanonical, err = validateWorldClueBoardDocument([]byte(row.DocumentJSON))
			if err != nil {
				return nil, err
			}
		}
		var visible map[string]bool
		if needsClueVisibility {
			visible, err = worldClueBoardVisibleClues(worldID, actorID)
			if err != nil {
				return nil, err
			}
		}
		removed, err := applyWorldClueBoardOperation(&document, input.Operation, visible)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(document)
		if err != nil {
			return nil, ErrWorldClueBoardInvalid
		}
		_, canonical, err := validateWorldClueBoardDocument(raw)
		if err != nil {
			return nil, err
		}
		result := &WorldClueBoardOperationResult{WorldClueBoardWriteResult: protocol.WorldClueBoardWriteResult{WorldID: worldID, BoardKey: boardKey, Scope: input.Scope, Revision: row.Revision}, RemovedRelation: removed}
		if !row.UpdatedAt.IsZero() {
			result.UpdatedAt = row.UpdatedAt.UnixMilli()
		}
		if bytes.Equal(canonical, currentCanonical) {
			return result, nil
		}
		if row.Revision == math.MaxInt64 {
			return nil, ErrWorldClueBoardConflict
		}
		_, canWrite, err = worldClueBoardAccess(worldID, actorID, input.Scope, false)
		if err != nil {
			return nil, err
		}
		if !canWrite {
			return nil, ErrWorldClueBoardDenied
		}
		now := time.Now()
		var written *gorm.DB
		if row.ID == "" {
			row = model.WorldClueBoardModel{WorldID: worldID, BoardKey: boardKey, Scope: input.Scope, OwnerUserID: "", Revision: 1, DocumentJSON: string(canonical), UpdatedBy: actorID}
			row.Init()
			row.CreatedAt, row.UpdatedAt = now, now
			written = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		} else {
			written = db.Model(&model.WorldClueBoardModel{}).Where("id = ? AND revision = ?", row.ID, row.Revision).Updates(map[string]any{"document_json": string(canonical), "revision": row.Revision + 1, "updated_by": actorID, "updated_at": now})
		}
		if written.Error != nil {
			if errors.Is(written.Error, gorm.ErrDuplicatedKey) {
				continue
			}
			return nil, written.Error
		}
		if written.RowsAffected != 1 {
			continue
		}
		result.Revision++
		result.UpdatedAt, result.Changed = now.UnixMilli(), true
		return result, nil
	}
	return nil, ErrWorldClueBoardConflict
}

// FilterWorldClueBoardOperationForActor returns only recipient-visible fields.
// Errors are fail-closed: the caller must not send the unfiltered operation.
func FilterWorldClueBoardOperationForActor(worldID, actorID string, operation *protocol.WorldClueBoardOperation, removed *protocol.WorldClueBoardRelation) (*protocol.WorldClueBoardOperation, error) {
	if _, _, err := worldClueBoardAccess(worldID, actorID, protocol.WorldClueBoardScopeShared, false); err != nil {
		return nil, err
	}
	if operation == nil {
		return nil, nil
	}
	filtered := *operation
	switch operation.Type {
	case protocol.WorldClueBoardPlacementsPut:
		visible, err := worldClueBoardVisibleClues(worldID, actorID)
		if err != nil {
			return nil, err
		}
		filtered.Placements = make(map[string]protocol.WorldClueBoardPlacement)
		for id, placement := range operation.Placements {
			if visible == nil || visible[id] {
				filtered.Placements[id] = placement
			}
		}
		if len(filtered.Placements) == 0 {
			return nil, nil
		}
	case protocol.WorldClueBoardRelationPut:
		visible, err := worldClueBoardVisibleClues(worldID, actorID)
		if err != nil {
			return nil, err
		}
		if operation.Relation == nil || !worldClueBoardRelationVisible(*operation.Relation, visible) {
			return nil, nil
		}
	case protocol.WorldClueBoardRelationRemove:
		visible, err := worldClueBoardVisibleClues(worldID, actorID)
		if err != nil {
			return nil, err
		}
		if removed == nil || !worldClueBoardRelationVisible(*removed, visible) {
			return nil, nil
		}
	case protocol.WorldClueBoardQuickdrawDiff:
	default:
		return nil, nil
	}
	raw, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	if len(raw) > WorldClueBoardRealtimeEventMaxBytes {
		return nil, nil
	}
	return &filtered, nil
}

var (
	ErrWorldClueBoardInvalid     = errors.New("world clue board invalid")
	ErrWorldClueBoardDenied      = errors.New("world clue board permission denied")
	ErrWorldClueBoardConflict    = errors.New("world clue board conflict")
	ErrWorldClueBoardTooLarge    = errors.New("world clue board payload too large")
	ErrWorldClueBoardUnsupported = errors.New("world clue board unsupported snapshot")
)

type WorldClueBoardPutInput struct {
	Scope string
	// A pointer is intentional: revision zero is a valid first-create value,
	// while an omitted field is a malformed request.
	ExpectedRevision *int64
	Document         json.RawMessage
}

func EmptyWorldClueBoardDocument() protocol.WorldClueBoardDocument {
	return protocol.WorldClueBoardDocument{
		Version:    protocol.WorldClueBoardVersion,
		Placements: map[string]protocol.WorldClueBoardPlacement{},
		Relations:  []protocol.WorldClueBoardRelation{},
		Quickdraw:  nil,
	}
}

func validateBoardText(value, field string, max int, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is required", ErrWorldClueBoardInvalid, field)
	}
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%w: %s is too long", ErrWorldClueBoardInvalid, field)
	}
	return nil
}

func validateQuickdrawSnapshot(value *protocol.WorldClueBoardQuickdraw) error {
	if value == nil {
		return nil
	}
	engineVersion := strings.TrimSpace(value.EngineVersion)
	if engineVersion != protocol.WorldClueBoardQuickdrawV020 && engineVersion != "0.2.0" {
		return fmt.Errorf("%w: unsupported quickdraw engine version", ErrWorldClueBoardUnsupported)
	}
	if len(value.Snapshot) == 0 || !json.Valid(value.Snapshot) {
		return fmt.Errorf("%w: quickdraw snapshot is not valid JSON", ErrWorldClueBoardUnsupported)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(value.Snapshot, &envelope); err != nil || envelope == nil {
		return fmt.Errorf("%w: quickdraw snapshot must be an object", ErrWorldClueBoardUnsupported)
	}
	documentRaw, ok := envelope["document"]
	if !ok {
		return fmt.Errorf("%w: quickdraw snapshot document is missing", ErrWorldClueBoardUnsupported)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(documentRaw, &document); err != nil || document == nil {
		return fmt.Errorf("%w: quickdraw snapshot document is invalid", ErrWorldClueBoardUnsupported)
	}
	storeRaw, ok := document["store"]
	if !ok {
		return fmt.Errorf("%w: quickdraw snapshot store is missing", ErrWorldClueBoardUnsupported)
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(storeRaw, &store); err != nil || store == nil {
		return fmt.Errorf("%w: quickdraw snapshot store is invalid", ErrWorldClueBoardUnsupported)
	}
	for id, raw := range store {
		if err := validateBoardText(id, "quickdraw record id", 200, true); err != nil {
			return err
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(raw, &record); err != nil || record == nil {
			return fmt.Errorf("%w: quickdraw record is invalid", ErrWorldClueBoardUnsupported)
		}
		var recordID string
		if idRaw, exists := record["id"]; !exists || json.Unmarshal(idRaw, &recordID) != nil || strings.TrimSpace(recordID) == "" || recordID != id {
			return fmt.Errorf("%w: quickdraw record id is invalid", ErrWorldClueBoardUnsupported)
		}
		typeRaw, exists := record["typeName"]
		if !exists {
			return fmt.Errorf("%w: quickdraw record type is missing", ErrWorldClueBoardUnsupported)
		}
		var typeName string
		if err := json.Unmarshal(typeRaw, &typeName); err != nil || (typeName != "shape" && typeName != "asset") {
			return fmt.Errorf("%w: quickdraw record type is unsupported", ErrWorldClueBoardUnsupported)
		}
		readNumber := func(field string, required bool) (float64, error) {
			rawValue, present := record[field]
			if !present {
				if required {
					return 0, fmt.Errorf("%w: quickdraw %s record field %s is missing", ErrWorldClueBoardUnsupported, typeName, field)
				}
				return 0, nil
			}
			var number float64
			if err := json.Unmarshal(rawValue, &number); err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
				return 0, fmt.Errorf("%w: quickdraw %s record field %s is invalid", ErrWorldClueBoardUnsupported, typeName, field)
			}
			return number, nil
		}
		if typeName == "shape" {
			var shapeType string
			if typeValue, ok := record["type"]; !ok || json.Unmarshal(typeValue, &shapeType) != nil {
				return fmt.Errorf("%w: quickdraw shape type is invalid", ErrWorldClueBoardUnsupported)
			}
			switch shapeType {
			case "draw", "highlight", "geo", "arrow", "line", "text", "note", "image":
			default:
				return fmt.Errorf("%w: quickdraw shape type is unsupported", ErrWorldClueBoardUnsupported)
			}
			for _, field := range []string{"x", "y", "rot", "z"} {
				if _, err := readNumber(field, true); err != nil {
					return err
				}
			}
			var props map[string]json.RawMessage
			if propsRaw, ok := record["props"]; !ok || json.Unmarshal(propsRaw, &props) != nil || props == nil {
				return fmt.Errorf("%w: quickdraw shape props are invalid", ErrWorldClueBoardUnsupported)
			}
		} else {
			var src string
			if srcRaw, ok := record["src"]; !ok || json.Unmarshal(srcRaw, &src) != nil || strings.TrimSpace(src) == "" {
				return fmt.Errorf("%w: quickdraw asset source is invalid", ErrWorldClueBoardUnsupported)
			}
			width, err := readNumber("w", true)
			if err != nil || width <= 0 {
				if err != nil {
					return err
				}
				return fmt.Errorf("%w: quickdraw asset width is invalid", ErrWorldClueBoardUnsupported)
			}
			height, err := readNumber("h", true)
			if err != nil || height <= 0 {
				if err != nil {
					return err
				}
				return fmt.Errorf("%w: quickdraw asset height is invalid", ErrWorldClueBoardUnsupported)
			}
		}
	}
	return nil
}

func validateWorldClueBoardDocument(raw []byte) (protocol.WorldClueBoardDocument, []byte, error) {
	if len(raw) == 0 {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: document is required", ErrWorldClueBoardInvalid)
	}
	if len(raw) > WorldClueBoardMaxDocumentBytes {
		return protocol.WorldClueBoardDocument{}, nil, ErrWorldClueBoardTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var top map[string]json.RawMessage
	if err := decoder.Decode(&top); err != nil || top == nil {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: document must be a JSON object", ErrWorldClueBoardInvalid)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: document contains trailing data", ErrWorldClueBoardInvalid)
	}
	versionRaw, ok := top["version"]
	if !ok {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: version is required", ErrWorldClueBoardInvalid)
	}
	var version int
	if err := json.Unmarshal(versionRaw, &version); err != nil || version != protocol.WorldClueBoardVersion {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: unsupported document version", ErrWorldClueBoardInvalid)
	}
	placementsRaw, ok := top["placements"]
	if !ok {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placements is required", ErrWorldClueBoardInvalid)
	}
	var placements map[string]protocol.WorldClueBoardPlacement
	if err := json.Unmarshal(placementsRaw, &placements); err != nil || placements == nil {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placements must be an object", ErrWorldClueBoardInvalid)
	}
	var placementObjects map[string]json.RawMessage
	if err := json.Unmarshal(placementsRaw, &placementObjects); err != nil || placementObjects == nil {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placements must be an object", ErrWorldClueBoardInvalid)
	}
	if len(placements) > 100000 {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: too many placements", ErrWorldClueBoardInvalid)
	}
	for clueID, placement := range placements {
		if err := validateBoardText(clueID, "placement clue id", 200, true); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
		// encoding/json intentionally maps missing/null numeric fields to zero.
		// Inspect each placement object first so malformed coordinates cannot be
		// silently persisted as a valid origin placement.
		placementRaw, exists := placementObjects[clueID]
		if !exists {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement %s must be an object", ErrWorldClueBoardInvalid, clueID)
		}
		var placementFields map[string]json.RawMessage
		if err := json.Unmarshal(placementRaw, &placementFields); err != nil || placementFields == nil {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement %s must be an object", ErrWorldClueBoardInvalid, clueID)
		}
		for _, field := range []string{"x", "y"} {
			value, ok := placementFields[field]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement %s.%s is required", ErrWorldClueBoardInvalid, clueID, field)
			}
			var number float64
			if err := json.Unmarshal(value, &number); err != nil {
				return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement %s.%s must be a number", ErrWorldClueBoardInvalid, clueID, field)
			}
		}
		for _, field := range []string{"width", "pinned"} {
			value, ok := placementFields[field]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				continue
			}
			if field == "width" {
				var number float64
				if err := json.Unmarshal(value, &number); err != nil {
					return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement %s.width must be a number", ErrWorldClueBoardInvalid, clueID)
				}
			} else {
				var pinned bool
				if err := json.Unmarshal(value, &pinned); err != nil {
					return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement %s.pinned must be a boolean", ErrWorldClueBoardInvalid, clueID)
				}
			}
		}
		if !math.IsNaN(placement.X) && (math.IsInf(placement.X, 0) || math.Abs(placement.X) > 1e8) {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement x is out of range", ErrWorldClueBoardInvalid)
		}
		if !math.IsNaN(placement.Y) && (math.IsInf(placement.Y, 0) || math.Abs(placement.Y) > 1e8) {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement y is out of range", ErrWorldClueBoardInvalid)
		}
		if math.IsNaN(placement.X) || math.IsNaN(placement.Y) {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement coordinates are invalid", ErrWorldClueBoardInvalid)
		}
		if placement.Width != nil && (math.IsNaN(*placement.Width) || math.IsInf(*placement.Width, 0) || *placement.Width <= 0 || *placement.Width > 1e6) {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: placement width is invalid", ErrWorldClueBoardInvalid)
		}
	}
	relationsRaw, ok := top["relations"]
	if !ok {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: relations is required", ErrWorldClueBoardInvalid)
	}
	var relations []protocol.WorldClueBoardRelation
	if err := json.Unmarshal(relationsRaw, &relations); err != nil || relations == nil {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: relations must be an array", ErrWorldClueBoardInvalid)
	}
	if len(relations) > 100000 {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: too many relations", ErrWorldClueBoardInvalid)
	}
	seenRelationIDs := make(map[string]struct{}, len(relations))
	for _, relation := range relations {
		if err := validateBoardText(relation.ID, "relation id", 160, true); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
		if _, exists := seenRelationIDs[relation.ID]; exists {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: relation ids must be unique", ErrWorldClueBoardInvalid)
		}
		seenRelationIDs[relation.ID] = struct{}{}
		if err := validateBoardText(string(relation.SourceRef.Kind), "source endpoint kind", 32, true); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
		if err := validateBoardText(relation.SourceRef.ID, "source endpoint id", 200, true); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
		if err := validateBoardText(string(relation.TargetRef.Kind), "target endpoint kind", 32, true); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
		if err := validateBoardText(relation.TargetRef.ID, "target endpoint id", 200, true); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
		if relation.SourceRef.Kind != protocol.WorldClueBoardRelationEndpointClue && relation.SourceRef.Kind != protocol.WorldClueBoardRelationEndpointQuickdraw {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: invalid source endpoint kind", ErrWorldClueBoardInvalid)
		}
		if relation.TargetRef.Kind != protocol.WorldClueBoardRelationEndpointClue && relation.TargetRef.Kind != protocol.WorldClueBoardRelationEndpointQuickdraw {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: invalid target endpoint kind", ErrWorldClueBoardInvalid)
		}
		if relation.SourceRef.Kind == relation.TargetRef.Kind && relation.SourceRef.ID == relation.TargetRef.ID {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: relation cannot self-connect", ErrWorldClueBoardInvalid)
		}
		switch relation.Kind {
		case protocol.WorldClueBoardRelationRelated, protocol.WorldClueBoardRelationReferences,
			protocol.WorldClueBoardRelationSupports, protocol.WorldClueBoardRelationContradicts,
			protocol.WorldClueBoardRelationCauses:
		default:
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: invalid relation kind", ErrWorldClueBoardInvalid)
		}
		if err := validateBoardText(relation.Label, "relation label", 500, false); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
	}
	quickdrawRaw, ok := top["quickdraw"]
	if !ok {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: quickdraw is required", ErrWorldClueBoardInvalid)
	}
	var quickdraw *protocol.WorldClueBoardQuickdraw
	if string(bytes.TrimSpace(quickdrawRaw)) != "null" {
		var value protocol.WorldClueBoardQuickdraw
		if err := json.Unmarshal(quickdrawRaw, &value); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: quickdraw must be an object or null", ErrWorldClueBoardInvalid)
		}
		quickdraw = &value
		if err := validateQuickdrawSnapshot(quickdraw); err != nil {
			return protocol.WorldClueBoardDocument{}, nil, err
		}
	}
	var document protocol.WorldClueBoardDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: document shape is invalid", ErrWorldClueBoardInvalid)
	}
	document.Version = protocol.WorldClueBoardVersion
	document.Placements = placements
	document.Relations = relations
	document.Quickdraw = quickdraw
	canonical, err := json.Marshal(document)
	if err != nil {
		return protocol.WorldClueBoardDocument{}, nil, fmt.Errorf("%w: cannot encode document", ErrWorldClueBoardInvalid)
	}
	if len(canonical) > WorldClueBoardMaxDocumentBytes {
		return protocol.WorldClueBoardDocument{}, nil, ErrWorldClueBoardTooLarge
	}
	return document, canonical, nil
}

func ensureWorldClueBoardMember(tx *gorm.DB, worldID, actorID string) error {
	var world model.WorldModel
	if err := tx.Where("id = ? AND status = ?", worldID, "active").Limit(1).Find(&world).Error; err != nil {
		return err
	}
	if world.ID == "" {
		return ErrWorldNotFound
	}
	var member model.WorldMemberModel
	if err := tx.Where("world_id = ? AND user_id = ?", worldID, actorID).Limit(1).Find(&member).Error; err != nil {
		return err
	}
	if member.ID == "" {
		return ErrWorldArchiveForbidden
	}
	return nil
}

func validateWorldClueBoardScope(boardKey string) error {
	// Keep the route key canonical.  Accepting a whitespace-padded value here
	// while persisting the untrimmed string would create a second database scope
	// that is not addressable by the fixed `boardKey=main` contract.
	if boardKey != protocol.WorldClueBoardKeyMain {
		return fmt.Errorf("%w: only boardKey=main is supported", ErrWorldClueBoardInvalid)
	}
	return nil
}

func boardResponse(row *model.WorldClueBoardModel, document protocol.WorldClueBoardDocument) *protocol.WorldClueBoardResponse {
	updatedAt := int64(0)
	if row != nil && !row.UpdatedAt.IsZero() {
		updatedAt = row.UpdatedAt.UnixMilli()
	}
	return &protocol.WorldClueBoardResponse{
		WorldID: row.WorldID, BoardKey: row.BoardKey, Scope: row.Scope, OwnerUserID: row.OwnerUserID,
		Revision: row.Revision, Document: document, UpdatedAt: updatedAt,
	}
}

func worldClueBoardAccess(worldID, actorID, scope string, wholeWrite bool) (string, bool, error) {
	if scope == "" {
		scope = protocol.WorldClueBoardScopePersonal
	}
	if scope != protocol.WorldClueBoardScopePersonal && scope != protocol.WorldClueBoardScopeShared {
		return "", false, ErrWorldClueBoardInvalid
	}
	if err := ensureWorldClueBoardMember(model.GetDB(), worldID, actorID); err != nil {
		return "", false, err
	}
	if scope == protocol.WorldClueBoardScopePersonal {
		return actorID, true, nil
	}
	role, err := worldClueRole(model.GetDB(), worldID, actorID)
	if err != nil {
		return "", false, err
	}
	if wholeWrite && !worldClueIsAdminRole(role) {
		return "", false, ErrWorldClueBoardDenied
	}
	return "", worldClueIsAdminRole(role) || role == model.WorldRoleMember, nil
}

func worldClueBoardVisibleClues(worldID, actorID string) (map[string]bool, error) {
	db := model.GetDB()
	role, err := worldClueRole(db, worldID, actorID)
	if err != nil {
		return nil, err
	}
	if worldClueIsAdminRole(role) {
		return nil, nil
	}
	if role == "" {
		return nil, ErrWorldClueDenied
	}
	var clues []model.WorldClueModel
	if err := db.Select("id", "status", "default_access").Where("world_id = ? AND status <> ?", worldID, model.WorldClueStatusArchived).Find(&clues).Error; err != nil {
		return nil, err
	}
	var accessRows []model.WorldClueAccessModel
	if err := db.Select("clue_id", "access_override").Where("world_id = ? AND user_id = ?", worldID, actorID).Find(&accessRows).Error; err != nil {
		return nil, err
	}
	accessByClue := make(map[string]string, len(accessRows))
	for _, row := range accessRows {
		accessByClue[row.ClueID] = row.AccessOverride
	}
	visible := make(map[string]bool, len(clues))
	for i := range clues {
		override := model.WorldClueAccessInherit
		if access, exists := accessByClue[clues[i].ID]; exists {
			override = access
		}
		access := effectiveWorldClueAccess(role, clues[i].DefaultAccess, override)
		if canViewWorldClue(&clues[i], role, access) {
			visible[clues[i].ID] = true
		}
	}
	return visible, nil
}

func worldClueBoardRelationVisible(relation protocol.WorldClueBoardRelation, visible map[string]bool) bool {
	if visible == nil {
		return true
	}
	for _, endpoint := range []protocol.WorldClueBoardRelationEndpointRef{relation.SourceRef, relation.TargetRef} {
		if endpoint.Kind == protocol.WorldClueBoardRelationEndpointClue && !visible[endpoint.ID] {
			return false
		}
	}
	return true
}

func filterWorldClueBoardDocumentForActor(worldID, actorID string, document protocol.WorldClueBoardDocument) (protocol.WorldClueBoardDocument, error) {
	visible, err := worldClueBoardVisibleClues(worldID, actorID)
	if err != nil {
		return document, err
	}
	if visible == nil {
		return document, nil
	}
	placements := make(map[string]protocol.WorldClueBoardPlacement)
	for id, placement := range document.Placements {
		if visible[id] {
			placements[id] = placement
		}
	}
	relations := make([]protocol.WorldClueBoardRelation, 0)
	for _, relation := range document.Relations {
		if worldClueBoardRelationVisible(relation, visible) {
			relations = append(relations, relation)
		}
	}
	document.Placements, document.Relations = placements, relations
	return document, nil
}

func WorldClueBoardGet(worldID, boardKey, actorID string, scopes ...string) (*protocol.WorldClueBoardResponse, error) {
	if err := validateWorldClueBoardScope(boardKey); err != nil {
		return nil, err
	}
	scope := protocol.WorldClueBoardScopePersonal
	if len(scopes) > 0 && scopes[0] != "" {
		scope = scopes[0]
	}
	ownerID, canWrite, err := worldClueBoardAccess(worldID, actorID, scope, false)
	if err != nil {
		return nil, err
	}
	db := model.GetDB()
	var row model.WorldClueBoardModel
	if err := db.Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ?", worldID, boardKey, scope, ownerID).Limit(1).Find(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		document := EmptyWorldClueBoardDocument()
		return &protocol.WorldClueBoardResponse{WorldID: worldID, BoardKey: boardKey, Scope: scope, OwnerUserID: ownerID, Revision: 0, Document: document, CanWrite: canWrite}, nil
	}
	document, _, err := validateWorldClueBoardDocument([]byte(row.DocumentJSON))
	if err != nil {
		// A corrupt/unsupported persisted snapshot is an error. Never turn it
		// into an empty document that could be overwritten by a later PUT.
		return nil, err
	}
	if scope == protocol.WorldClueBoardScopeShared {
		document, err = filterWorldClueBoardDocumentForActor(worldID, actorID, document)
		if err != nil {
			return nil, err
		}
	}
	response := boardResponse(&row, document)
	response.CanWrite = canWrite
	return response, nil
}

func WorldClueBoardPut(worldID, boardKey, actorID string, input WorldClueBoardPutInput) (*protocol.WorldClueBoardWriteResult, error) {
	if err := validateWorldClueBoardScope(boardKey); err != nil {
		return nil, err
	}
	// Authorize before parsing an attacker-controlled document so anonymous or
	// non-member callers cannot use validation/size errors as a write oracle.
	scope := input.Scope
	if scope == "" {
		scope = protocol.WorldClueBoardScopePersonal
	}
	ownerID, _, err := worldClueBoardAccess(worldID, actorID, scope, true)
	if err != nil {
		return nil, err
	}
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 0 || *input.ExpectedRevision == int64(^uint64(0)>>1) {
		return nil, fmt.Errorf("%w: expectedRevision is required", ErrWorldClueBoardInvalid)
	}
	_, canonical, err := validateWorldClueBoardDocument(input.Document)
	if err != nil {
		return nil, err
	}
	db := model.GetDB()
	var result protocol.WorldClueBoardWriteResult
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := ensureWorldClueBoardMember(tx, worldID, actorID); err != nil {
			return err
		}
		if scope == protocol.WorldClueBoardScopeShared {
			role, err := worldClueRole(tx, worldID, actorID)
			if err != nil {
				return err
			}
			if !worldClueIsAdminRole(role) {
				return ErrWorldClueBoardDenied
			}
		}
		// Never let a PUT turn an unreadable persisted document into a clean
		// replacement. A scoped read is used only to validate the existing
		// snapshot; the actual write below still relies on the revision predicate
		// and RowsAffected for optimistic concurrency.
		var current model.WorldClueBoardModel
		if err := tx.Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ?", worldID, boardKey, scope, ownerID).Limit(1).Find(&current).Error; err != nil {
			return err
		}
		if current.ID != "" {
			if _, _, err := validateWorldClueBoardDocument([]byte(current.DocumentJSON)); err != nil {
				return err
			}
		}
		expected := *input.ExpectedRevision
		if expected == 0 {
			now := time.Now()
			row := &model.WorldClueBoardModel{
				WorldID: worldID, BoardKey: boardKey, Scope: scope,
				OwnerUserID: ownerID, Revision: 1, DocumentJSON: string(canonical), UpdatedBy: actorID,
			}
			row.Init()
			row.CreatedAt = now
			row.UpdatedAt = now
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
			if created.Error != nil {
				if errors.Is(created.Error, gorm.ErrDuplicatedKey) {
					return ErrWorldClueBoardConflict
				}
				return created.Error
			}
			if created.RowsAffected != 1 {
				return ErrWorldClueBoardConflict
			}
			result = protocol.WorldClueBoardWriteResult{WorldID: worldID, BoardKey: boardKey, Scope: scope, Revision: 1, UpdatedAt: now.UnixMilli()}
			return nil
		}
		now := time.Now()
		updated := tx.Model(&model.WorldClueBoardModel{}).
			Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ? AND revision = ?", worldID, boardKey, scope, ownerID, expected).
			Updates(map[string]any{"document_json": string(canonical), "revision": expected + 1, "updated_by": actorID, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrWorldClueBoardConflict
		}
		result = protocol.WorldClueBoardWriteResult{WorldID: worldID, BoardKey: boardKey, Scope: scope, Revision: expected + 1, UpdatedAt: now.UnixMilli()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CleanupWorldClueBoardForMember is called from the existing WorldLeave
// transaction. It intentionally only deletes the leaving user's personal
// board; shared clue data and other members' boards are untouched.
func CleanupWorldClueBoardForMember(tx *gorm.DB, worldID, userID string) error {
	if tx == nil {
		return errors.New("nil transaction")
	}
	return tx.Unscoped().Where("world_id = ? AND scope = ? AND owner_user_id = ?", worldID, protocol.WorldClueBoardScopePersonal, userID).Delete(&model.WorldClueBoardModel{}).Error
}

// CleanupWorldClueBoardsForWorld is called from WorldDelete's transaction.
func CleanupWorldClueBoardsForWorld(tx *gorm.DB, worldID string) error {
	if tx == nil {
		return errors.New("nil transaction")
	}
	return tx.Unscoped().Where("world_id = ?", worldID).Delete(&model.WorldClueBoardModel{}).Error
}
