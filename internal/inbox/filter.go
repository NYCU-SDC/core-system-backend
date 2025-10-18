package inbox

import (
	"NYCU-SDC/core-system-backend/internal"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	MaxSearchLength = 255
)

// InboxFilterRequest represents the filter parameters for inbox messages
type InboxFilterRequest struct {
	IsRead     *bool  `json:"isRead,omitempty"`
	IsStarred  *bool  `json:"isStarred,omitempty"`
	IsArchived *bool  `json:"isArchived,omitempty"`
	Search     string `json:"search,omitempty"`
}

// ParseFilterRequest parses filter parameters from HTTP request query parameters
func ParseFilterRequest(r *http.Request) (*InboxFilterRequest, error) {
	query := r.URL.Query()
	filter := &InboxFilterRequest{}

	// Parse isRead parameter
	isReadStr := query.Get("isRead")
	if isReadStr != "" {
		isRead, err := strconv.ParseBool(isReadStr)
		if err != nil {
			return nil, err
		}
		filter.IsRead = &isRead
	}

	// Parse isStarred parameter
	isStarredStr := query.Get("isStarred")
	if isStarredStr != "" {
		isStarred, err := strconv.ParseBool(isStarredStr)
		if err != nil {
			return nil, err
		}
		filter.IsStarred = &isStarred
	}

	// Parse isArchived parameter
	isArchivedStr := query.Get("isArchived")
	if isArchivedStr != "" {
		isArchived, err := strconv.ParseBool(isArchivedStr)
		if err != nil {
			return nil, err
		}
		filter.IsArchived = &isArchived
	}

	// Parse search parameter
	filter.Search = strings.TrimSpace(query.Get("search"))

	// Validate search length
	if len(filter.Search) > MaxSearchLength {
		return nil, internal.ErrSearchTooLong
	}

	return filter, nil
}

// ToQueryParams converts the filter request to URL query parameters
func (f *InboxFilterRequest) ToQueryParams() url.Values {
	params := url.Values{}

	if f.IsRead != nil {
		params.Set("isRead", strconv.FormatBool(*f.IsRead))
	}

	if f.IsStarred != nil {
		params.Set("isStarred", strconv.FormatBool(*f.IsStarred))
	}

	if f.IsArchived != nil {
		params.Set("isArchived", strconv.FormatBool(*f.IsArchived))
	}

	if f.Search != "" {
		params.Set("search", f.Search)
	}

	return params
}
