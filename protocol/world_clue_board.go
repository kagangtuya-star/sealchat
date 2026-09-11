package protocol

import "encoding/json"

const (
	WorldClueBoardScopePersonal = "personal"
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
