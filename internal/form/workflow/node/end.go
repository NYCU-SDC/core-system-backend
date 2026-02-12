package node

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// EndNode represents an end node
type EndNode struct {
	node map[string]interface{}
}

func NewEndNode(node map[string]interface{}) (Validatable, error) {
	return &EndNode{node: node}, nil
}

func (n *EndNode) Validate(ctx context.Context, formID uuid.UUID, nodeMap map[string]map[string]interface{}, questionStore QuestionStore) error {
	nodeID, _ := n.node["id"].(string)

	// Validate field names (check for typos and invalid fields)
	err := n.validateFieldNames(nodeID)
	if err != nil {
		return err
	}

	return nil
}

// validateFieldNames validates that the node only contains valid field names
func (n *EndNode) validateFieldNames(nodeID string) error {
	validFields := map[string]bool{
		"id":    true,
		"type":  true,
		"label": true,
	}

	var invalidFields []string
	for fieldName := range n.node {
		if !validFields[fieldName] {
			invalidFields = append(invalidFields, fieldName)
		}
	}

	if len(invalidFields) > 0 {
		return fmt.Errorf("end node '%s' contains invalid field(s): %v. Valid fields are: id, label, type", nodeID, invalidFields)
	}

	return nil
}
