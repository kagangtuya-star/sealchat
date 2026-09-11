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

var (
	ErrWorldClueBoardInvalid     = errors.New("world clue board invalid")
	ErrWorldClueBoardDenied      = errors.New("world clue board permission denied")
	ErrWorldClueBoardConflict    = errors.New("world clue board conflict")
	ErrWorldClueBoardTooLarge    = errors.New("world clue board payload too large")
	ErrWorldClueBoardUnsupported = errors.New("world clue board unsupported snapshot")
)

type WorldClueBoardPutInput struct {
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

func WorldClueBoardGet(worldID, boardKey, actorID string) (*protocol.WorldClueBoardResponse, error) {
	if err := validateWorldClueBoardScope(boardKey); err != nil {
		return nil, err
	}
	db := model.GetDB()
	if err := ensureWorldClueBoardMember(db, worldID, actorID); err != nil {
		return nil, err
	}
	var row model.WorldClueBoardModel
	if err := db.Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ?", worldID, boardKey, protocol.WorldClueBoardScopePersonal, actorID).Limit(1).Find(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		document := EmptyWorldClueBoardDocument()
		return &protocol.WorldClueBoardResponse{WorldID: worldID, BoardKey: boardKey, Scope: protocol.WorldClueBoardScopePersonal, OwnerUserID: actorID, Revision: 0, Document: document}, nil
	}
	document, _, err := validateWorldClueBoardDocument([]byte(row.DocumentJSON))
	if err != nil {
		// A corrupt/unsupported persisted snapshot is an error. Never turn it
		// into an empty document that could be overwritten by a later PUT.
		return nil, err
	}
	return boardResponse(&row, document), nil
}

func WorldClueBoardPut(worldID, boardKey, actorID string, input WorldClueBoardPutInput) (*protocol.WorldClueBoardWriteResult, error) {
	if err := validateWorldClueBoardScope(boardKey); err != nil {
		return nil, err
	}
	// Authorize before parsing an attacker-controlled document so anonymous or
	// non-member callers cannot use validation/size errors as a write oracle.
	if err := ensureWorldClueBoardMember(model.GetDB(), worldID, actorID); err != nil {
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
		// Never let a PUT turn an unreadable persisted document into a clean
		// replacement. A scoped read is used only to validate the existing
		// snapshot; the actual write below still relies on the revision predicate
		// and RowsAffected for optimistic concurrency.
		var current model.WorldClueBoardModel
		if err := tx.Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ?", worldID, boardKey, protocol.WorldClueBoardScopePersonal, actorID).Limit(1).Find(&current).Error; err != nil {
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
				WorldID: worldID, BoardKey: boardKey, Scope: protocol.WorldClueBoardScopePersonal,
				OwnerUserID: actorID, Revision: 1, DocumentJSON: string(canonical), UpdatedBy: actorID,
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
			result = protocol.WorldClueBoardWriteResult{WorldID: worldID, BoardKey: boardKey, Scope: protocol.WorldClueBoardScopePersonal, Revision: 1, UpdatedAt: now.UnixMilli()}
			return nil
		}
		now := time.Now()
		updated := tx.Model(&model.WorldClueBoardModel{}).
			Where("world_id = ? AND board_key = ? AND scope = ? AND owner_user_id = ? AND revision = ?", worldID, boardKey, protocol.WorldClueBoardScopePersonal, actorID, expected).
			Updates(map[string]any{"document_json": string(canonical), "revision": expected + 1, "updated_by": actorID, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrWorldClueBoardConflict
		}
		result = protocol.WorldClueBoardWriteResult{WorldID: worldID, BoardKey: boardKey, Scope: protocol.WorldClueBoardScopePersonal, Revision: expected + 1, UpdatedAt: now.UnixMilli()}
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
