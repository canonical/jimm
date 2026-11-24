// Copyright 2026 Canonical.

package params

// EntityMetadataTarget identifies a model or an application within a model.
// ModelTag is required for application targets.
type EntityMetadataTarget struct {
	EntityTag string `json:"entity-tag"`
	ModelTag  string `json:"model-tag,omitempty"`
}

type GetMetadataRequest struct {
	Entity EntityMetadataTarget `json:"entity"`
}

type SetMetadataRequest struct {
	Entity   EntityMetadataTarget `json:"entity"`
	Metadata map[string]string    `json:"metadata"`
}

type RemoveMetadataRequest struct {
	Entity EntityMetadataTarget `json:"entity"`
	All    bool                 `json:"all"`
	Keys   []string             `json:"keys,omitempty"`
}

type QueryMetadataRequest struct {
	Matcher *MetadataMatcher `json:"matcher"`
}

// MetadataMatcher matches an exact key/value pair. The key must not be empty.
type MetadataMatcher struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MetadataResults struct {
	Metadata map[string]string `json:"metadata"`
}

type QueryMetadataResponse struct {
	Results []MetadataMatch `json:"results"`
}

type MetadataMatch struct {
	Entity EntityMetadataTarget `json:"entity"`
	Key    string               `json:"key"`
	Value  string               `json:"value"`
}
