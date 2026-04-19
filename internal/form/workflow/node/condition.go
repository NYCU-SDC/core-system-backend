package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"NYCU-SDC/core-system-backend/internal/form/question"

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
	err = n.validateConditionRule(ctx, formID, nodeID, conditionRule, questionStore)
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
		"payload":       true,
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

// patternUUIDRegex finds substrings that look like UUIDs (8-4-4-4-12 hex) in condition patterns. Used with uuid.Parse to validate.
var patternUUIDRegex = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// extractUUIDsFromPattern returns all valid UUID substrings found in pattern (validated via github.com/google/uuid).
// Deduplicated, order preserved by first occurrence.
func extractUUIDsFromPattern(pattern string) []string {
	uuids := patternUUIDRegex.FindAllString(pattern, -1)
	if len(uuids) == 0 {
		return nil
	}

	// Deduplicate and validate UUIDs
	seen := make(map[string]bool)
	out := make([]string, 0, len(uuids))
	for _, s := range uuids {
		_, err := uuid.Parse(s)
		if err != nil {
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (n *ConditionNode) validateConditionRule(ctx context.Context, formID uuid.UUID, nodeID string, rule ConditionRule, questionStore QuestionStore) error {
	// Normalize source to uppercase for comparison (API may send "choice" or "CHOICE")
	rule.Source = ConditionSource(strings.ToUpper(string(rule.Source)))
	if rule.Source != ConditionSourceChoice && rule.Source != ConditionSourceNonChoice {
		return fmt.Errorf("condition node '%s' has invalid conditionRule.source: '%s'", nodeID, rule.Source)
	}

	// Validate question
	if rule.Question == "" {
		return fmt.Errorf("condition node '%s' conditionRule.question cannot be empty", nodeID)
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
		questionID, err := uuid.Parse(rule.Question)
		if err != nil {
			return fmt.Errorf("condition node '%s' conditionRule.question '%s' is not a valid UUID", nodeID, rule.Question)
		}

		answerable, err := questionStore.Get(ctx, questionID)
		if err != nil {
			return fmt.Errorf("condition node '%s' references non-existent question '%s' in conditionRule.question", nodeID, rule.Question)
		}

		q := answerable.Question()

		// Validate question belongs to the form
		if answerable.FormID() != formID {
			return fmt.Errorf("condition node '%s' references question '%s' that belongs to a different form", nodeID, rule.Question)
		}

		// Validate question type matches condition source
		switch rule.Source {
		case ConditionSourceChoice:
			if !question.ContainsType(question.ChoiceTypes, q.Type) {
				return fmt.Errorf("condition node '%s' with source 'CHOICE' requires question type %s, but question '%s' has type '%s'", nodeID, question.FormatAllowedTypes(question.ChoiceTypes), rule.Question, q.Type)
			}
			// For CHOICE: every UUID in the pattern must be a choice option ID of the question
			uuidsInPattern := extractUUIDsFromPattern(rule.Pattern)
			if len(uuidsInPattern) > 0 {
				choices, extractErr := question.ExtractChoices(q.Metadata)
				if extractErr != nil {
					return fmt.Errorf("condition node '%s' conditionRule.pattern references choice options but question '%s' has invalid or missing choices: %w", nodeID, rule.Question, extractErr)
				}
				if len(choices) == 0 {
					return fmt.Errorf("condition node '%s' conditionRule.pattern references choice options but question '%s' has no choices", nodeID, rule.Question)
				}
				choiceIDSet := make(map[string]bool)
				for _, c := range choices {
					choiceIDSet[c.ID.String()] = true
				}
				var errs []error
				for _, u := range uuidsInPattern {
					if !choiceIDSet[u] {
						errs = append(errs, fmt.Errorf("condition node '%s' conditionRule.pattern references non-existent choice option '%s' for question '%s'", nodeID, u, rule.Question))
					}
				}
				if len(errs) > 0 {
					return errors.Join(errs...)
				}
			}
		case ConditionSourceNonChoice:
			if !question.ContainsType(question.NonChoiceTypes, q.Type) {
				return fmt.Errorf("condition node '%s' with source 'NONCHOICE' requires question type %s, but question '%s' has type '%s'", nodeID, question.FormatAllowedTypes(question.NonChoiceTypes), rule.Question, q.Type)
			}
		}
	}

	return nil
}
