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
	PermissionMode  PermissionMode
}

type MetadataUpdate struct {
	XMLName         xml.Name `xml:"metadata"`
	Cwd             *string  `xml:"cwd,omitempty"`
	Model           *string  `xml:"model,omitempty"`
	Provider        *string  `xml:"provider,omitempty"`
	Timezone        *string  `xml:"timezone,omitempty"`
	ReasoningEffort *string  `xml:"reasoning-effort,omitempty"`
	PermissionMode  *string  `xml:"permission-mode,omitempty"`
}

func (metadata SessionMetadata) Diff(previous *SessionMetadata) MetadataUpdate {
	update := MetadataUpdate{}
	if previous == nil || metadata.Cwd != previous.Cwd {
		update.Cwd = new(metadata.Cwd)
	}
	if previous == nil || metadata.Model != previous.Model {
		update.Model = new(metadata.Model)
	}
	if previous == nil || metadata.Provider != previous.Provider {
		update.Provider = new(metadata.Provider)
	}
	if previous == nil || metadata.Timezone != previous.Timezone {
		update.Timezone = new(metadata.Timezone)
	}
	if previous == nil || metadata.ReasoningEffort != previous.ReasoningEffort {
		update.ReasoningEffort = new(metadata.ReasoningEffort)
	}
	if metadata.PermissionMode != "" && (previous == nil || metadata.PermissionMode != previous.PermissionMode) {
		update.PermissionMode = new(string(metadata.PermissionMode))
	}

	return update
}

func (update MetadataUpdate) Empty() bool {
	return update.Cwd == nil &&
		update.Model == nil &&
		update.Provider == nil &&
		update.Timezone == nil &&
		update.ReasoningEffort == nil &&
		update.PermissionMode == nil
}

func (update MetadataUpdate) XML() (string, error) {
	encoded, err := xml.MarshalIndent(update, "", "\t")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (metadata SessionMetadata) XML() (string, error) {
	var permissionMode *string
	if metadata.PermissionMode != "" {
		permissionMode = new(string(metadata.PermissionMode))
	}

	return MetadataUpdate{
		Cwd:             new(metadata.Cwd),
		Model:           new(metadata.Model),
		Provider:        new(metadata.Provider),
		Timezone:        new(metadata.Timezone),
		ReasoningEffort: new(metadata.ReasoningEffort),
		PermissionMode:  permissionMode,
	}.XML()
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
	if update.PermissionMode != nil {
		s.metadata.PermissionMode = PermissionMode(*update.PermissionMode).Normalized()
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
