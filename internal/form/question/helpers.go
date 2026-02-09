package question

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
