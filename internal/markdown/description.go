package markdown

import "encoding/json"

// DefaultDescriptionJSON returns data as JSON when non-empty; otherwise the canonical empty ProseMirror doc.
func DefaultDescriptionJSON(data []byte) json.RawMessage {
	if len(data) == 0 {
		return json.RawMessage(EmptyDocumentJSON)
	}
	return json.RawMessage(data)
}
