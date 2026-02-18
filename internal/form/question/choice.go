package question

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"NYCU-SDC/core-system-backend/internal/form/shared"

	"github.com/google/uuid"
)

type ChoiceOption struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type Choice struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type SingleChoice struct {
	question Question
	formID   uuid.UUID
	Choices  []Choice
}

func (s SingleChoice) Question() Question {
	return s.question
}

func (s SingleChoice) FormID() uuid.UUID {
	return s.formID
}

func (s SingleChoice) Validate(rawValue json.RawMessage) error {
	var choiceIDs []string
	err := json.Unmarshal(rawValue, &choiceIDs)
	if err != nil {
		return fmt.Errorf("invalid single choice value format: %w", err)
	}

	if len(choiceIDs) == 0 {
		return nil // No value means no selection
	}

	if len(choiceIDs) > 1 {
		return fmt.Errorf("single choice cannot have multiple selections")
	}

	value := choiceIDs[0]
	for _, choice := range s.Choices {
		if choice.ID.String() == value {
			return nil
		}
	}

	return ErrInvalidChoiceID{
		QuestionID: s.question.ID.String(),
		ChoiceID:   value,
	}
}

func NewSingleChoice(q Question, formID uuid.UUID) (SingleChoice, error) {
	choices, err := validateAndExtractChoices(q, true)
	if err != nil {
		return SingleChoice{}, err
	}

	return SingleChoice{
		question: q,
		formID:   formID,
		Choices:  choices,
	}, nil
}

func (s SingleChoice) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends string[] with single element for single choice
	_, selectedChoices, err := decodeMultipleChoiceIDs(rawValue, s.Choices, s.question.ID.String(), 1)
	if err != nil {
		return nil, err
	}

	if len(selectedChoices) > 1 {
		return nil, errors.New("single choice cannot have multiple selections")
	}

	selectedChoice := selectedChoices[0]
	return shared.SingleChoiceAnswer{
		ChoiceID: selectedChoice.ID,
		Snapshot: shared.ChoiceSnapshot{
			Name:        selectedChoice.Name,
			Description: selectedChoice.Description,
		},
	}, nil
}

func (s SingleChoice) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.SingleChoiceAnswer
	err := json.Unmarshal(rawValue, &answer)
	if err != nil {
		return nil, fmt.Errorf("invalid single choice answer in storage: %w", err)
	}

	return answer, nil
}

func (s SingleChoice) EncodeRequest(answer any) (json.RawMessage, error) {
	singleChoiceAnswer, ok := answer.(shared.SingleChoiceAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.SingleChoiceAnswer, got %T", answer)
	}

	// API expects string[] with single element
	choiceIDs := []uuid.UUID{singleChoiceAnswer.ChoiceID}
	return encodeChoiceIDsToRequest(choiceIDs)
}

func (s SingleChoice) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := s.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	singleChoiceAnswer, ok := answer.(shared.SingleChoiceAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.SingleChoiceAnswer, got %T", answer)
	}

	return singleChoiceAnswer.Snapshot.Name, nil
}

type MultiChoice struct {
	question Question
	formID   uuid.UUID
	Choices  []Choice
}

func (m MultiChoice) Question() Question {
	return m.question
}

func (m MultiChoice) FormID() uuid.UUID {
	return m.formID
}

func (m MultiChoice) Validate(rawValue json.RawMessage) error {
	var choiceIDs []string
	err := json.Unmarshal(rawValue, &choiceIDs)
	if err != nil {
		return fmt.Errorf("invalid multiple choice value format: %w", err)
	}

	if len(choiceIDs) == 0 {
		return nil // No value means no selection
	}

	for _, idStr := range choiceIDs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}

		valid := false
		for _, choice := range m.Choices {
			if choice.ID.String() == idStr {
				valid = true
				break
			}
		}

		if !valid {
			return ErrInvalidChoiceID{
				QuestionID: m.question.ID.String(),
				ChoiceID:   idStr,
			}
		}
	}

	return nil
}

