package node

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// StartNode represents a start node
type StartNode struct {
	node map[string]interface{}
}

func NewStartNode(node map[string]interface{}) (Validatable, error) {
	return &StartNode{node: node}, nil
}

func (n *StartNode) Validate(ctx context.Context, formID uuid.UUID, nodeMap map[string]map[string]interface{}, questionStore QuestionStore) error {
	nodeID, _ := n.node["id"].(string)

	// Validate field names (check for typos and invalid fields)
	err := n.validateFieldNames(nodeID)
	if err != nil {
		return err
	}

	// Start node must have a next field
	next, ok := n.node["next"].(string)
	if !ok || next == "" {
		return fmt.Errorf("start node '%s' must have a 'next' field", nodeID)
	}

	// Validate that next node exists
	_, exists := nodeMap[next]
	if !exists {
		return fmt.Errorf("start node '%s' references non-existent node '%s' in next", nodeID, next)
	}

	return nil
}

// validateFieldNames validates that the node only contains valid field names
func (n *StartNode) validateFieldNames(nodeID string) error {
	validFields := map[string]bool{
		"id":    true,
		"type":  true,
		"label": true,
		"next":  true,
	}

	var invalidFields []string
	for fieldName := range n.node {
		if !validFields[fieldName] {
			invalidFields = append(invalidFields, fieldName)
		}
	}

	if len(invalidFields) > 0 {
		return fmt.Errorf("start node '%s' contains invalid field(s): %v. Valid fields are: id, label, next, type", nodeID, invalidFields)
	}

	return nil
}
