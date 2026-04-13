/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
"fmt"
"strings"

tea "github.com/charmbracelet/bubbletea"
"github.com/masteryyh/agenty/pkg/backend"
)

// maskWizKey returns a partially-masked display string for an API key.
func maskWizKey(key string) string {
r := []rune(key)
if len(r) <= 4 {
return strings.Repeat("*", len(r))
}
return string(r[:2]) + "****" + string(r[len(r)-2:])
}

// RunWizardTUI runs the first-time setup wizard as a full-screen TUI.
func RunWizardTUI(b backend.Backend) error {
providers, err := b.ListProviders(1, 100)
if err != nil {
return fmt.Errorf("failed to list providers: %w", err)
}

settings, _ := b.GetSystemSettings()

m := newWizardModel(b, providers.Data, settings)
p := tea.NewProgram(m, tea.WithAltScreen())
finalModel, err := p.Run()
if err != nil {
return err
}

if wm, ok := finalModel.(wizardModel); ok && wm.aborted {
return nil
}

return nil
}
