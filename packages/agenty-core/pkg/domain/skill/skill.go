package skill

import "fmt"

// Entry is the metadata needed to advertise and resolve one installed skill.
type Entry struct {
	Name          string `json:"name"`
	DirectoryName string `json:"directoryName"`
	Description   string `json:"description"`
	Location      string `json:"location"`
	Source        string `json:"source"`
	AutoEnabled   bool   `json:"autoEnabled"`
	Warning       string `json:"warning,omitempty"`
}

type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Name     string `json:"name,omitempty"`
}

type Reference struct {
	Name string
	Path string
}

type Resolved struct {
	Entry   Entry
	Content string
}

func (e Entry) ValidateReference(name, path string) error {
	if e.Name != name {
		return fmt.Errorf("skill reference name %q does not match registry entry %q", name, e.Name)
	}
	if e.Location != path {
		return fmt.Errorf("skill reference path %q does not match registry location %q", path, e.Location)
	}
	return nil
}
