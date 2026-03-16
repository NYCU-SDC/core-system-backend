package workflow

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// SectionTitleProvider returns section IDs to titles for a form.
// Used to enrich section node labels with current section titles.
type SectionTitleProvider interface {
	SectionTitlesByFormID(ctx context.Context, formID uuid.UUID) (map[string]string, error)
}

// enrichWorkflowLabels updates workflow JSON so that section node labels
// reflect the current section title and condition node labels reflect the
// condition rule (e.g. "When [question title] matches [pattern]").
// Workflow is expected in API format (type uppercase). Returns enriched JSON.
// If sectionTitles or questionStore is nil, section or condition enrichment
// is skipped respectively; if both are nil, the workflow is returned unchanged.
func enrichWorkflowLabels(
	ctx context.Context,
	workflowJSON []byte,
	formID uuid.UUID,
	sectionTitles map[string]string,
	questionStore QuestionStore,
) ([]byte, error) {
	if sectionTitles == nil && questionStore == nil {
		return workflowJSON, nil
	}

	var nodes []map[string]interface{}
	err := json.Unmarshal(workflowJSON, &nodes)
	if err != nil {
		return workflowJSON, fmt.Errorf("%w: %w", internal.ErrUnmarshalWorkflow, err)
	}

	for i := range nodes {
		typ, _ := nodes[i]["type"].(string)
		nodeID, _ := nodes[i]["id"].(string)

		switch typ {
		case string(NodeTypeSection):
			if sectionTitles != nil {
				title, ok := sectionTitles[nodeID]
				if ok && title != "" {
					nodes[i]["label"] = title
				}
			}
		case string(NodeTypeCondition):
			if questionStore != nil {
				label := conditionLabelFromRule(ctx, nodes[i], questionStore)
				nodes[i]["label"] = label
			}
		}
		// START and END: leave label unchanged
	}

	enriched, err := json.Marshal(nodes)
	if err != nil {
		return workflowJSON, fmt.Errorf("%w: %w", internal.ErrMarshalWorkflow, err)
	}
	return enriched, nil
}

func conditionLabelFromRule(ctx context.Context, node map[string]interface{}, questionStore QuestionStore) string {
	fallback := "No label"
	label, ok := node["label"].(string)
	if ok {
		fallback = label
	}

	// If no condition rule, return fallback
	conditionRule, ok := node["conditionRule"].(map[string]interface{})
	if !ok {
		return fallback
	}

	// If no question, return fallback
	questionIDStr, ok := conditionRule["question"].(string)
	if !ok {
		return fallback
	}
	if questionIDStr == "" {
		return fallback
	}

	// Parse question ID
	questionID, err := uuid.Parse(questionIDStr)
	if err != nil {
		return fallback
	}

	// Parse pattern
	pattern, ok := conditionRule["pattern"].(string)
	if !ok {
		return fallback
	}

	// Get question
	answerable, err := questionStore.GetByID(ctx, questionID)
	if err != nil {
		return fallback
	}

	// Get question title
	q := answerable.Question()
	title := ""
	if q.Title.Valid {
		title = q.Title.String
	}
	if title == "" {
		title = questionIDStr
	}

	// If pattern is empty, return formatted label without pattern
	if pattern == "" {
		return fmt.Sprintf("When %s", title)
	}

	// If pattern is not empty, return formatted label with pattern
	return fmt.Sprintf("When %s matches %s", title, pattern)
}
