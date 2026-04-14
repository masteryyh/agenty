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
	"math/rand/v2"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/masteryyh/agenty/pkg/backend"
	"github.com/masteryyh/agenty/pkg/chat/provider"
	"github.com/masteryyh/agenty/pkg/cli/theme"
	"github.com/masteryyh/agenty/pkg/consts"
	"github.com/masteryyh/agenty/pkg/models"
	"github.com/masteryyh/agenty/pkg/utils/signal"
)

type appMode int

const (
	modeChat appMode = iota
	modeOverlay
)

type streamEventMsg struct {
	event provider.StreamEvent
}

type streamDoneMsg struct {
	err error
}

type commandDoneMsg struct {
	result CommandResult
	err    error
}

type tokenCountMsg struct {
	count int
}

type refreshSessionMsg struct {
	tokenConsumed int
	messages      []models.ChatMessageDto
}

type argCompletionMsg struct {
	cmdName string
	args    []string
	prefix  string
}

type completionMode int

const (
	completeCmdMode completionMode = iota
	completeArgMode
)

type spinTickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg {
		return spinTickMsg(t)
	})
}

type chatModel struct {
	backend   backend.Backend
	bridge    *UIBridge
	sessionID uuid.UUID
	modelID   uuid.UUID
	agentID   uuid.UUID
	modelName string
	agentName string
	chatState *ChatState

	mode    appMode
	overlay any
	overlayRespCh chan overlayResponse

	viewport viewport.Model
	input    textarea.Model

	bannerContent string
	chatLog       *strings.Builder
	outputLog     *strings.Builder

	tokenConsumed int

	showReasoning bool
	lastMessages  []models.ChatMessageDto

	stream     streamModel
	completion completionModel

	pendingHistory bool

	huhFormWidth int

	width  int
	height int
	ready  bool
}

const maxInputLines = 5

func newChatModel(b backend.Backend, bridge *UIBridge, sessionID uuid.UUID, modelID uuid.UUID, agentID uuid.UUID, modelName, agentName string, messages []models.ChatMessageDto) chatModel {
	ta := textarea.New()
	ta.Prompt = "  "
	ta.Placeholder = "Type a message or /help for commands..."
	ta.CharLimit = 8192
	ta.ShowLineNumbers = false
	ta.EndOfBufferCharacter = ' '
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))
	ta.SetHeight(1)
	ta.SetWidth(80)

	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = theme.InputPromptFocused
	ta.BlurredStyle.Prompt = theme.InputPromptBlurred
	ta.FocusedStyle.Text = theme.InputText
	ta.FocusedStyle.Placeholder = theme.InputPlaceholder
	ta.FocusedStyle.EndOfBuffer = lipgloss.NewStyle()
	ta.BlurredStyle.EndOfBuffer = lipgloss.NewStyle()
	_ = ta.Focus()

	historyContent := renderMessageHistoryToString(messages, false)

	m := chatModel{
		backend:      b,
		bridge:       bridge,
		sessionID:    sessionID,
		modelID:      modelID,
		agentID:      agentID,
		modelName:    modelName,
		agentName:    agentName,
		chatState:    &ChatState{},
		input:        ta,
		mode:         modeChat,
		chatLog:      new(strings.Builder),
		outputLog:    new(strings.Builder),
		lastMessages: messages,
		stream:       newStreamModel(),
	}

	banner := strings.Trim(consts.ASCIIArts[rand.IntN(len(consts.ASCIIArts))], "\n")
	bannerLine := styleUserHeader.Render(banner) + "\n" +
		styleBarSep.Render("  ai agent platform") + "\n\n"
	m.bannerContent = bannerLine
	m.chatLog.WriteString(bannerLine)
	m.chatLog.WriteString(historyContent)

	return m
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(tea.WindowSize(), textarea.Blink, m.fetchTokenCount())
}

