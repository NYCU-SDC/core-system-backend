package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"NYCU-SDC/core-system-backend/internal/form/question"
	"NYCU-SDC/core-system-backend/internal/form/workflow/node"

	"github.com/google/uuid"
)

// uuidPattern matches UUID format in error messages
const uuidPattern = `([a-f0-9-]{36})`

// nodeIDPatternTemplates defines all patterns for extracting node IDs from error messages.
// Each template uses %s as placeholder for the UUID pattern.
// Order matters: more specific patterns (with node type) should come before generic ones.
var nodeIDPatternTemplates = []string{
	// Node type specific patterns: "start node 'uuid'", "section node 'uuid'", etc.
	node.TypeStart + " node '%s'",
	node.TypeSection + " node '%s'",
	node.TypeCondition + " node '%s'",
	node.TypeEnd + " node '%s'",
	// Generic node pattern: "node 'uuid' is unreachable"
	"node '%s'",
	// Duplicate node ID pattern: "duplicate node id 'uuid'"
	"duplicate node id '%s'",
}

// nodeIDPatterns contains compiled regex patterns for extracting node IDs from error messages.
var nodeIDPatterns = func() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(nodeIDPatternTemplates))
	for _, template := range nodeIDPatternTemplates {
		pattern := regexp.MustCompile(fmt.Sprintf(template, uuidPattern))
		patterns = append(patterns, pattern)
	}
	return patterns
}()

// ValidationInfoType represents the category of validation error
type ValidationInfoType string

const (
	ValidationTypeWorkflow ValidationInfoType = "workflow_validation_failed"
	ValidationTypeGraph    ValidationInfoType = "graph_validation_failed"
	ValidationTypeNode     ValidationInfoType = "node_validation_failed"
	ValidationTypeUnknown  ValidationInfoType = "unknown"
)

// QuestionStore defines the interface for querying form questions
// This allows the validator to check if condition rule question IDs exist and match expected types
type QuestionStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (question.Answerable, error)
}

type workflowValidator struct{}

// ValidateNodeIDsUnchanged checks that node IDs in the new workflow match the current workflow.
// It ensures that no node IDs are added or removed during an update.
func (v workflowValidator) ValidateNodeIDsUnchanged(ctx context.Context, currentWorkflow, newWorkflow []byte) error {
	return validateNodeIDsUnchanged(currentWorkflow, newWorkflow)
}

// ValidateUpdateNodeIDs validates that node IDs haven't changed during an update.
// If currentWorkflow is nil, validation is skipped (first update scenario).
// Otherwise, it ensures that no node IDs are added or removed.
func (v workflowValidator) ValidateUpdateNodeIDs(ctx context.Context, currentWorkflow []byte, newWorkflow []byte) error {
	// If workflow doesn't exist yet (first update), skip node ID validation
	if currentWorkflow == nil {
		// First update - no existing workflow to compare against
		return nil
	}

	// Validate that node IDs haven't changed
	return validateNodeIDsUnchanged(currentWorkflow, newWorkflow)
}

// NewValidator creates a new workflow validator using the built-in NodeType set.
func NewValidator() Validator {
	return workflowValidator{}
}

// Activate validates the workflow JSON structure before activation.
// It checks:
// - Valid JSON format
// - Array structure
// - Node structure and required fields
// - Valid node types
// - Reachability (all nodes are reachable)
// - Condition rule question IDs exist and types match
// Returns all validation errors if validation fails
func (v workflowValidator) Activate(ctx context.Context, formID uuid.UUID, workflow []byte, questionStore QuestionStore) error {
	nodes, err := parseWorkflow(workflow)
	if err != nil {
		return err
	}

	err = validateNodeCount(nodes)
	if err != nil {
		return err
	}

	nodes, nodeMap, validationErrors, err := runCommonWorkflowValidation(ctx, formID, nodes, questionStore, true)
	if err != nil {
		return err
	}

	err = formatValidationErrors(validationErrors)
	if err != nil {
		return err
	}

	if nodeMap != nil {
		err := validateReachability(nodes, nodeMap)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("reachability validation failed: %w", err))
		}
	}

	if len(validationErrors) > 0 {
		return fmt.Errorf("validation failed: %w", errors.Join(validationErrors...))
	}
	return nil
}

