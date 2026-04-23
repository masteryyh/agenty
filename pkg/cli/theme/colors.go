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

package theme

import "github.com/charmbracelet/lipgloss"

type Palette struct {
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Accent     lipgloss.Color
	Highlight  lipgloss.Color
	Special    lipgloss.Color
	Magenta    lipgloss.Color
	Success    lipgloss.Color
	Error      lipgloss.Color
	Warning    lipgloss.Color
	Text       lipgloss.Color
	TextMuted  lipgloss.Color
	TextSubtle lipgloss.Color
	TextFaint  lipgloss.Color
}

var darkPalette = Palette{
	Primary:    lipgloss.Color("43"),
	Secondary:  lipgloss.Color("39"),
	Accent:     lipgloss.Color("214"),
	Highlight:  lipgloss.Color("81"),
	Special:    lipgloss.Color("69"),
	Magenta:    lipgloss.Color("201"),
	Success:    lipgloss.Color("82"),
	Error:      lipgloss.Color("196"),
	Warning:    lipgloss.Color("214"),
	Text:       lipgloss.Color("252"),
	TextMuted:  lipgloss.Color("242"),
	TextSubtle: lipgloss.Color("238"),
	TextFaint:  lipgloss.Color("244"),
}

var lightPalette = Palette{
	Primary:    lipgloss.Color("30"),
	Secondary:  lipgloss.Color("25"),
	Accent:     lipgloss.Color("166"),
	Highlight:  lipgloss.Color("36"),
	Special:    lipgloss.Color("54"),
	Magenta:    lipgloss.Color("125"),
	Success:    lipgloss.Color("28"),
	Error:      lipgloss.Color("160"),
	Warning:    lipgloss.Color("130"),
	Text:       lipgloss.Color("235"),
	TextMuted:  lipgloss.Color("238"),
	TextSubtle: lipgloss.Color("244"),
	TextFaint:  lipgloss.Color("242"),
}

var Colors = darkPalette
var IsDark = true

func InitTheme(isDark bool) {
	IsDark = isDark
	if isDark {
		Colors = darkPalette
	} else {
		Colors = lightPalette
	}
	initStyles()
}