func (m chatModel) updateHuhOverlay(msg tea.Msg) (chatModel, tea.Cmd, bool) {
	if m.mode != modeOverlay {
		return m, nil, false
	}
	form, ok := m.overlay.(*huh.Form)
	if !ok {
		return m, nil, false
	}
	newModel, cmd := form.Update(msg)
	if f, ok := newModel.(*huh.Form); ok {
		m.overlay = f
		form = f
	}
	if form.State == huh.StateCompleted {
		if m.overlayRespCh != nil {
			m.overlayRespCh <- overlayResponse{formSubmitted: true}
			m.overlayRespCh = nil
		}
		m.mode = modeChat
		m.overlay = nil
	} else if form.State == huh.StateAborted {
		if m.overlayRespCh != nil {
			m.overlayRespCh <- overlayResponse{formSubmitted: false}
			m.overlayRespCh = nil
		}
		m.mode = modeChat
		m.overlay = nil
	}
	return m, cmd, true
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		SetRenderWidth(msg.Width)
		m.input.SetWidth(m.width - 4)
		vpHeight := m.calcViewportHeight()
		if !m.ready {
			m.chatLog.Reset()
			m.chatLog.WriteString(m.bannerContent)
			if len(m.lastMessages) > 0 {
				m.chatLog.WriteString(renderMessageHistoryToString(m.lastMessages, m.showReasoning))
			}
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent(m.chatLog.String())
			m.viewport.GotoBottom()
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}
		if m.mode == modeOverlay {
			if m2, cmd, routed := m.updateHuhOverlay(msg); routed {
				return m2, cmd
			}
		}
		return m, nil

	case tokenCountMsg:
		m.tokenConsumed = msg.count
		return m, nil

	case refreshSessionMsg:
		m.tokenConsumed = msg.tokenConsumed
		m.lastMessages = msg.messages
		m.pendingHistory = false
		m.chatLog.Reset()
		m.chatLog.WriteString(m.bannerContent)
		if len(msg.messages) > 0 {
			m.chatLog.WriteString(renderMessageHistoryToString(msg.messages, m.showReasoning))
		}
		m.refreshViewport()
		return m, nil

	case overlayRequestMsg:
		m.mode = modeOverlay
		req := msg.request
		switch req.kind {
		case overlayKindList:
			o := newListOverlay(req.title, req.items, req.hints, req.responseCh)
			o.subtitle = req.subtitle
			m.overlay = o
		case overlayKindMultiSelect:
			m.overlay = newMultiSelectOverlay(req.title, req.options, req.defaultIndices, req.responseCh)
		case overlayKindHuhForm:
			formWidth := m.width - 8
			if formWidth > 80 {
				formWidth = 80
			}
			if formWidth < 20 {
				formWidth = 20
			}
			m.huhFormWidth = formWidth
			form := req.huhForm.WithWidth(formWidth).WithTheme(theme.NewHuhTheme())
			m.overlay = form
			m.overlayRespCh = req.responseCh
			return m, form.Init()
		case overlayKindLogViewer:
			m.overlay = newLogViewerOverlay(m.width, m.height, req.responseCh)
		}
		return m, nil

	case appendOutputMsg:
		m.outputLog.WriteString(msg.text)
		m.updateViewportContent()
		return m, nil

	case clearChatMsg:
		m.chatLog.Reset()
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		if m.mode == modeOverlay {
			if m2, cmd, routed := m.updateHuhOverlay(msg); routed {
				return m2, cmd
			}
			return m.handleOverlayKey(msg)
		}
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		if m.mode == modeOverlay {
			if lv, ok := m.overlay.(*logViewerOverlay); ok {
				if cmd := lv.handleMouse(msg); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		} else if m.mode == modeChat {
			var vpCmd tea.Cmd
			m.viewport, vpCmd = m.viewport.Update(msg)
			if vpCmd != nil {
				cmds = append(cmds, vpCmd)
			}
		}
		return m, tea.Batch(cmds...)

	case streamEventMsg:
		if msg.event.Type == provider.EventModelSwitch {
			if id, err := uuid.Parse(msg.event.ModelID); err == nil {
				m.modelID = id
			}
			m.modelName = msg.event.ModelName
			m.adjustThinkingForModel(msg.event.ModelThinking, msg.event.ModelThinkingLevels)
			m.appendToChatLog(renderStatusMessage("⚠", "模型已自动切换至 "+msg.event.ModelName))
			m.refreshViewport()
			return m, m.stream.waitForEvent()
		}
		m.stream.handleEvent(msg.event, m.modelName)
		m.refreshViewport()
		return m, m.stream.waitForEvent()

	case streamDoneMsg:
		m.stream.active = false
		content := m.stream.finalize(m.showReasoning)
		m.chatLog.WriteString(content)
		m.refreshViewport()
		if msg.err != nil {
			m.appendToChatLog(renderErrorMessage(msg.err.Error()))
			m.refreshViewport()
			return m, m.fetchTokenCount()
		}
		m.pendingHistory = true
		return m, m.fetchAndRefreshHistory()

	case commandDoneMsg:
		return m.handleCommandDone(msg)

	case argCompletionMsg:
		m.completion.handleArgMsg(msg)
		m.updateLayout()
		return m, nil

	case spinTickMsg:
		if m.stream.active {
			m.stream.spinIdx = (m.stream.spinIdx + 1) % len(spinnerFrames)
			m.stream.tickCount++
			return m, tickCmd()
		}
		return m, nil

	default:
		if m.mode == modeOverlay {
			if m2, cmd, routed := m.updateHuhOverlay(msg); routed {
				return m2, cmd
			}
		} else {
			var taCmd tea.Cmd
			m.input, taCmd = m.input.Update(msg)
			if taCmd != nil {
				return m, taCmd
			}
		}
	}

	return m, nil
}