func NewMultiChoice(q Question, formID uuid.UUID) (MultiChoice, error) {
	choices, err := validateAndExtractChoices(q, true)
	if err != nil {
		return MultiChoice{}, err
	}

	return MultiChoice{
		question: q,
		formID:   formID,
		Choices:  choices,
	}, nil
}

func (m MultiChoice) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends string[] for multiple choice
	_, selectedChoices, err := decodeMultipleChoiceIDs(rawValue, m.Choices, m.question.ID.String(), 1)
	if err != nil {
		return nil, err
	}

	// Build answer with snapshots
	answer := shared.MultipleChoiceAnswer{
		Choices: make([]struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}, 0, len(selectedChoices)),
	}

	for _, selectedChoice := range selectedChoices {
		answer.Choices = append(answer.Choices, struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}{
			ChoiceID: selectedChoice.ID,
			Snapshot: shared.ChoiceSnapshot{
				Name:        selectedChoice.Name,
				Description: selectedChoice.Description,
			},
		})
	}

	return answer, nil
}

func (m MultiChoice) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.MultipleChoiceAnswer
	err := json.Unmarshal(rawValue, &answer)
	if err != nil {
		return nil, fmt.Errorf("invalid multiple choice answer in storage: %w", err)
	}

	return answer, nil
}

func (m MultiChoice) EncodeRequest(answer any) (json.RawMessage, error) {
	multiChoiceAnswer, ok := answer.(shared.MultipleChoiceAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.MultipleChoiceAnswer, got %T", answer)
	}

	// API expects string[] of choice IDs
	choiceIDs := make([]uuid.UUID, len(multiChoiceAnswer.Choices))
	for i, choice := range multiChoiceAnswer.Choices {
		choiceIDs[i] = choice.ChoiceID
	}
	return encodeChoiceIDsToRequest(choiceIDs)
}

func (m MultiChoice) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := m.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	multiChoiceAnswer, ok := answer.(shared.MultipleChoiceAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.MultipleChoiceAnswer, got %T", answer)
	}

	if len(multiChoiceAnswer.Choices) == 0 {
		return "", nil
	}

	names := make([]string, len(multiChoiceAnswer.Choices))
	for i, choice := range multiChoiceAnswer.Choices {
		names[i] = choice.Snapshot.Name
	}
	return strings.Join(names, ", "), nil
}

type DetailedMultiChoice struct {
	question Question
	formID   uuid.UUID
	Choices  []Choice
}

func (m DetailedMultiChoice) Question() Question {
	return m.question
}

func (m DetailedMultiChoice) FormID() uuid.UUID {
	return m.formID
}

func (m DetailedMultiChoice) Validate(rawValue json.RawMessage) error {
	var choiceIDs []string
	err := json.Unmarshal(rawValue, &choiceIDs)
	if err != nil {
		return fmt.Errorf("invalid detailed multiple choice value format: %w", err)
	}

	if len(choiceIDs) == 0 {
		return nil // No value means no selection
	}

	for _, idStr := range choiceIDs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}

		valid := false
		for _, choice := range m.Choices {
			if choice.ID.String() == idStr {
				valid = true
				break
			}
		}

		if !valid {
			return ErrInvalidChoiceID{
				QuestionID: m.question.ID.String(),
				ChoiceID:   idStr,
			}
		}
	}

	return nil
}

func NewDetailedMultiChoice(q Question, formID uuid.UUID) (DetailedMultiChoice, error) {
	choices, err := validateAndExtractChoices(q, false)
	if err != nil {
		return DetailedMultiChoice{}, err
	}

	return DetailedMultiChoice{
		question: q,
		formID:   formID,
		Choices:  choices,
	}, nil
}

func (m DetailedMultiChoice) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends string[] for detailed multiple choice
	_, selectedChoices, err := decodeMultipleChoiceIDs(rawValue, m.Choices, m.question.ID.String(), 1)
	if err != nil {
		return nil, err
	}

	// Build answer with snapshots
	answer := shared.DetailedMultipleChoiceAnswer{
		Choices: make([]struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}, 0, len(selectedChoices)),
	}

	for _, selectedChoice := range selectedChoices {
		answer.Choices = append(answer.Choices, struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
		}{
			ChoiceID: selectedChoice.ID,
			Snapshot: shared.ChoiceSnapshot{
				Name:        selectedChoice.Name,
				Description: selectedChoice.Description,
			},
		})
	}

	return answer, nil
}

