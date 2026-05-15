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

import (
	"fmt"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/models"
)

type overlayKind int

const (
	overlayKindList overlayKind = iota
	overlayKindMultiSelect
	overlayKindHuhForm
	overlayKindSettingsEditor
	overlayKindLogViewer
)

type overlayRequest struct {
	kind              overlayKind
	title             string
	subtitle          string
	items             []string
	hints             string
	cursor            int
	listValidate      func(action ListAction, idx int) error
	listDeleteConfirm func(idx int) string
	options           []string
	defaultIndices    []int
	huhForm           *huh.Form
	formValidate      func() error
	backend           backend.Backend
	settings          *models.SystemSettingsDto
	responseCh        chan overlayResponse
}

type overlayResponse struct {
	listAction      ListAction
	listIndex       int
	formSubmitted   bool
	selectedIndices []int
	err             error
}

type overlayRequestMsg struct {
	request overlayRequest
}

type appendOutputMsg struct {
	text string
}

type clearChatMsg struct{}

type UIBridge struct {
	program   *tea.Program
	done      chan struct{}
	closeOnce sync.Once
}

func newUIBridge() *UIBridge {
	return &UIBridge{
		done: make(chan struct{}),
	}
}

func (b *UIBridge) Close() {
	b.closeOnce.Do(func() { close(b.done) })
}

func (b *UIBridge) ShowList(title string, items []string, hints string, subtitle ...string) (*ListResult, error) {
	return b.ShowListWithCursorAndActions(title, items, hints, 0, nil, nil, subtitle...)
}

func (b *UIBridge) ShowListWithCursor(title string, items []string, hints string, cursor int, subtitle ...string) (*ListResult, error) {
	return b.ShowListWithCursorAndActions(title, items, hints, cursor, nil, nil, subtitle...)
}

func (b *UIBridge) ShowListWithCursorAndValidate(title string, items []string, hints string, cursor int, validate func(action ListAction, idx int) error, subtitle ...string) (*ListResult, error) {
	return b.ShowListWithCursorAndActions(title, items, hints, cursor, validate, nil, subtitle...)
}

func (b *UIBridge) ShowListWithCursorAndActions(title string, items []string, hints string, cursor int, validate func(action ListAction, idx int) error, deleteConfirm func(idx int) string, subtitle ...string) (*ListResult, error) {
	sub := ""
	if len(subtitle) > 0 {
		sub = subtitle[0]
	}
	respCh := make(chan overlayResponse, 1)
	b.program.Send(overlayRequestMsg{
		request: overlayRequest{
			kind:              overlayKindList,
			title:             title,
			subtitle:          sub,
			items:             items,
			hints:             hints,
			cursor:            cursor,
			listValidate:      validate,
			listDeleteConfirm: deleteConfirm,
			responseCh:        respCh,
		},
	})
	select {
	case resp := <-respCh:
		return &ListResult{Action: resp.listAction, Index: resp.listIndex}, resp.err
	case <-b.done:
		return &ListResult{Action: ListActionCancel, Index: -1}, nil
	}
}

func (b *UIBridge) ShowHuhForm(form *huh.Form) (bool, error) {
	return b.ShowValidatedHuhForm(form, nil)
}

func (b *UIBridge) ShowValidatedHuhForm(form *huh.Form, validate func() error) (bool, error) {
	respCh := make(chan overlayResponse, 1)
	b.program.Send(overlayRequestMsg{
		request: overlayRequest{
			kind:         overlayKindHuhForm,
			huhForm:      form,
			formValidate: validate,
			responseCh:   respCh,
		},
	})
	select {
	case resp := <-respCh:
		return resp.formSubmitted, resp.err
	case <-b.done:
		return false, nil
	}
}

func (b *UIBridge) ShowSettingsEditor(backend backend.Backend, settings *models.SystemSettingsDto) error {
	respCh := make(chan overlayResponse, 1)
	b.program.Send(overlayRequestMsg{
		request: overlayRequest{
			kind:       overlayKindSettingsEditor,
			backend:    backend,
			settings:   settings,
			responseCh: respCh,
		},
	})
	select {
	case resp := <-respCh:
		return resp.err
	case <-b.done:
		return nil
	}
}

func (b *UIBridge) ShowConfirm(message string) (bool, error) {
	var result bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(message).Affirmative("Yes").Negative("No").Value(&result),
	))
	respCh := make(chan overlayResponse, 1)
	b.program.Send(overlayRequestMsg{
		request: overlayRequest{
			kind:       overlayKindHuhForm,
			huhForm:    form,
			responseCh: respCh,
		},
	})
	select {
	case resp := <-respCh:
		if !resp.formSubmitted {
			return false, nil
		}
		return result, nil
	case <-b.done:
		return false, nil
	}
}

func (b *UIBridge) ShowMultiSelect(title string, options []string, defaultIndices []int) ([]int, error) {
	respCh := make(chan overlayResponse, 1)
	b.program.Send(overlayRequestMsg{
		request: overlayRequest{
			kind:           overlayKindMultiSelect,
			title:          title,
			options:        options,
			defaultIndices: defaultIndices,
			responseCh:     respCh,
		},
	})
	select {
	case resp := <-respCh:
		return resp.selectedIndices, resp.err
	case <-b.done:
		return nil, nil
	}
}

func (b *UIBridge) ShowLogViewer() {
	respCh := make(chan overlayResponse, 1)
	b.program.Send(overlayRequestMsg{
		request: overlayRequest{
			kind:       overlayKindLogViewer,
			responseCh: respCh,
		},
	})
	select {
	case <-respCh:
	case <-b.done:
	}
}

func (b *UIBridge) Info(format string, args ...any) {
	b.program.Send(appendOutputMsg{text: renderStatusMessage("ℹ", fmt.Sprintf(format, args...))})
}

func (b *UIBridge) Warning(format string, args ...any) {
	b.program.Send(appendOutputMsg{text: renderStatusMessage("⚠", styleYellow.Render(fmt.Sprintf(format, args...)))})
}

func (b *UIBridge) Success(format string, args ...any) {
	b.program.Send(appendOutputMsg{text: renderStatusMessage("✓", styleGreen.Render(fmt.Sprintf(format, args...)))})
}

func (b *UIBridge) Error(format string, args ...any) {
	b.program.Send(appendOutputMsg{text: renderErrorMessage(fmt.Sprintf(format, args...))})
}

func (b *UIBridge) Print(text string) {
	b.program.Send(appendOutputMsg{text: text})
}

func (b *UIBridge) Println(text string) {
	b.program.Send(appendOutputMsg{text: text + "\n"})
}

func (b *UIBridge) Printf(format string, args ...any) {
	b.program.Send(appendOutputMsg{text: fmt.Sprintf(format, args...)})
}

func (b *UIBridge) PrintHistory(messages []models.ChatMessageDto) {
	b.Print(renderMessageHistoryToString(messages, false))
}

func (b *UIBridge) PrintCommandHints(localMode bool) {
	b.Print(renderCommandHintsToString(localMode))
}