func (m chatModel) View() string {
	if !m.ready {
		return "  Initializing..."
	}

	if m.mode == modeOverlay && m.overlay != nil {
		overlayView := m.renderOverlay()
		lines := strings.Split(overlayView, "\n")
		for len(lines) < m.height {
			lines = append(lines, "")
		}
		if len(lines) > m.height {
			lines = lines[:m.height]
		}
		return strings.Join(lines, "\n")
	}

	vpView := m.viewport.View()
	topSep := m.renderTopSeparator()
	inputView := m.input.View()
	botSep := m.renderBottomSeparator()
	hintsLine := m.renderHintsLine()

	parts := []string{vpView, topSep}
	parts = append(parts, inputView)
	if m.completion.visible && len(m.completion.items) > 0 {
		parts = append(parts, m.completion.render())
	}
	parts = append(parts, botSep, hintsLine)

	return strings.Join(parts, "\n")
}

func (m chatModel) renderOverlay() string {
	switch o := m.overlay.(type) {
	case *listOverlay:
		return o.render(m.width, m.height)
	case *multiSelectOverlay:
		return o.render(m.width, m.height)
	case *logViewerOverlay:
		return o.render(m.width, m.height)
	case *huh.Form:
		view := o.View()
		lines := strings.Split(view, "\n")
		topPad := (m.height - len(lines)) / 3
		if topPad < 2 {
			topPad = 2
		}
		leftPad := (m.width - m.huhFormWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}
		if leftPad > 0 {
			pad := strings.Repeat(" ", leftPad)
			paddedLines := make([]string, len(lines))
			for i, l := range lines {
				paddedLines[i] = pad + l
			}
			return strings.Repeat("\n", topPad) + strings.Join(paddedLines, "\n")
		}
		return strings.Repeat("\n", topPad) + view
	}
	return ""
}

func (m *chatModel) handleOverlayKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var done bool
	var cmd tea.Cmd
	switch o := m.overlay.(type) {
	case *listOverlay:
		done = o.handleKey(msg)
	case *multiSelectOverlay:
		done = o.handleKey(msg)
	case *logViewerOverlay:
		done, cmd = o.handleKey(msg)
	}
	if done {
		m.mode = modeChat
		m.overlay = nil
	}
	return m, cmd
}

func (m chatModel) renderTopSeparator() string {
	var rightParts []string

	modelPart := styleBarModel.Render(" ▸ " + m.modelName)
	rightParts = append(rightParts, modelPart)

	if m.chatState.Thinking {
		level := m.chatState.ThinkingLevel
		if level == "" {
			level = "auto"
		}
		thinkPart := styleBarThink.Render("  thinking: " + level)
		rightParts = append(rightParts, thinkPart)
	}

	rightLabel := strings.Join(rightParts, "") + " "
	rightWidth := lipgloss.Width(rightLabel)

	leftWidth := m.width - rightWidth
	if leftWidth < 0 {
		leftWidth = 0
	}

	if m.stream.active && m.stream.phrase != "" {
		frame := styleSpinner.Render(spinnerFrames[m.stream.spinIdx%len(spinnerFrames)])
		phrase := styleSpinTxt.Render(m.stream.phrase)
		indicator := frame + " " + phrase
		indicatorWidth := lipgloss.Width(indicator)
		dashWidth := leftWidth - indicatorWidth - 2
		if dashWidth < 0 {
			dashWidth = 0
		}
		dashes := styleBarSep.Render(strings.Repeat("─", dashWidth))
		return indicator + "  " + dashes + rightLabel
	}

	leftPart := styleBarSep.Render(strings.Repeat("─", leftWidth))
	return leftPart + rightLabel
}

func (m chatModel) renderBottomSeparator() string {
	return styleBarSep.Render(strings.Repeat("─", m.width))
}