func (m DetailedMultiChoice) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.DetailedMultipleChoiceAnswer
	err := json.Unmarshal(rawValue, &answer)
	if err != nil {
		return nil, fmt.Errorf("invalid detailed multiple choice answer in storage: %w", err)
	}

	return answer, nil
}

func (m DetailedMultiChoice) EncodeRequest(answer any) (json.RawMessage, error) {
	detailedMultiChoiceAnswer, ok := answer.(shared.DetailedMultipleChoiceAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.DetailedMultipleChoiceAnswer, got %T", answer)
	}

	// API expects string[] of choice IDs
	choiceIDs := make([]string, len(detailedMultiChoiceAnswer.Choices))
	for i, choice := range detailedMultiChoiceAnswer.Choices {
		choiceIDs[i] = choice.ChoiceID.String()
	}
	return json.Marshal(choiceIDs)
}

func (m DetailedMultiChoice) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := m.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	detailedMultiChoiceAnswer, ok := answer.(shared.DetailedMultipleChoiceAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.DetailedMultipleChoiceAnswer, got %T", answer)
	}

	if len(detailedMultiChoiceAnswer.Choices) == 0 {
		return "", nil
	}

	names := make([]string, len(detailedMultiChoiceAnswer.Choices))
	for i, choice := range detailedMultiChoiceAnswer.Choices {
		names[i] = choice.Snapshot.Name
	}
	return strings.Join(names, ", "), nil
}

type Ranking struct {
	question Question
	formID   uuid.UUID
	Rank     []Choice
}

func (r Ranking) Question() Question {
	return r.question
}

func (r Ranking) FormID() uuid.UUID {
	return r.formID
}

func (r Ranking) Validate(rawValue json.RawMessage) error {
	var choiceIDs []string
	err := json.Unmarshal(rawValue, &choiceIDs)
	if err != nil {
		return fmt.Errorf("invalid ranking value format: %w", err)
	}

	for _, idStr := range choiceIDs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}

		valid := false
		for _, choice := range r.Rank {
			if choice.ID.String() == idStr {
				valid = true
				break
			}
		}

		if !valid {
			return ErrInvalidChoiceID{
				QuestionID: r.question.ID.String(),
				ChoiceID:   idStr,
			}
		}
	}

	return nil
}

func NewRanking(q Question, formID uuid.UUID) (Ranking, error) {
	choices, err := validateAndExtractChoices(q, true)
	if err != nil {
		return Ranking{}, err
	}

	return Ranking{
		question: q,
		formID:   formID,
		Rank:     choices,
	}, nil
}

func (r Ranking) DecodeRequest(rawValue json.RawMessage) (any, error) {
	// API sends string[] for ranking (ordered by rank)
	var choiceIDs []string
	err := json.Unmarshal(rawValue, &choiceIDs)
	if err != nil {
		return nil, fmt.Errorf("invalid ranking value format: %w", err)
	}

	if len(choiceIDs) == 0 {
		return nil, errors.New("ranking requires at least one choice ID")
	}

	// Build answer with snapshots and rank
	answer := shared.RankingAnswer{
		RankedChoices: make([]struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
			Rank     int                   `json:"rank"`
		}, 0, len(choiceIDs)),
	}

	for rank, idStr := range choiceIDs {
		choiceID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid choice ID format: %w", err)
		}

		// Find the choice in metadata to create snapshot
		var selectedChoice *Choice
		for _, choice := range r.Rank {
			if choice.ID == choiceID {
				selectedChoice = &choice
				break
			}
		}

		if selectedChoice == nil {
			return nil, ErrInvalidChoiceID{
				QuestionID: r.question.ID.String(),
				ChoiceID:   choiceID.String(),
			}
		}

		answer.RankedChoices = append(answer.RankedChoices, struct {
			ChoiceID uuid.UUID             `json:"choiceId"`
			Snapshot shared.ChoiceSnapshot `json:"snapshot"`
			Rank     int                   `json:"rank"`
		}{
			ChoiceID: choiceID,
			Snapshot: shared.ChoiceSnapshot{
				Name:        selectedChoice.Name,
				Description: selectedChoice.Description,
			},
			Rank: rank + 1, // 1-based ranking
		})
	}

	return answer, nil
}