// Validate performs validation for workflows (used by Update).
func (v workflowValidator) Validate(ctx context.Context, formID uuid.UUID, workflow []byte, questionStore QuestionStore) error {
	nodes, err := parseWorkflow(workflow)
	if err != nil {
		return err
	}

	err = validateNodeCount(nodes)
	if err != nil {
		return err
	}

	nodes, nodeMap, validationErrors, err := runCommonWorkflowValidation(ctx, formID, nodes, questionStore, false)
	if err != nil {
		return err
	}

	err = formatValidationErrors(validationErrors)
	if err != nil {
		return err
	}

	if nodeMap != nil {
		err := validateReference(nodes, nodeMap)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("graph references validation failed: %w", err))
		}

		// Validate that condition nodes don't reference sections that come after them
		err = validateConditionSectionOrder(nodes)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("condition section order validation failed: %w", err))
		}

		if questionStore != nil {
			for _, n := range nodes {
				nodeType, _ := n["type"].(string)
				if nodeType != string(NodeTypeCondition) {
					continue
				}

				rawRule, ok := n["conditionRule"]
				if !ok {
					continue
				}

				nodeID, _ := n["id"].(string)

				err := validateDraftConditionQuestion(ctx, formID, nodeID, rawRule, questionStore)
				if err != nil {
					validationErrors = append(validationErrors, err)
				}
			}
		}
	}

	err = formatValidationErrors(validationErrors)
	if err != nil {
		return err
	}

	return nil
}

// formatValidationErrors joins and wraps validation errors in a consistent way.
func formatValidationErrors(validationErrors []error) error {
	if len(validationErrors) == 0 {
		return nil
	}

	return fmt.Errorf("validation failed: %w", errors.Join(validationErrors...))
}

// parseWorkflow parses workflow JSON into nodes. Call once at entry point and pass nodes to helpers.
func parseWorkflow(workflow []byte) ([]map[string]interface{}, error) {
	var nodes []map[string]interface{}
	err := json.Unmarshal(workflow, &nodes)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}
	return nodes, nil
}

// validateNodeCount validates that the workflow has at least 2 nodes (start and end).
func validateNodeCount(nodes []map[string]interface{}) error {
	if len(nodes) < 2 {
		return fmt.Errorf("workflow must contain at least 2 nodes, one start node and one end node")
	}
	return nil
}

// runCommonWorkflowValidation performs shared validation: node validation and required node types.
// Caller must parse workflow and validate node count before calling. Returns (nodes, nodeMap, validationErrors, err).
func runCommonWorkflowValidation(
	ctx context.Context,
	formID uuid.UUID,
	nodes []map[string]interface{},
	questionStore QuestionStore,
	isActivate bool,
) ([]map[string]interface{}, map[string]map[string]interface{}, []error, error) {
	var errs []error
	nodeMap, startCount, endCount, nodeErrs := validateNodes(ctx, formID, nodes, questionStore, isActivate)
	if len(nodeErrs) > 0 {
		errs = append(errs, nodeErrs...)
	}

	nodeTypeErrs := validateRequiredNodeTypes(startCount, endCount)
	if len(nodeTypeErrs) > 0 {
		errs = append(errs, nodeTypeErrs...)
	}

	return nodes, nodeMap, errs, nil
}