func (m chatModel) renderHintsLine() string {
	var leftItems []string
	leftItems = append(leftItems, styleGray.Render(fmt.Sprintf("tokens: %d", m.tokenConsumed)))
	if m.stream.active {
		leftItems = append(leftItems, styleStreaming.Render("streaming..."))
	}

	sepStr := styleHintMuted.Render(" · ")
	leftStr := "  " + strings.Join(leftItems, sepStr)

	rightStr := styleHintMuted.Render("/help · alt+↵ newline · ctrl+r thinking  ")

	gap := m.width - lipgloss.Width(leftStr) - lipgloss.Width(rightStr)
	if gap < 0 {
		gap = 0
	}

	return leftStr + strings.Repeat(" ", gap) + rightStr
}

func findCommand(name string) *Command {
	for i := range commands {
		if commands[i].Name == name {
			return &commands[i]
		}
	}
	return nil
}

func (m *chatModel) calcViewportHeight() int {
	inputH := m.input.LineCount()
	if inputH < 1 {
		inputH = 1
	}
	if inputH > maxInputLines {
		inputH = maxInputLines
	}

	completionsH := m.completion.height()

	h := m.height - 1 - inputH - completionsH - 1 - 1
	if h < 1 {
		h = 1
	}
	return h
}

func (m *chatModel) updateLayout() {
	if m.ready {
		vpHeight := m.calcViewportHeight()
		if m.viewport.Height != vpHeight {
			m.viewport.Height = vpHeight
		}
	}
}

func (m *chatModel) fetchTokenCount() tea.Cmd {
	sid := m.sessionID
	b := m.backend
	return func() tea.Msg {
		session, err := b.GetSession(sid)
		if err != nil {
			return nil
		}
		return tokenCountMsg{count: int(session.TokenConsumed)}
	}
}

func (m *chatModel) fetchAndRefreshHistory() tea.Cmd {
	sid := m.sessionID
	b := m.backend
	return func() tea.Msg {
		session, err := b.GetSession(sid)
		if err != nil {
			return nil
		}
		return refreshSessionMsg{
			tokenConsumed: int(session.TokenConsumed),
			messages:      session.Messages,
		}
	}
}

func (m *chatModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyCtrlR:
		return m.toggleReasoning()

	case tea.KeyPgUp:
		m.viewport.HalfViewUp()
		m.completion.visible = false
		return m, nil

	case tea.KeyPgDown:
		m.viewport.HalfViewDown()
		m.completion.visible = false
		return m, nil

	case tea.KeyTab:
		newInput, changed, cmd := m.completion.handleTab(m.input.Value(), m.backend, m.modelID)
		if changed {
			m.input.SetValue(newInput)
			m.input.CursorEnd()
		}
		m.updateLayout()
		return m, cmd

	case tea.KeyEsc:
		if m.completion.visible {
			m.completion.dismiss()
			m.updateLayout()
			return m, nil
		}
		return m, nil

	case tea.KeyEnter:
		if msg.Alt {
			var taCmd tea.Cmd
			m.input, taCmd = m.input.Update(msg)
			newH := m.input.LineCount()
			if newH < 1 {
				newH = 1
			}
			if newH > maxInputLines {
				newH = maxInputLines
			}
			if m.input.Height() != newH {
				m.input.SetHeight(newH)
			}
			m.updateLayout()
			return m, taCmd
		}

		if selected, ok := m.completion.handleEnterSelection(); ok {
			m.input.SetValue("")
			m.input.SetHeight(1)
			m.updateLayout()
			return m.handleSlashInput(selected)
		}

		if m.stream.active {
			return m, nil
		}

		input := strings.TrimSpace(m.input.Value())
		if input == "" {
			return m, nil
		}

		m.input.SetValue("")
		m.input.SetHeight(1)
		m.completion.visible = false
		m.updateLayout()

		if strings.HasPrefix(input, "/") {
			return m.handleSlashInput(input)
		}

		return m.handleChatInput(input)
	}

	if m.stream.active {
		return m, nil
	}

	var taCmd tea.Cmd
	m.input, taCmd = m.input.Update(msg)

	m.completion.updateLive(m.input.Value())

	newH := m.input.LineCount()
	if newH < 1 {
		newH = 1
	}
	if newH > maxInputLines {
		newH = maxInputLines
	}
	if m.input.Height() != newH {
		m.input.SetHeight(newH)
	}
	m.updateLayout()

	return m, taCmd
}

func (m *chatModel) handleSlashInput(input string) (tea.Model, tea.Cmd) {
	parts := parseSlashInput(input)
	if len(parts) == 0 {
		return m, nil
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "/exit":
		return m, tea.Quit
	case "/help":
		m.appendToChatLog(renderCommandHintsToString())
		m.refreshViewport()
		return m, nil
	}

	return m.execInteractiveCommand(input)
}

