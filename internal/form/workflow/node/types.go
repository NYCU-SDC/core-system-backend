package node

import (
	"context"
	"fmt"

	"NYCU-SDC/core-system-backend/internal/form/question"

	"github.com/google/uuid"
)

// QuestionStore defines the interface for querying form questions
type QuestionStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

// Validatable defines the interface for node validation
// Similar to Answerable in question package
type Validatable interface {
	Validate(ctx context.Context, formID uuid.UUID, nodeMap map[string]map[string]interface{}, questionStore QuestionStore) error
}

// ConditionSource represents the source type for condition rules
// Reference to question.ChoiceTypes and question.NonChoiceTypes
type ConditionSource string

const (
	ConditionSourceChoice    ConditionSource = "choice"
	ConditionSourceNonChoice ConditionSource = "nonChoice"
)

// ConditionRule represents a condition rule for condition nodes
type ConditionRule struct {
	Source         ConditionSource `json:"source"`
	NodeID         string          `json:"nodeId"`
	Key            string          `json:"key"`
	ChoiceOptionID string          `json:"choiceOptionId,omitempty"` // For choice source
	Pattern        string          `json:"pattern"`
}

// Node type constants to avoid importing workflow package
const (
	TypeStart     = "start"
	TypeSection   = "section"
	TypeCondition = "condition"
	TypeEnd       = "end"
)

// NewNode creates a Validatable instance based on node type.
// Returns the node type as a string to avoid circular dependency.
func New(node map[string]interface{}) (Validatable, string, error) {
	nodeType, ok := node["type"].(string)
	if !ok || nodeType == "" {
		return nil, "", fmt.Errorf("node missing required field 'type'")
	}

	switch nodeType {
	case TypeStart:
		validatable, err := NewStartNode(node)
		return validatable, nodeType, err
	case TypeSection:
		validatable, err := NewSectionNode(node)
		return validatable, nodeType, err
	case TypeCondition:
		validatable, err := NewConditionNode(node)
		return validatable, nodeType, err
	case TypeEnd:
		validatable, err := NewEndNode(node)
		return validatable, nodeType, err
	default:
		return nil, "", fmt.Errorf("unsupported node type: %s", nodeType)
	}
}
