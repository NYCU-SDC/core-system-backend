package workflow

import (
	"encoding/json"
	"fmt"
)

// nodeStructure holds the structural fields of a workflow node used for equality.
// Label and other display-only fields are ignored.
type nodeStructure struct {
	ID            string
	Type          string
	Next          string
	NextTrue      string
	NextFalse     string
	ConditionRule conditionRuleStructure
}

type conditionRuleStructure struct {
	Source         string
	Question       string
	Pattern        string
	ChoiceOptionID string
}

// structurallyEqual reports whether two workflow JSON payloads are
// structurally equal: same node set (by id), same type, edges (next/nextTrue/
// nextFalse), and conditionRule (source, question, pattern, choiceOptionId).
// Label and other display-only fields are ignored so that question/section
// changes that only affect labels do not count as workflow changes.
func structurallyEqual(current, incoming []byte) (bool, error) {
	currentNodes, err := parseForCompare(current)
	if err != nil {
		return false, fmt.Errorf("current workflow: %w", err)
	}
	incomingNodes, err := parseForCompare(incoming)
	if err != nil {
		return false, fmt.Errorf("incoming workflow: %w", err)
	}

	currentStruct := buildNodeStructureMap(currentNodes)
	incomingStruct := buildNodeStructureMap(incomingNodes)

	if len(currentStruct) != len(incomingStruct) {
		return false, nil
	}
	for id, curr := range currentStruct {
		inc, ok := incomingStruct[id]
		if !ok {
			return false, nil
		}
		if !nodeStructureEqual(curr, inc) {
			return false, nil
		}
	}
	return true, nil
}

func parseForCompare(workflow []byte) ([]map[string]interface{}, error) {
	var nodes []map[string]interface{}
	err := json.Unmarshal(workflow, &nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}
	return nodes, nil
}

func buildNodeStructureMap(nodes []map[string]interface{}) map[string]nodeStructure {
	out := make(map[string]nodeStructure)
	for _, node := range nodes {
		id, _ := node["id"].(string)
		if id == "" {
			continue
		}

		typ, ok := node["type"].(string)
		if !ok {
			continue
		}

		// Edge fields are optional; use empty string when absent.
		next, _ := node["next"].(string)
		nextTrue, _ := node["nextTrue"].(string)
		nextFalse, _ := node["nextFalse"].(string)

		rule := conditionRuleStructure{}
		cr, ok := node["conditionRule"].(map[string]interface{})
		if ok {
			rule.Source = strVal(cr, "source")
			rule.Question = strVal(cr, "question")
			rule.Pattern = strVal(cr, "pattern")
			rule.ChoiceOptionID = strVal(cr, "choiceOptionId")
		}

		out[id] = nodeStructure{
			ID:            id,
			Type:          typ,
			Next:          next,
			NextTrue:      nextTrue,
			NextFalse:     nextFalse,
			ConditionRule: rule,
		}
	}
	return out
}

func strVal(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}

	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func nodeStructureEqual(a, b nodeStructure) bool {
	if a.ID != b.ID || a.Type != b.Type ||
		a.Next != b.Next || a.NextTrue != b.NextTrue || a.NextFalse != b.NextFalse {
		return false
	}
	crA, crB := a.ConditionRule, b.ConditionRule
	return crA.Source == crB.Source &&
		crA.Question == crB.Question &&
		crA.Pattern == crB.Pattern &&
		crA.ChoiceOptionID == crB.ChoiceOptionID
}
