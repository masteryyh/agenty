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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/masteryyh/agenty/pkg/customerrors"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error {
	return e.err
}

func withExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{code: code, err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	var coded *exitCodeError
	if errors.As(err, &coded) {
		return coded.code
	}

	if businessErr := customerrors.GetBusinessError(err); businessErr != nil {
		switch businessErr.Code {
		case 400, 403, 404, 409:
			return 2
		}
	}

	return 1
}

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeTable(cmd *cobra.Command, headers []string, rows [][]string) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(writer, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeKeyValues(cmd *cobra.Command, rows [][2]string) error {
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeLine(cmd *cobra.Command, format string, args ...any) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), format+"\n", args...)
	return err
}

func writeActionResult(cmd *cobra.Command, value any, format string, args ...any) error {
	if outputJSON {
		return writeJSON(cmd, value)
	}
	if quietOutput {
		return nil
	}
	return writeLine(cmd, format, args...)
}

func isInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func confirmAction(cmd *cobra.Command, prompt string) (bool, error) {
	if !isInteractiveTerminal() {
		return false, nil
	}

	if _, err := fmt.Fprint(cmd.ErrOrStderr(), prompt); err != nil {
		return false, err
	}

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
