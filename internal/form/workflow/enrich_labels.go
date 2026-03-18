package workflow

import (
	"NYCU-SDC/core-system-backend/internal"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// EnrichWorkflowResponse returns the given API-format workflow JSON with section and
// condition labels enriched (section title, condition rule). Uses questionStore.ListSections
// for section titles; if enrichment fails, returns the original workflow.
func (s *Service) EnrichWorkflowResponse(
	ctx context.Context,
	formID uuid.UUID,
	workflowJSON []byte,
) ([]byte, error) {
	if s.questionStore == nil {
		return workflowJSON, nil
	}

	// Get sections from question store
	sections, err := s.questionStore.ListSections(ctx, formID)
	if err != nil {
		return workflowJSON, err
	}

	// Convert sections to map of section IDs to section titles
	sectionTitles := make(map[string]string, len(sections))
	for id, sec := range sections {
		if !sec.Title.Valid {
			sectionTitles[id] = ""
			continue
		}
		sectionTitles[id] = sec.Title.String
	}

	var nodes []map[string]interface{}
	err = json.Unmarshal(workflowJSON, &nodes)
	if err != nil {
		return workflowJSON, fmt.Errorf("%w: %w", internal.ErrUnmarshalWorkflow, err)
	}

	for i := range nodes {
		typ, _ := nodes[i]["type"].(string)
		nodeID, _ := nodes[i]["id"].(string)

		switch strings.ToLower(typ) {
		case string(NodeTypeSection):
			title, ok := sectionTitles[nodeID]
			if ok && title != "" {
				nodes[i]["label"] = title
			}
		case string(NodeTypeCondition):
			label := conditionLabelFromRule(ctx, nodes[i], s.questionStore)
			nodes[i]["label"] = label
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
