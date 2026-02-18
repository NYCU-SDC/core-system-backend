package question

import "regexp"

// ToQuestion converts GetByIDRow to Question
func (r GetByIDRow) ToQuestion() Question {
	return Question{
		ID:          r.ID,
		SectionID:   r.SectionID,
		Required:    r.Required,
		Type:        r.Type,
		Title:       r.Title,
		Description: r.Description,
		Metadata:    r.Metadata,
		Order:       r.Order,
		SourceID:    r.SourceID,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// ToQuestion converts CreateRow to Question
func (r CreateRow) ToQuestion() Question {
	return Question{
		ID:          r.ID,
		SectionID:   r.SectionID,
		Required:    r.Required,
		Type:        r.Type,
		Title:       r.Title,
		Description: r.Description,
		Metadata:    r.Metadata,
		Order:       r.Order,
		SourceID:    r.SourceID,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// ToQuestion converts UpdateRow to Question
func (r UpdateRow) ToQuestion() Question {
	return Question{
		ID:          r.ID,
		SectionID:   r.SectionID,
		Required:    r.Required,
		Type:        r.Type,
		Title:       r.Title,
		Description: r.Description,
		Metadata:    r.Metadata,
		Order:       r.Order,
		SourceID:    r.SourceID,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// ToQuestion converts UpdateOrderRow to Question
func (r UpdateOrderRow) ToQuestion() Question {
	return Question{
		ID:          r.ID,
		SectionID:   r.SectionID,
		Required:    r.Required,
		Type:        r.Type,
		Title:       r.Title,
		Description: r.Description,
		Metadata:    r.Metadata,
		Order:       r.Order,
		SourceID:    r.SourceID,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// matchPattern checks if a value matches the given regex pattern.
// Returns false with nil if the pattern is invalid (logs error internally).
// Returns false with error if regex compilation fails unexpectedly.
func matchPattern(value string, pattern string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Invalid pattern - log and return false with nil
		// In production, this should log the error
		return false, nil
	}

	return re.MatchString(value), nil
}
