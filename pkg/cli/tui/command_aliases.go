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

package tui

import slcmd "github.com/masteryyh/agenty/pkg/cli/tui/command"

type ChatState = slcmd.ChatState
type CommandResult = slcmd.CommandResult
type Command = slcmd.Command
type ArgCompleter = slcmd.ArgCompleter
type ListAction = slcmd.ListAction
type ListResult = slcmd.ListResult

const (
	ListActionSelect = slcmd.ListActionSelect
	ListActionAdd    = slcmd.ListActionAdd
	ListActionEdit   = slcmd.ListActionEdit
	ListActionDelete = slcmd.ListActionDelete
	ListActionCancel = slcmd.ListActionCancel
)

var ErrCancelled = slcmd.ErrCancelled
var commands = slcmd.Commands()

func parseSlashInput(input string) []string {
	return slcmd.ParseSlashInput(input)
}

func commandHandler(name string) slcmd.CommandHandler {
	return slcmd.Handler(name)
}

func findCommand(name string, localMode bool) *Command {
	return slcmd.FindCommand(name, localMode)
}

func matchingCommands(input string, localMode bool) []Command {
	return slcmd.MatchingCommands(input, localMode)
}

func filterByPrefix(items []string, prefix string) []string {
	return slcmd.FilterByPrefix(items, prefix)
}

func commandVisible(cmd Command, localMode bool) bool {
	return slcmd.CommandVisible(cmd, localMode)
}
