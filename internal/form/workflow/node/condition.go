package node

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// ConditionNode represents a condition node
type ConditionNode struct {
	node map[string]interface{}
}

func NewConditionNode(node map[string]interface{}) (Validatable, error) {
	return &ConditionNode{node: node}, nil
}

func (n *ConditionNode) Validate(ctx context.Context, formID uuid.UUID, nodeMap map[string]map[string]interface{}, questionStore QuestionStore) error {
	nodeID, _ := n.node["id"].(string)

	// Validate field names (check for typos and invalid fields)
	err := n.validateFieldNames(nodeID)
	if err != nil {
		return err
	}

	// Condition node must have nextTrue and nextFalse
	nextTrue, ok := n.node["nextTrue"].(string)
	if !ok || nextTrue == "" {
		return fmt.Errorf("condition node '%s' must have a 'nextTrue' field", nodeID)
	}

	nextFalse, ok := n.node["nextFalse"].(string)
	if !ok || nextFalse == "" {
		return fmt.Errorf("condition node '%s' must have a 'nextFalse' field", nodeID)
	}

	// Validate that nextTrue and nextFalse nodes exist
	_, exists := nodeMap[nextTrue]
	if !exists {
		return fmt.Errorf("condition node '%s' references non-existent node '%s' in nextTrue", nodeID, nextTrue)
	}
	if _, exists := nodeMap[nextFalse]; !exists {
		return fmt.Errorf("condition node '%s' references non-existent node '%s' in nextFalse", nodeID, nextFalse)
	}

	// Validate conditionRule
	conditionRuleRaw, ok := n.node["conditionRule"]
	if !ok {
		return fmt.Errorf("condition node '%s' must have a 'conditionRule' field", nodeID)
	}

	// Parse conditionRule
	conditionRuleBytes, err := json.Marshal(conditionRuleRaw)
	if err != nil {
		return fmt.Errorf("condition node '%s' has invalid conditionRule format: %w", nodeID, err)
	}

	var conditionRule ConditionRule
	if err := json.Unmarshal(conditionRuleBytes, &conditionRule); err != nil {
		return fmt.Errorf("condition node '%s' has invalid conditionRule format: %w", nodeID, err)
	}

	// Validate conditionRule fields
	err = n.validateConditionRule(ctx, formID, nodeID, conditionRule, nodeMap, questionStore)
	if err != nil {
		return err
	}

	return nil
}

// validateFieldNames validates that the node only contains valid field names
func (n *ConditionNode) validateFieldNames(nodeID string) error {
	validFields := map[string]bool{
		"id":            true,
		"type":          true,
		"label":         true,
		"nextTrue":      true,
		"nextFalse":     true,
		"conditionRule": true,
	}

	var invalidFields []string
	for fieldName := range n.node {
		if !validFields[fieldName] {
			invalidFields = append(invalidFields, fieldName)
		}
	}

	if len(invalidFields) > 0 {
		return fmt.Errorf("condition node '%s' contains invalid field(s): %v. Valid fields are: conditionRule, id, label, nextFalse, nextTrue, type", nodeID, invalidFields)
	}

	return nil
}

func (n *ConditionNode) validateConditionRule(ctx context.Context, formID uuid.UUID, nodeID string, rule ConditionRule, nodeMap map[string]map[string]interface{}, questionStore QuestionStore) error {
	// Validate source
	if rule.Source != ConditionSourceChoice && rule.Source != ConditionSourceNonChoice {
		return fmt.Errorf("condition node '%s' has invalid conditionRule.source: '%s'", nodeID, rule.Source)
	}

	// Validate nodeId exists
	_, exists := nodeMap[rule.NodeID]
	if !exists {
		return fmt.Errorf("condition node '%s' references non-existent node '%s' in conditionRule.nodeId", nodeID, rule.NodeID)
	}

	// Validate key
	if rule.Key == "" {
		return fmt.Errorf("condition node '%s' conditionRule.key cannot be empty", nodeID)
	}

	// Validate pattern (required for both choice and nonChoice sources)
	if rule.Pattern == "" {
		return fmt.Errorf("condition node '%s' conditionRule.pattern cannot be empty", nodeID)
	}

	// Validate pattern is a valid regex
	_, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return fmt.Errorf("condition node '%s' conditionRule.pattern is not a valid regex: %w", nodeID, err)
	}

	// Validate question ID exists and type matches condition source
	if questionStore != nil {
		questionID, err := uuid.Parse(rule.Key)
		if err != nil {
			return fmt.Errorf("condition node '%s' conditionRule.key '%s' is not a valid UUID", nodeID, rule.Key)
		}

		answerable, err := questionStore.GetByID(ctx, questionID)
		if err != nil {
			return fmt.Errorf("condition node '%s' references non-existent question '%s' in conditionRule.key", nodeID, rule.Key)
		}

		q := answerable.Question()

		// Validate question belongs to the form
		if q.FormID != formID {
			return fmt.Errorf("condition node '%s' references question '%s' that belongs to a different form", nodeID, rule.Key)
		}

		// Validate question type matches condition source
		switch rule.Source {
		case ConditionSourceChoice:
			// Choice source requires single_choice or multiple_choice question type
			if string(q.Type) != "single_choice" && string(q.Type) != "multiple_choice" {
				return fmt.Errorf("condition node '%s' with source 'choice' requires question type 'single_choice' or 'multiple_choice', but question '%s' has type '%s'", nodeID, rule.Key, q.Type)
			}
		case ConditionSourceNonChoice:
			// NonChoice source requires short_text, long_text, or date question type
			if string(q.Type) != "short_text" && string(q.Type) != "long_text" && string(q.Type) != "date" {
				return fmt.Errorf("condition node '%s' with source 'nonChoice' requires question type 'short_text', 'long_text', or 'date', but question '%s' has type '%s'", nodeID, rule.Key, q.Type)
			}
		}
	}

	return nil
}
