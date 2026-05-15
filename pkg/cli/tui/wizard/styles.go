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

package wizard

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/masteryyh/agenty/pkg/cli/theme"
)

var (
	styleBold   lipgloss.Style
	styleCyan   lipgloss.Style
	styleWhite  lipgloss.Style
	styleGray   lipgloss.Style
	styleGreen  lipgloss.Style
	styleRed    lipgloss.Style
	styleYellow lipgloss.Style
)

func refreshStyles() {
	styleBold = theme.Bold
	styleCyan = theme.Cyan
	styleWhite = theme.White
	styleGray = theme.Muted
	styleGreen = theme.Green
	styleRed = theme.Red
	styleYellow = theme.Yellow
}