// validateRequiredFields validates that a node has all required fields: id, type, label
func validateRequiredFields(node map[string]interface{}, index int) error {
	// Check for 'id' field
	_, ok := node["id"]
	if !ok || node["id"] == "" {
		return fmt.Errorf("node at index %d missing required field 'id'", index)
	}

	// Check for 'type' field
	_, ok = node["type"]
	if !ok || node["type"] == "" {
		return fmt.Errorf("node at index %d missing required field 'type'", index)
	}

	// Check for 'label' field
	_, ok = node["label"]
	if !ok || node["label"] == "" {
		return fmt.Errorf("node at index %d missing required field 'label'", index)
	}

	return nil
}

// validateNodeID validates that a node has a valid ID field
func validateNodeID(node map[string]interface{}, index int) (string, error) {
	id, ok := node["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("node at index %d missing required field 'id'", index)
	}

	// Validate UUID format
	_, err := uuid.Parse(id)
	if err != nil || id == "" {
		return "", fmt.Errorf("node at index %d has invalid UUID format for id '%s': %w", index, id, err)
	}

	return id, nil
}

// validateNodes validates all nodes in the workflow and returns:
// - nodeMap: map of node ID to node
// - startNodeCount: number of start nodes found
// - endNodeCount: number of end nodes found
// - errors: all validation errors collected
// When isActivate is true, this performs full node-specific validation (used by Activate).
// When false, it performs a relaxed validation suitable for draft Update.
func validateNodes(ctx context.Context, formID uuid.UUID, nodes []map[string]interface{}, questionStore QuestionStore, isActivate bool) (map[string]map[string]interface{}, int, int, []error) {
	nodeMap := make(map[string]map[string]interface{})
	nodeIDs := make(map[string]bool)
	validatedNodes := make([]node.Validatable, 0, len(nodes))
	startNodeCount := 0
	endNodeCount := 0
	var validationErrors []error

	for i, nodeData := range nodes {
		// Validate required fields (id, type, label)
		if err := validateRequiredFields(nodeData, i); err != nil {
			validationErrors = append(validationErrors, err)
			continue // Skip this node but continue validating others
		}

		// Validate node ID
		id, err := validateNodeID(nodeData, i)
		if err != nil {
			validationErrors = append(validationErrors, err)
			continue // Skip this node but continue validating others
		}

		// Check for duplicate node IDs
		if nodeIDs[id] {
			validationErrors = append(validationErrors, fmt.Errorf("duplicate node id '%s' at index %d", id, i))
			continue // Skip this node but continue validating others
		}
		nodeIDs[id] = true
		nodeMap[id] = nodeData

		// Validate node type and build Validatable instance
		validatedNode, nodeTypeStr, err := node.New(nodeData)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("node at index %d: %w", i, err))
			continue // Skip this node but continue validating others
		}
		if isActivate {
			validatedNodes = append(validatedNodes, validatedNode)
		}

		// Count start and end nodes
		if nodeTypeStr == string(NodeTypeStart) {
			startNodeCount++
		}
		if nodeTypeStr == string(NodeTypeEnd) {
			endNodeCount++
		}
	}

	// Validate each node using Validatable interface
	// This validates node-specific rules (e.g., condition nodes must have conditionRule)
	// Only validate nodes that were successfully created
	if isActivate {
		for i, validatedNode := range validatedNodes {
			// Pass context, formID, and questionStore for condition rule validation
			err := validatedNode.Validate(ctx, formID, nodeMap, questionStore)
			if err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("node at index %d: %w", i, err))
			}
		}
	}

	return nodeMap, startNodeCount, endNodeCount, validationErrors
}

// validateRequiredNodeTypes validates that the workflow contains exactly one start node and exactly one end node
// Returns all validation errors found
func validateRequiredNodeTypes(startNodeCount, endNodeCount int) []error {
	var validationErrors []error

	if startNodeCount != 1 {
		validationErrors = append(validationErrors, fmt.Errorf("workflow must contain exactly one start node, found %d", startNodeCount))
	}

	if endNodeCount != 1 {
		validationErrors = append(validationErrors, fmt.Errorf("workflow must contain exactly one end node, found %d", endNodeCount))
	}

	return validationErrors
}