func (r Ranking) DecodeStorage(rawValue json.RawMessage) (any, error) {
	var answer shared.RankingAnswer
	err := json.Unmarshal(rawValue, &answer)
	if err != nil {
		return nil, fmt.Errorf("invalid ranking answer in storage: %w", err)
	}

	return answer, nil
}

func (r Ranking) EncodeRequest(answer any) (json.RawMessage, error) {
	rankingAnswer, ok := answer.(shared.RankingAnswer)
	if !ok {
		return nil, fmt.Errorf("expected shared.RankingAnswer, got %T", answer)
	}

	// API expects string[] of choice IDs in rank order
	// Sort by rank first to ensure correct order
	type rankedChoice struct {
		choiceID string
		rank     int
	}

	rankedChoices := make([]rankedChoice, len(rankingAnswer.RankedChoices))
	for i, choice := range rankingAnswer.RankedChoices {
		rankedChoices[i] = rankedChoice{
			choiceID: choice.ChoiceID.String(),
			rank:     choice.Rank,
		}
	}

	// Sort by rank
	for i := 0; i < len(rankedChoices)-1; i++ {
		for j := i + 1; j < len(rankedChoices); j++ {
			if rankedChoices[i].rank > rankedChoices[j].rank {
				rankedChoices[i], rankedChoices[j] = rankedChoices[j], rankedChoices[i]
			}
		}
	}

	// Extract choice IDs in order
	choiceIDs := make([]string, len(rankedChoices))
	for i, choice := range rankedChoices {
		choiceIDs[i] = choice.choiceID
	}

	return json.Marshal(choiceIDs)
}

func (r Ranking) DisplayValue(rawValue json.RawMessage) (string, error) {
	answer, err := r.DecodeStorage(rawValue)
	if err != nil {
		return "", err
	}

	rankingAnswer, ok := answer.(shared.RankingAnswer)
	if !ok {
		return "", fmt.Errorf("expected shared.RankingAnswer, got %T", answer)
	}

	if len(rankingAnswer.RankedChoices) == 0 {
		return "", nil
	}

	// Sort by rank
	type sortedChoice struct {
		name string
		rank int
	}

	sorted := make([]sortedChoice, len(rankingAnswer.RankedChoices))
	for i, choice := range rankingAnswer.RankedChoices {
		sorted[i] = sortedChoice{
			name: choice.Snapshot.Name,
			rank: choice.Rank,
		}
	}

	// Sort by rank
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].rank > sorted[j].rank {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	names := make([]string, len(sorted))
	for i, choice := range sorted {
		names[i] = choice.name
	}
	return strings.Join(names, " > "), nil
}

// GenerateChoiceMetadata creates and validates metadata JSON for choice-based questions
func GenerateChoiceMetadata(questionType string, choiceOptions []ChoiceOption) ([]byte, error) {
	// For choice questions, require at least one choice
	if len(choiceOptions) == 0 {
		return nil, ErrMetadataValidate{
			QuestionID: questionType,
			RawData:    []byte(fmt.Sprintf("%v", choiceOptions)),
			Message:    "no choices provided for choice question",
		}
	}

	// Generate choices with UUIDs
	choices := make([]Choice, len(choiceOptions))
	for i, option := range choiceOptions {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			return nil, ErrMetadataValidate{
				QuestionID: questionType,
				RawData:    []byte(fmt.Sprintf("%v", choiceOptions)),
				Message:    "choice name cannot be empty",
			}
		}
		choices[i] = Choice{
			ID:          uuid.New(),
			Name:        name,
			Description: strings.TrimSpace(option.Description),
		}
	}

	if questionType == "detailed_multiple_choice" {
		hasDescription := false
		for _, choice := range choices {
			if strings.TrimSpace(choice.Description) != "" {
				hasDescription = true
				break
			}
		}
		if !hasDescription {
			return nil, ErrMetadataValidate{
				QuestionID: questionType,
				RawData:    []byte(fmt.Sprintf("%v", choiceOptions)),
				Message:    "detailed multiple choice requires at least one choice with description",
			}
		}
	}

	metadata := map[string]any{
		"choice": choices,
	}

	return json.Marshal(metadata)
}

