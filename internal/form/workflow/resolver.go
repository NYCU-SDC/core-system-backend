package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"NYCU-SDC/core-system-backend/internal/form/question"

	"github.com/google/uuid"
)

// ResolveSections traverses the workflow and returns an ordered list of section IDs
// that should be filled based on the provided answers.
//
// The method starts from the start node and follows the workflow path:
// - For section nodes: records the section ID and continues to next
// - For condition nodes: evaluates the condition based on answers and follows nextTrue or nextFalse
// - For end nodes: stops traversal
//
// If a condition cannot be evaluated (answer doesn't exist), the method stops and returns
// only the sections that are certain to be filled up to that point (simplified version).
func (s *Service) ResolveSections(ctx context.Context, formID uuid.UUID, answers []Answer, answerableMap map[string]question.Answerable) ([]uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "ResolveSections")
	defer span.End()

	// Get the workflow for this form
	workflowRow, err := s.queries.Get(ctx, formID)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to get workflow for form %s: %w", formID, err)
	}

	// Parse workflow JSON into nodes
	var nodes []map[string]interface{}
	if err := json.Unmarshal(workflowRow.Workflow, &nodes); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}

	// Build node map for quick lookup
	nodeMap := make(map[string]map[string]interface{})
	for _, node := range nodes {
		if id, ok := node["id"].(string); ok {
			nodeMap[id] = node
		}
	}

	// Build answer map for quick lookup (questionID -> answer value)
	answerMap := make(map[string][]byte)
	for _, answer := range answers {
		answerMap[answer.QuestionID.String()] = answer.Value
	}

	// Find start node
	var currentNodeID string
	for _, node := range nodes {
		if nodeType, ok := node["type"].(string); ok && nodeType == "start" {
			currentNodeID, _ = node["id"].(string)
			break
		}
	}

	if currentNodeID == "" {
		return nil, fmt.Errorf("start node not found in workflow")
	}

	// Traverse the workflow and collect section IDs
	var sectionIDs []uuid.UUID
	visited := make(map[string]bool) // Prevent infinite loops

	for currentNodeID != "" {
		// Check for cycles
		if visited[currentNodeID] {
			return nil, fmt.Errorf("cycle detected in workflow at node %s", currentNodeID)
		}
		visited[currentNodeID] = true

		currentNode, exists := nodeMap[currentNodeID]
		if !exists {
			return nil, fmt.Errorf("node %s not found in workflow", currentNodeID)
		}

		nodeType, ok := currentNode["type"].(string)
		if !ok {
			return nil, fmt.Errorf("node %s has no type", currentNodeID)
		}

		switch nodeType {
		case "start":
			// Move to next node
			next, _ := currentNode["next"].(string)
			currentNodeID = next

		case "section":
			// Record this section ID
			sectionID, err := uuid.Parse(currentNodeID)
			if err != nil {
				return nil, fmt.Errorf("invalid section node ID %s: %w", currentNodeID, err)
			}
			sectionIDs = append(sectionIDs, sectionID)

			// Move to next node
			next, _ := currentNode["next"].(string)
			currentNodeID = next

		case "condition":
			// Evaluate condition and determine next node
			nextNodeID, canEvaluate, err := s.evaluateCondition(currentNode, answerMap, answerableMap)
			if err != nil {
				span.RecordError(err)
				return nil, fmt.Errorf("failed to evaluate condition at node %s: %w", currentNodeID, err)
			}

			// If cannot evaluate (answer doesn't exist), stop here
			if !canEvaluate {
				return sectionIDs, nil
			}

			currentNodeID = nextNodeID

		case "end":
			// Reached the end, stop traversal
			return sectionIDs, nil

		default:
			return nil, fmt.Errorf("unknown node type %s at node %s", nodeType, currentNodeID)
		}
	}

	return sectionIDs, nil
}

// evaluateCondition evaluates a condition node and returns the next node ID to follow.
// Returns (nextNodeID, canEvaluate, error).
// If canEvaluate is false, it means the answer needed for evaluation doesn't exist.
func (s *Service) evaluateCondition(conditionNode map[string]interface{}, answerMap map[string][]byte, answerableMap map[string]question.Answerable) (string, bool, error) {
	// Extract conditionRule
	conditionRuleRaw, ok := conditionNode["conditionRule"]
	if !ok {
		return "", false, fmt.Errorf("condition node missing conditionRule")
	}

	// Parse conditionRule
	conditionRuleBytes, err := json.Marshal(conditionRuleRaw)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal conditionRule: %w", err)
	}

	var conditionRule struct {
		Source  string `json:"source"`  // "choice" or "nonChoice"
		NodeID  string `json:"nodeId"`  // Referenced section node ID (unused in resolver)
		Key     string `json:"key"`     // Question ID
		Pattern string `json:"pattern"` // Regex pattern
	}

	if err := json.Unmarshal(conditionRuleBytes, &conditionRule); err != nil {
		return "", false, fmt.Errorf("failed to unmarshal conditionRule: %w", err)
	}

	// Get the answer for this question
	answerValue, answerExists := answerMap[conditionRule.Key]
	if !answerExists {
		// Answer doesn't exist, cannot evaluate
		return "", false, nil
	}

	// Get the answerable for this question
	answerable, answerableExists := answerableMap[conditionRule.Key]
	if !answerableExists {
		// Question doesn't exist, cannot evaluate
		return "", false, nil
	}

	// Use MatchesPattern from the Answerable interface
	conditionResult, err := answerable.MatchesPattern(answerValue, conditionRule.Pattern)
	if err != nil {
		return "", false, fmt.Errorf("failed to match pattern for question %s: %w", conditionRule.Key, err)
	}

	// Return the appropriate next node based on condition result
	if conditionResult {
		nextTrue, _ := conditionNode["nextTrue"].(string)
		return nextTrue, true, nil
	}

	nextFalse, _ := conditionNode["nextFalse"].(string)
	return nextFalse, true, nil
}