// validateReachability checks if all nodes in the workflow are reachable
// It ensures:
// - All node references (next, nextTrue, nextFalse) point to valid nodes
// - All nodes can be reached from entry points (start nodes or first node)
// - No orphaned nodes exist
func validateReachability(nodes []map[string]interface{}, nodeMap map[string]map[string]interface{}) error {
	// First validate references
	err := validateReference(nodes, nodeMap)
	if err != nil {
		return err
	}

	// Build graph for reachability validation
	graph := make(map[string][]string)

	// Build adjacency list and validate references
	for _, node := range nodes {
		nodeID, _ := node["id"].(string)
		nodeType, _ := node["type"].(string)

		var nextNodes []string

		// Handle different node types and their connection fields
		if nodeType == string(NodeTypeCondition) {
			// Condition nodes have nextTrue and nextFalse
			nextTrue, ok := node["nextTrue"].(string)
			if ok && nextTrue != "" {
				nextNodes = append(nextNodes, nextTrue)
			}
			nextFalse, ok := node["nextFalse"].(string)
			if ok && nextFalse != "" {
				nextNodes = append(nextNodes, nextFalse)
			}
		} else {
			// Other nodes have next field
			next, ok := node["next"].(string)
			if ok && next != "" {
				nextNodes = append(nextNodes, next)
			}
		}

		graph[nodeID] = nextNodes
	}

	// Find the start node (there should be exactly one)
	startNodeID := ""
	for _, node := range nodes {
		nodeID, _ := node["id"].(string)
		nodeType, _ := node["type"].(string)

		// The start node is the only entry point
		if nodeType == string(NodeTypeStart) {
			startNodeID = nodeID
			break
		}
	}

	// BFS traversal from the start node to ensure all nodes are reachable
	visited := make(map[string]bool)
	queue := make([]string, 0)

	// Start from the start node
	queue = append(queue, startNodeID)
	visited[startNodeID] = true

	// Traverse the graph
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Visit all connected nodes
		for _, nextNodeID := range graph[current] {
			if !visited[nextNodeID] {
				visited[nextNodeID] = true
				queue = append(queue, nextNodeID)
			}
		}
	}

	// Check if all nodes are reachable
	var unreachableErrors []error
	for nodeID := range nodeMap {
		if !visited[nodeID] {
			unreachableErrors = append(unreachableErrors, fmt.Errorf("node '%s' is unreachable from the start node", nodeID))
		}
	}

	if len(unreachableErrors) > 0 {
		return errors.Join(unreachableErrors...)
	}

	return nil
}

// validateReferences validates that any explicit reference fields point to nodes that exist.
func validateReference(nodes []map[string]interface{}, nodeMap map[string]map[string]interface{}) error {
	var referenceErrors []error

	for _, node := range nodes {
		nodeID, _ := node["id"].(string)
		nodeType, _ := node["type"].(string)

		if nodeType == string(NodeTypeCondition) {
			nextTrue, ok := node["nextTrue"].(string)
			if ok && nextTrue != "" {
				_, exists := nodeMap[nextTrue]
				if !exists {
					referenceErrors = append(referenceErrors, fmt.Errorf("condition node '%s' references non-existent node '%s' in nextTrue", nodeID, nextTrue))
				}
			}

			nextFalse, ok := node["nextFalse"].(string)
			if ok && nextFalse != "" {
				_, exists := nodeMap[nextFalse]
				if !exists {
					referenceErrors = append(referenceErrors, fmt.Errorf("condition node '%s' references non-existent node '%s' in nextFalse", nodeID, nextFalse))
				}
			}
		} else {
			next, ok := node["next"].(string)
			if ok && next != "" {
				_, exists := nodeMap[next]
				if !exists {
					referenceErrors = append(referenceErrors, fmt.Errorf("node '%s' references non-existent node '%s' in next", nodeID, next))
				}
			}
		}
	}

	if len(referenceErrors) > 0 {
		return fmt.Errorf("invalid node references found: %w", errors.Join(referenceErrors...))
	}
	return nil
}

