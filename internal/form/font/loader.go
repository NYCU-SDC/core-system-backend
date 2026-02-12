package font

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

type Font struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

//go:embed fonts.json
var fontsJSON []byte

var (
	once     sync.Once
	loadErr  error
	cached   []Font
	idToName map[string]string
)

func loadOnce() {
	var fonts []Font

	dec := json.NewDecoder(bytes.NewReader(fontsJSON))
	dec.DisallowUnknownFields()

	err := dec.Decode(&fonts)
	if err != nil {
		loadErr = fmt.Errorf("font: failed to decode fonts.json: %w", err)
		return
	}

	m := make(map[string]string, len(fonts))
	for i, f := range fonts {
		if f.ID == "" || f.Name == "" {
			loadErr = fmt.Errorf("font: invalid font entry at index %d (empty id/name)", i)
			return
		}
		if _, exists := m[f.ID]; exists {
			loadErr = fmt.Errorf("font: duplicated font id: %s", f.ID)
			return
		}
		m[f.ID] = f.Name
	}

	cached = fonts
	idToName = m
}

func List() ([]Font, error) {
	once.Do(loadOnce)
	if loadErr != nil {
		return nil, loadErr
	}
	return cached, nil
}

func Map() (map[string]string, error) {
	once.Do(loadOnce)
	if loadErr != nil {
		return nil, loadErr
	}
	return idToName, nil
}

func IsValidID(id string) (bool, error) {
	m, err := Map()
	if err != nil {
		return false, err
	}

	_, ok := m[id]
	return ok, nil
}

func NameByID(id string) (string, bool, error) {
	m, err := Map()
	if err != nil {
		return "", false, err
	}

	name, ok := m[id]
	return name, ok, nil
}
