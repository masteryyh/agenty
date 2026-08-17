package conversation

import (
	"encoding/xml"
)

type SessionMetadata struct {
	Cwd             string
	Model           string
	Provider        string
	Timezone        string
	ReasoningEffort string
}

type MetadataUpdate struct {
	XMLName         xml.Name `xml:"metadata"`
	Cwd             *string  `xml:"cwd,omitempty"`
	Model           *string  `xml:"model,omitempty"`
	Provider        *string  `xml:"provider,omitempty"`
	Timezone        *string  `xml:"timezone,omitempty"`
	ReasoningEffort *string  `xml:"reasoning-effort,omitempty"`
}

func (metadata SessionMetadata) Diff(previous *SessionMetadata) MetadataUpdate {
	update := MetadataUpdate{}
	if previous == nil || metadata.Cwd != previous.Cwd {
		update.Cwd = metadataStringPointer(metadata.Cwd)
	}
	if previous == nil || metadata.Model != previous.Model {
		update.Model = metadataStringPointer(metadata.Model)
	}
	if previous == nil || metadata.Provider != previous.Provider {
		update.Provider = metadataStringPointer(metadata.Provider)
	}
	if previous == nil || metadata.Timezone != previous.Timezone {
		update.Timezone = metadataStringPointer(metadata.Timezone)
	}
	if previous == nil || metadata.ReasoningEffort != previous.ReasoningEffort {
		update.ReasoningEffort = metadataStringPointer(metadata.ReasoningEffort)
	}

	return update
}

func (update MetadataUpdate) Empty() bool {
	return update.Cwd == nil &&
		update.Model == nil &&
		update.Provider == nil &&
		update.Timezone == nil &&
		update.ReasoningEffort == nil
}

func (update MetadataUpdate) XML() (string, error) {
	encoded, err := xml.MarshalIndent(update, "", "\t")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *Session) LastMetadata() *SessionMetadata {
	if s.metadata == nil {
		return nil
	}

	copy := *s.metadata
	return &copy
}

func (s *Session) applyMessageMetadata(message Message) {
	update, ok := parseMetadataMessage(message)
	if !ok {
		return
	}
	if s.metadata == nil {
		s.metadata = &SessionMetadata{}
	}

	if update.Cwd != nil {
		s.metadata.Cwd = *update.Cwd
	}
	if update.Model != nil {
		s.metadata.Model = *update.Model
	}
	if update.Provider != nil {
		s.metadata.Provider = *update.Provider
	}
	if update.Timezone != nil {
		s.metadata.Timezone = *update.Timezone
	}
	if update.ReasoningEffort != nil {
		s.metadata.ReasoningEffort = *update.ReasoningEffort
	}
}

func parseMetadataMessage(message Message) (MetadataUpdate, bool) {
	if !message.IsHidden() || len(message.Content) != 1 {
		return MetadataUpdate{}, false
	}

	textBlock, ok := message.Content[0].(TextBlock)
	if !ok {
		return MetadataUpdate{}, false
	}

	var update MetadataUpdate
	if err := xml.Unmarshal([]byte(textBlock.Text), &update); err != nil {
		return MetadataUpdate{}, false
	}
	return update, true
}

func metadataStringPointer(value string) *string {
	return &value
}
