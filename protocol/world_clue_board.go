package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type WorldClueBoardEventPayload struct {
	Operation *WorldClueBoardOperation `json:"operation,omitempty"`
	WorldID   string                   `json:"worldId"`
	BoardKey  string                   `json:"boardKey"`
	Scope     string                   `json:"scope"`
	Revision  int64                    `json:"revision"`
	UpdatedBy string                   `json:"updatedBy,omitempty"`
	ClientID  string                   `json:"clientId,omitempty"`
}

type WorldClueBoardOperationType string

const (
	WorldClueBoardPlacementsPut  WorldClueBoardOperationType = "placements.put"
	WorldClueBoardRelationPut    WorldClueBoardOperationType = "relation.put"
	WorldClueBoardRelationRemove WorldClueBoardOperationType = "relation.remove"
	WorldClueBoardQuickdrawDiff  WorldClueBoardOperationType = "quickdraw.diff"
)

type WorldClueBoardOperation struct {
	ID            string                             `json:"id"`
	Type          WorldClueBoardOperationType        `json:"type"`
	Placements    map[string]WorldClueBoardPlacement `json:"placements,omitempty"`
	Relation      *WorldClueBoardRelation            `json:"relation,omitempty"`
	RelationID    string                             `json:"relationId,omitempty"`
	QuickdrawDiff json.RawMessage                    `json:"quickdrawDiff,omitempty"`
}

// Preserve required-coordinate validation before decoding into zero-valued floats.
func (operation *WorldClueBoardOperation) UnmarshalJSON(data []byte) error {
	type operationAlias WorldClueBoardOperation
	var decoded operationAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Type == WorldClueBoardPlacementsPut {
		var raw struct {
			Placements map[string]map[string]json.RawMessage `json:"placements"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		for _, fields := range raw.Placements {
			for _, name := range []string{"x", "y"} {
				value := fields[name]
				if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return fmt.Errorf("placement %s is required", name)
				}
			}
		}
	}
	*operation = WorldClueBoardOperation(decoded)
	return nil
}

const (
	WorldClueBoardScopePersonal = "personal"
	WorldClueBoardScopeShared   = "shared"
	WorldClueBoardKeyMain       = "main"
	WorldClueBoardVersion       = 1
	WorldClueBoardQuickdrawV020 = "@quickdrawjs/core@0.2.0"
)

type WorldClueBoardPlacement struct {
	X      float64  `json:"x"`
	Y      float64  `json:"y"`
	Width  *float64 `json:"width,omitempty"`
	Pinned *bool    `json:"pinned,omitempty"`
}

type WorldClueBoardRelationKind string

const (
	WorldClueBoardRelationRelated     WorldClueBoardRelationKind = "related"
	WorldClueBoardRelationReferences  WorldClueBoardRelationKind = "references"
	WorldClueBoardRelationSupports    WorldClueBoardRelationKind = "supports"
	WorldClueBoardRelationContradicts WorldClueBoardRelationKind = "contradicts"
	WorldClueBoardRelationCauses      WorldClueBoardRelationKind = "causes"
)

type WorldClueBoardRelationEndpointKind string

const (
	WorldClueBoardRelationEndpointClue      WorldClueBoardRelationEndpointKind = "clue"
	WorldClueBoardRelationEndpointQuickdraw WorldClueBoardRelationEndpointKind = "quickdraw"
)

type WorldClueBoardRelationEndpointRef struct {
	Kind WorldClueBoardRelationEndpointKind `json:"kind"`
	ID   string                             `json:"id"`
}

type WorldClueBoardRelation struct {
	ID        string                            `json:"id"`
	SourceRef WorldClueBoardRelationEndpointRef `json:"sourceRef"`
	TargetRef WorldClueBoardRelationEndpointRef `json:"targetRef"`
	Kind      WorldClueBoardRelationKind        `json:"kind"`
	Label     string                            `json:"label,omitempty"`
}

// UnmarshalJSON keeps v1 documents readable while canonical writes use refs.
func (relation *WorldClueBoardRelation) UnmarshalJSON(data []byte) error {
	type relationAlias WorldClueBoardRelation
	var decoded relationAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var legacy struct {
		SourceClueID string `json:"sourceClueId"`
		TargetClueID string `json:"targetClueId"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	if decoded.SourceRef.ID == "" && legacy.SourceClueID != "" {
		decoded.SourceRef = WorldClueBoardRelationEndpointRef{Kind: WorldClueBoardRelationEndpointClue, ID: legacy.SourceClueID}
	}
	if decoded.TargetRef.ID == "" && legacy.TargetClueID != "" {
		decoded.TargetRef = WorldClueBoardRelationEndpointRef{Kind: WorldClueBoardRelationEndpointClue, ID: legacy.TargetClueID}
	}
	*relation = WorldClueBoardRelation(decoded)
	return nil
}

type WorldClueBoardQuickdraw struct {
	EngineVersion string          `json:"engineVersion"`
	Snapshot      json.RawMessage `json:"snapshot"`
}

// WorldClueBoardDocument is the v1 persisted board document. Quickdraw's
// public Snapshot shape is intentionally kept opaque to Go as RawMessage.
type WorldClueBoardDocument struct {
	Version    int                                `json:"version"`
	Placements map[string]WorldClueBoardPlacement `json:"placements"`
	Relations  []WorldClueBoardRelation           `json:"relations"`
	Quickdraw  *WorldClueBoardQuickdraw           `json:"quickdraw"`
}

type WorldClueBoardResponse struct {
	CanWrite    bool                   `json:"canWrite"`
	WorldID     string                 `json:"worldId"`
	BoardKey    string                 `json:"boardKey"`
	Scope       string                 `json:"scope"`
	OwnerUserID string                 `json:"ownerUserId"`
	Revision    int64                  `json:"revision"`
	Document    WorldClueBoardDocument `json:"document"`
	UpdatedAt   int64                  `json:"updatedAt"`
}

type WorldClueBoardWriteResult struct {
	WorldID   string `json:"worldId"`
	BoardKey  string `json:"boardKey"`
	Scope     string `json:"scope"`
	Revision  int64  `json:"revision"`
	UpdatedAt int64  `json:"updatedAt"`
}