// validateConditionSectionOrder checks that condition nodes reference sections
// that are visited before the condition in the workflow traversal.
// If a condition references a section that comes after it in the graph,
// the condition will always evaluate to false (section not yet visited).
// Returns error if any condition references a section that comes after it.
func validateConditionSectionOrder(nodes []map[string]interface{}) error {
	// Find the start node
	var startNodeID string
	for _, n := range nodes {
		nodeType, _ := n["type"].(string)
		if nodeType == string(NodeTypeStart) {
			startNodeID, _ = n["id"].(string)
			break
		}
	}

	if startNodeID == "" {
		// No start node found, skip this validation (other validations will catch this)
		return nil
	}

	// Build adjacency list for graph traversal
	graph := make(map[string][]string)
	for _, n := range nodes {
		nodeID, _ := n["id"].(string)
		nodeType, _ := n["type"].(string)

		var nextNodes []string
		switch nodeType {
		case string(NodeTypeCondition):
			nextTrue, ok := n["nextTrue"].(string)
			if !ok || nextTrue == "" {
				continue
			}
			nextNodes = append(nextNodes, nextTrue)
			nextFalse, ok := n["nextFalse"].(string)
			if !ok || nextFalse == "" {
				continue
			}
			nextNodes = append(nextNodes, nextFalse)
		default:
			next, ok := n["next"].(string)
			if !ok || next == "" {
				continue
			}
			nextNodes = append(nextNodes, next)
		}
		graph[nodeID] = nextNodes
	}

	// Collect all condition nodes with their conditionRule.nodeId
	type conditionInfo struct {
		conditionNodeID  string
		referencedNodeID string
	}
	var conditionsToCheck []conditionInfo

	for _, node := range nodes {
		nodeType, _ := node["type"].(string)
		if nodeType != string(NodeTypeCondition) {
			continue
		}

		nodeID, _ := node["id"].(string)
		rawRule, ok := node["conditionRule"]
		if !ok {
			continue
		}

		// Parse conditionRule to get nodeId
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}

		referencedNodeID, ok := ruleMap["nodeId"].(string)
		if !ok || referencedNodeID == "" {
			continue
		}

		conditionsToCheck = append(conditionsToCheck, conditionInfo{
			conditionNodeID:  nodeID,
			referencedNodeID: referencedNodeID,
		})
	}

	if len(conditionsToCheck) == 0 {
		return nil
	}

	// BFS traversal to determine visit order
	// Track when each node is first visited (order number)
	visitOrder := make(map[string]int)
	queue := []string{startNodeID}
	order := 0
	visitOrder[startNodeID] = order

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, nextNodeID := range graph[current] {
			_, visited := visitOrder[nextNodeID]
			if !visited {
				order++
				visitOrder[nextNodeID] = order
				queue = append(queue, nextNodeID)
			}
		}
	}

	// Check each condition: referenced section must be visited before the condition
	var orderErrors []error
	for _, cond := range conditionsToCheck {
		condOrder, condVisited := visitOrder[cond.conditionNodeID]
		refOrder, refVisited := visitOrder[cond.referencedNodeID]

		// If condition is not visited (unreachable), skip - other validation will catch it
		if !condVisited {
			continue
		}

		// If referenced node is not visited or comes after the condition
		if !refVisited || refOrder >= condOrder {
			orderErrors = append(orderErrors, fmt.Errorf(
				"condition node '%s' references section '%s' in conditionRule.nodeId that has not been visited yet; condition will always evaluate to false",
				cond.conditionNodeID, cond.referencedNodeID))
		}
	}

	if len(orderErrors) > 0 {
		return errors.Join(orderErrors...)
	}

	return nil
}

