package model

// SearchV2Folder is a folder entry in list-v2 responses.
type SearchV2Folder struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

// SearchV2Result matches storage-js listV2() response shape.
type SearchV2Result struct {
	HasNext    bool             `json:"hasNext"`
	Folders    []SearchV2Folder `json:"folders"`
	Objects    []FileObject     `json:"objects"`
	NextCursor string           `json:"nextCursor,omitempty"`
}
