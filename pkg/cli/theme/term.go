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

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const (
	seqDisableMouseAll = "\033[?1000l\033[?1002l\033[?1003l\033[?1006l"
	seqShowCursor      = "\033[?25h"
)

func RestoreTerminal() {
	fmt.Fprint(os.Stdout, seqDisableMouseAll+seqShowCursor)
	os.Stdout.Sync()
}

func DetectDarkBackground() bool {
	if v := os.Getenv("VSCODE_TERMINAL_THEME_KIND"); v != "" {
		return strings.EqualFold(v, "dark")
	}

	if v := os.Getenv("COLORFGBG"); v != "" {
		parts := strings.Split(v, ";")
		if len(parts) >= 2 {
			bg, err := strconv.Atoi(parts[len(parts)-1])
			if err == nil {
				return bg < 8
			}
		}
	}

	if runtime.GOOS == "darwin" {
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		if err == nil {
			return strings.TrimSpace(string(out)) == "Dark"
		}
		return false
	}

	return true
}