// mapToConditionRule converts a parsed conditionRule map to ConditionRule without marshal/unmarshal.
func mapToConditionRule(mapRule map[string]interface{}) node.ConditionRule {
	var rule node.ConditionRule
	source, _ := mapRule["source"].(string)
	rule.Source = node.ConditionSource(source)
	rule.NodeID, _ = mapRule["nodeId"].(string)
	rule.Key, _ = mapRule["key"].(string)
	rule.ChoiceOptionID, _ = mapRule["choiceOptionId"].(string)
	rule.Pattern, _ = mapRule["pattern"].(string)
	return rule
}

// validateDraftConditionQuestion performs the subset of conditionRule validation that must hold
// even for draft workflows: that the referenced question exists, belongs to the form, and its
// type is compatible with the condition source. It deliberately skips regex validation and
func validateDraftConditionQuestion(
	ctx context.Context,
	formID uuid.UUID,
	nodeID string,
	rawRule interface{},
	questionStore QuestionStore,
) error {
	ruleMap, ok := rawRule.(map[string]interface{})
	if !ok {
		return fmt.Errorf("condition node '%s' has invalid conditionRule format: expected object", nodeID)
	}

	rule := mapToConditionRule(ruleMap)

	// Only validate question existence and type compatibility in draft mode.
	if rule.Key == "" {
		return fmt.Errorf("condition node '%s' conditionRule.key cannot be empty", nodeID)
	}

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
	if answerable.FormID() != formID {
		return fmt.Errorf("condition node '%s' references question '%s' that belongs to a different form", nodeID, rule.Key)
	}

	// Validate question type matches condition source (same rules as strict mode).
	switch rule.Source {
	case node.ConditionSourceChoice:
		if !question.ContainsType(question.ChoiceTypes, q.Type) {
			return fmt.Errorf("condition node '%s' with source 'choice' requires question type %s, but question '%s' has type '%s'", nodeID, question.FormatAllowedTypes(question.ChoiceTypes), rule.Key, q.Type)
		}
	case node.ConditionSourceNonChoice:
		if !question.ContainsType(question.NonChoiceTypes, q.Type) {
			return fmt.Errorf("condition node '%s' with source 'nonChoice' requires question type %s, but question '%s' has type '%s'", nodeID, question.FormatAllowedTypes(question.NonChoiceTypes), rule.Key, q.Type)
		}
	}
	return nil
}

// extractNodeIDsFromNodes extracts all node IDs from parsed workflow nodes.
func extractNodeIDsFromNodes(nodes []map[string]interface{}) (map[string]bool, error) {
	nodeIDs := make(map[string]bool)
	for i, node := range nodes {
		id, err := validateNodeID(node, i)
		if err != nil {
			return nil, err
		}
		nodeIDs[id] = true
	}
	return nodeIDs, nil
}

// validateNodeIDsUnchanged checks that node IDs in the new workflow match the current workflow.
// Parses each workflow JSON once and passes parsed nodes to the comparison logic.
func validateNodeIDsUnchanged(currentWorkflow, newWorkflow []byte) error {
	currentNodes, err := parseWorkflow(currentWorkflow)
	if err != nil {
		return fmt.Errorf("failed to parse current workflow JSON: %w", err)
	}

	newNodes, err := parseWorkflow(newWorkflow)
	if err != nil {
		return fmt.Errorf("failed to parse new workflow JSON: %w", err)
	}

	currentNodeIDs, err := extractNodeIDsFromNodes(currentNodes)
	if err != nil {
		return fmt.Errorf("failed to extract node IDs from current workflow: %w", err)
	}

	newNodeIDs, err := extractNodeIDsFromNodes(newNodes)
	if err != nil {
		return fmt.Errorf("failed to extract node IDs from new workflow: %w", err)
	}

	// Check if all current node IDs exist in new workflow
	for id := range currentNodeIDs {
		if !newNodeIDs[id] {
			return fmt.Errorf("node ID '%s' was removed from workflow", id)
		}
	}

	// Check if any new node IDs were added
	for id := range newNodeIDs {
		if !currentNodeIDs[id] {
			return fmt.Errorf("node ID '%s' was added to workflow", id)
		}
	}

	return nil
}