func (m *chatModel) execInteractiveCommand(input string) (tea.Model, tea.Cmd) {
	parts := parseSlashInput(input)
	command := strings.ToLower(parts[0])
	args := parts[1:]

	handler, ok := commandRegistry[command]
	if !ok {
		m.appendToChatLog(renderMatchingCommandHints(input))
		m.refreshViewport()
		return m, nil
	}

	b := m.backend
	bridge := m.bridge
	sid := m.sessionID
	mid := m.modelID
	aid := m.agentID
	state := m.chatState

	go func() {
		result, err := handler(b, bridge, args, sid, mid, aid, state)
		bridge.program.Send(commandDoneMsg{result: result, err: err})
	}()

	return m, nil
}

func (m *chatModel) handleCommandDone(msg commandDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.appendToChatLog(renderErrorMessage(msg.err.Error()))
		m.refreshViewport()
		return m, nil
	}

	if msg.result.NewModelID != uuid.Nil {
		m.modelID = msg.result.NewModelID
		if msg.result.NewModelName != "" {
			m.modelName = msg.result.NewModelName
		}
	}

	if msg.result.NewAgentID != uuid.Nil {
		m.agentID = msg.result.NewAgentID
		if msg.result.NewAgentName != "" {
			m.agentName = msg.result.NewAgentName
		}
	}

	if msg.result.NewSessionID != uuid.Nil {
		m.sessionID = msg.result.NewSessionID
		m.chatState.HistoryOffset = 0
		m.tokenConsumed = int(msg.result.TokenConsumed)

		m.outputLog.Reset()
		m.chatLog.Reset()
		m.chatLog.WriteString(m.bannerContent)
		if len(msg.result.SessionMessages) > 0 {
			m.lastMessages = msg.result.SessionMessages
			m.chatLog.WriteString(renderMessageHistoryToString(msg.result.SessionMessages, m.showReasoning))
		} else {
			m.lastMessages = nil
		}
		m.refreshViewport()
	}

	if msg.result.ShouldExit {
		return m, tea.Quit
	}

	return m, m.fetchTokenCount()
}

func (m *chatModel) handleChatInput(input string) (tea.Model, tea.Cmd) {
	m.outputLog.Reset()
	m.appendToChatLog(renderUserHeader(time.Now()) + "\n")
	m.appendToChatLog(renderUserPlainBlock(input) + "\n")
	m.refreshViewport()

	m.stream.start()

	go func() {
		err := m.backend.StreamChat(signal.GetBaseContext(), m.sessionID, &models.ChatDto{
			ModelID:       m.modelID,
			Message:       input,
			Thinking:      m.chatState.Thinking,
			ThinkingLevel: m.chatState.ThinkingLevel,
		}, func(evt provider.StreamEvent) error {
			m.stream.ch <- evt
			return nil
		})
		close(m.stream.ch)
		m.stream.doneCh <- err
	}()

	return m, tea.Batch(m.stream.waitForEvent(), tickCmd())
}

func (m *chatModel) toggleReasoning() (tea.Model, tea.Cmd) {
	m.showReasoning = !m.showReasoning
	if m.pendingHistory {
		return m, nil
	}
	if len(m.lastMessages) > 0 {
		history := renderMessageHistoryToString(m.lastMessages, m.showReasoning)
		m.chatLog.Reset()
		m.chatLog.WriteString(m.bannerContent)
		m.chatLog.WriteString(history)
		m.refreshViewport()
	}
	return m, nil
}

func (m *chatModel) adjustThinkingForModel(modelThinking bool, thinkingLevels []string) {
	if !m.chatState.Thinking {
		return
	}
	if !modelThinking {
		m.chatState.Thinking = false
		m.chatState.ThinkingLevel = ""
		return
	}
	if len(thinkingLevels) == 0 {
		m.chatState.ThinkingLevel = ""
		return
	}
	for _, l := range thinkingLevels {
		if l == m.chatState.ThinkingLevel {
			return
		}
	}
	m.chatState.Thinking = false
	m.chatState.ThinkingLevel = ""
}

func (m *chatModel) appendToChatLog(s string) {
	m.chatLog.WriteString(s)
}

func (m *chatModel) viewportContent() string {
	content := m.chatLog.String()
	if m.outputLog.Len() > 0 {
		content += m.outputLog.String()
	}
	if m.stream.active {
		content += m.stream.liveContent(m.showReasoning)
	}
	return content
}

func (m *chatModel) updateViewportContent() {
	m.updateLayout()
	m.viewport.SetContent(m.viewportContent())
}

func (m *chatModel) refreshViewport() {
	m.updateLayout()
	m.viewport.SetContent(m.viewportContent())
	m.viewport.GotoBottom()
}