func ExtractChoices(data []byte) ([]Choice, error) {
	var partial map[string]json.RawMessage
	err := json.Unmarshal(data, &partial)
	if err != nil {
		return nil, fmt.Errorf("could not parse partial json: %w", err)
	}

	var choices []Choice
	if raw, ok := partial["choice"]; ok {
		err := json.Unmarshal(raw, &choices)
		if err != nil {
			return nil, fmt.Errorf("could not parse choices: %w", err)
		}
	}

	return choices, nil
}

// validateAndExtractChoices is a helper function that validates and extracts choices from metadata
// It returns an error if metadata is invalid or choices are malformed
func validateAndExtractChoices(q Question, allowSourceWithNil bool) ([]Choice, error) {
	metadata := q.Metadata

	// Allow empty metadata for questions with SourceID
	if allowSourceWithNil && q.SourceID.Valid && metadata == nil {
		return []Choice{}, nil
	}

	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}

	choices, err := ExtractChoices(metadata)
	if err != nil {
		return nil, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "could not extract choices from metadata",
		}
	}

	if len(choices) == 0 {
		return nil, ErrMetadataBroken{
			QuestionID: q.ID.String(),
			RawData:    metadata,
			Message:    "no choices found in metadata",
		}
	}

	for _, choice := range choices {
		if choice.ID == uuid.Nil {
			return nil, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice ID cannot be nil",
			}
		}

		if strings.TrimSpace(choice.Name) == "" {
			return nil, ErrMetadataBroken{
				QuestionID: q.ID.String(),
				RawData:    metadata,
				Message:    "choice name cannot be empty",
			}
		}
	}

	return choices, nil
}

// findChoiceByID searches for a choice by ID and returns a pointer to it
func findChoiceByID(choices []Choice, choiceID uuid.UUID) *Choice {
	for _, choice := range choices {
		if choice.ID == choiceID {
			return &choice
		}
	}
	return nil
}

// decodeMultipleChoiceIDs is a helper function to decode and validate multiple choice IDs from API request
func decodeMultipleChoiceIDs(rawValue json.RawMessage, choices []Choice, questionID string, minChoices int) ([]uuid.UUID, []*Choice, error) {
	var choiceIDs []string
	err := json.Unmarshal(rawValue, &choiceIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid choice value format: %w", err)
	}

	if len(choiceIDs) < minChoices {
		return nil, nil, fmt.Errorf("requires at least %d choice ID(s)", minChoices)
	}

	parsedIDs := make([]uuid.UUID, 0, len(choiceIDs))
	selectedChoices := make([]*Choice, 0, len(choiceIDs))

	for _, idStr := range choiceIDs {
		choiceID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid choice ID format: %w", err)
		}

		selectedChoice := findChoiceByID(choices, choiceID)
		if selectedChoice == nil {
			return nil, nil, ErrInvalidChoiceID{
				QuestionID: questionID,
				ChoiceID:   choiceID.String(),
			}
		}

		parsedIDs = append(parsedIDs, choiceID)
		selectedChoices = append(selectedChoices, selectedChoice)
	}

	return parsedIDs, selectedChoices, nil
}

// encodeChoiceIDsToRequest converts choice IDs to the API request format (string[])
func encodeChoiceIDsToRequest(choiceIDs []uuid.UUID) (json.RawMessage, error) {
	strIDs := make([]string, len(choiceIDs))
	for i, id := range choiceIDs {
		strIDs[i] = id.String()
	}
	return json.Marshal(strIDs)
}