// extractLeafErrors extracts leaf errors from a joined error (errors.Join style).
// For non-joined errors, it returns the error as-is without unwrapping.
func extractLeafErrors(err error) []error {
	if err == nil {
		return nil
	}

	// Handle joined errors (errors.Join style)
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}

	allErrors := make([]error, 0, len(joined.Unwrap()))
	for _, e := range joined.Unwrap() {
		allErrors = append(allErrors, extractLeafErrors(e)...)
	}
	return allErrors
}

// parseValidationErrors extracts individual validation errors from a joined error
// and parses them to extract node IDs and error types where applicable.
// Each line with a node ID gets its own ValidationInfo with the prefix prepended.
func parseValidationErrors(err error) []ValidationInfo {
	var validationInfos []ValidationInfo

	allErrors := extractLeafErrors(err)

	for _, e := range allErrors {
		msg := e.Error()

		// Extract type from full message (before splitting)
		errType := extractValidationType(msg)

		// Extract prefix and split into lines
		_, lines := extractPrefixAndLines(msg)

		for _, line := range lines {
			if line == "" {
				continue
			}

			nodeID := extractNodeID(line)

			validationInfos = append(validationInfos, ValidationInfo{
				NodeID:  nodeID,
				Type:    errType,
				Message: line, // Message without prefix
			})
		}
	}

	return validationInfos
}

// extractPrefixAndLines splits an error message into prefix and individual lines.
// Strips known prefixes from the content.
func extractPrefixAndLines(msg string) (string, []string) {
	prefix := ""
	content := msg

	// Extract top-level prefix
	if strings.HasPrefix(content, "workflow validation failed: ") {
		prefix = "workflow validation failed: "
		content = strings.TrimPrefix(content, prefix)
	}

	// Split remaining content by newlines
	lines := strings.Split(content, "\n")

	// Clean each line by stripping nested prefixes
	for i, line := range lines {
		lines[i] = stripNestedPrefixes(line)
	}

	return prefix, lines
}

// stripNestedPrefixes removes known prefixes from a line.
func stripNestedPrefixes(line string) string {
	result := line

	// Strip static prefixes
	staticPrefixes := []string{
		"reachability validation failed: ",
		"graph validation failed: ",
		"invalid node references found: ",
	}
	for _, p := range staticPrefixes {
		result = strings.TrimPrefix(result, p)
	}

	// Strip "node at index N: " pattern
	nodeAtIndexPattern := regexp.MustCompile(`^node at index \d+: `)
	result = nodeAtIndexPattern.ReplaceAllString(result, "")

	return result
}

// extractNodeID extracts a single node ID from a line, returns nil if none found.
func extractNodeID(line string) *string {
	for _, pattern := range nodeIDPatterns {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) > 1 {
			id := matches[1]
			// Validate it's a proper UUID
			_, parseErr := uuid.Parse(id)
			if parseErr == nil {
				return &id
			}
		}
	}
	return nil
}

// extractValidationType determines the validation error category from a message.
func extractValidationType(msg string) ValidationInfoType {
	if strings.Contains(msg, "reachability validation failed") || strings.Contains(msg, "graph validation failed") {
		return ValidationTypeGraph
	}
	if strings.Contains(msg, "node at index") || strings.HasPrefix(msg, "node ") ||
		strings.Contains(msg, " node '") {
		return ValidationTypeNode
	}
	if strings.Contains(msg, "workflow") {
		return ValidationTypeWorkflow
	}
	return ValidationTypeUnknown
}
