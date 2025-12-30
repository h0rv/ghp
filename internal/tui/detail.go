package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/pkg/browser"
	"github.com/robby/ghp/internal/domain"
	"github.com/robby/ghp/internal/gh"
)

// Layout constants
const (
	leftPanelRatio = 0.35 // Left panel takes 35% of width
	minLeftWidth   = 30
	maxLeftWidth   = 50
	headerHeight   = 1
	footerHeight   = 1
	borderSize     = 2 // Top + bottom border
)

// Detail view styles
var (
	detailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	commentAuthorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	commentTimeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	commentBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	pendingCommentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("228")).
				Italic(true)

	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240"))

	focusedPanelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205"))

	commentInputBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("228"))

	scrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("228")).
			Bold(true)
)

// DetailModel represents the card detail view with split-screen layout
type DetailModel struct {
	// Dependencies
	client *gh.Client
	ctx    context.Context

	// Card data
	card     *domain.Card
	comments []domain.Comment

	// UI components
	spinner      spinner.Model
	commentInput textarea.Model
	viewport     viewport.Model

	// State
	commentMode     bool
	confirmExit     bool // Show "unsaved changes" prompt
	loading         bool
	loadingAction   string
	loadingComments bool
	commentsError   string
	errorMsg        string
	successMsg      string

	// View dimensions
	width  int
	height int
}

// NewDetailModel creates a new detail view model
func NewDetailModel(card *domain.Card, client *gh.Client, ctx context.Context) DetailModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ta := textarea.New()
	ta.Placeholder = "Write your comment here..."
	ta.CharLimit = 65535
	ta.SetHeight(6)
	ta.SetWidth(40) // Will be resized
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // No highlight on cursor line
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("228"))
	ta.BlurredStyle.Base = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	vp := viewport.New(40, 10) // Will be resized in WindowSizeMsg
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3

	return DetailModel{
		client:       client,
		ctx:          ctx,
		card:         card,
		spinner:      sp,
		commentInput: ta,
		viewport:     vp,
	}
}

// Init initializes the detail model
func (m DetailModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, tea.WindowSize()}
	if m.card.ContentType == domain.ContentTypeIssue || m.card.ContentType == domain.ContentTypePullRequest {
		m.loadingComments = true
		cmds = append(cmds, m.loadComments())
	}
	return tea.Batch(cmds...)
}

// Update handles messages
func (m DetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeComponents()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case commentPostedMsg:
		m.loading = false
		m.commentMode = false
		m.successMsg = "Comment posted!"
		m.commentInput.Reset()
		// Reload comments to show the new one
		return m, m.loadComments()

	case commentErrorMsg:
		m.loading = false
		m.errorMsg = fmt.Sprintf("Failed: %v", msg.err)
		return m, nil

	case commentsLoadedMsg:
		m.loadingComments = false
		m.comments = msg.comments
		m.updateViewportContent()
		return m, nil

	case commentsErrorMsg:
		m.loadingComments = false
		m.commentsError = msg.err.Error()
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.MouseMsg:
		// Forward mouse events to viewport when not in comment mode
		if !m.commentMode {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Update textarea when in comment mode (for blink, etc.)
	if m.commentMode {
		var cmd tea.Cmd
		m.commentInput, cmd = m.commentInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// resizeComponents calculates and sets component dimensions
func (m *DetailModel) resizeComponents() {
	// Calculate panel widths
	leftWidth := int(float64(m.width) * leftPanelRatio)
	if leftWidth < minLeftWidth {
		leftWidth = minLeftWidth
	}
	if leftWidth > maxLeftWidth {
		leftWidth = maxLeftWidth
	}

	// Right panel gets remaining width minus borders and gap
	rightWidth := m.width - leftWidth - 3 // 3 = gap between panels
	if rightWidth < 30 {
		rightWidth = 30
	}

	// Content height = total - header - footer - borders
	contentHeight := m.height - headerHeight - footerHeight - borderSize
	if contentHeight < 10 {
		contentHeight = 10
	}

	// Set viewport dimensions (account for border in right panel)
	m.viewport.Width = rightWidth - borderSize - 2     // -2 for padding
	m.viewport.Height = contentHeight - borderSize - 8 // Reserve space for comment input

	// Update comment input width
	m.commentInput.SetWidth(rightWidth - borderSize - 4)

	// Re-render viewport content with new width (body + comments)
	if m.card.Body != "" || len(m.comments) > 0 {
		m.updateViewportContent()
	}
}

// handleKeyPress processes keyboard input
func (m DetailModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Confirm exit dialog
	if m.confirmExit {
		switch msg.String() {
		case "y", "Y":
			// Discard and exit
			m.confirmExit = false
			m.commentMode = false
			m.commentInput.Reset()
			m.commentInput.Blur()
			return m, func() tea.Msg { return closeDetailMsg{} }
		case "n", "N", "esc":
			// Cancel, stay in comment mode
			m.confirmExit = false
			return m, nil
		case "s", "S":
			// Save and exit
			m.confirmExit = false
			comment := strings.TrimSpace(m.commentInput.Value())
			if comment != "" {
				m.loading = true
				m.loadingAction = "Posting..."
				return m, m.postComment(comment)
			}
			return m, nil
		}
		return m, nil
	}

	// Comment mode - textarea gets all key events except special ones
	if m.commentMode {
		switch msg.String() {
		case "esc":
			// Check if there's unsaved content
			if strings.TrimSpace(m.commentInput.Value()) != "" {
				m.confirmExit = true
				return m, nil
			}
			m.commentMode = false
			m.commentInput.Blur()
			return m, nil
		case "ctrl+s":
			comment := strings.TrimSpace(m.commentInput.Value())
			if comment != "" {
				m.loading = true
				m.loadingAction = "Posting..."
				return m, m.postComment(comment)
			}
			return m, nil
		default:
			// Forward ALL other keys to textarea
			var cmd tea.Cmd
			m.commentInput, cmd = m.commentInput.Update(msg)
			return m, cmd
		}
	}

	// Normal mode - viewport scrolling
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return closeDetailMsg{} }
	case "o":
		if m.card.URL != "" {
			_ = browser.OpenURL(m.card.URL)
		}
	case "c":
		if m.card.ContentType == domain.ContentTypeIssue || m.card.ContentType == domain.ContentTypePullRequest {
			m.commentMode = true
			m.commentInput.Focus()
			m.errorMsg = ""
			m.successMsg = ""
			return m, textarea.Blink
		}
	case "j", "down":
		m.viewport.LineDown(1)
	case "k", "up":
		m.viewport.LineUp(1)
	case "ctrl+d":
		m.viewport.HalfViewDown()
	case "ctrl+u":
		m.viewport.HalfViewUp()
	case "g":
		m.viewport.GotoTop()
	case "G":
		m.viewport.GotoBottom()
	}

	return m, nil
}

// View renders the split-screen detail view
func (m DetailModel) View() string {
	width := m.width
	height := m.height
	if width == 0 {
		width = 100
	}
	if height == 0 {
		height = 30
	}

	// Calculate panel widths
	leftWidth := int(float64(width) * leftPanelRatio)
	if leftWidth < minLeftWidth {
		leftWidth = minLeftWidth
	}
	if leftWidth > maxLeftWidth {
		leftWidth = maxLeftWidth
	}
	rightWidth := width - leftWidth - 1 // 1 char gap

	// Content height
	contentHeight := height - headerHeight - footerHeight
	if contentHeight < 10 {
		contentHeight = 10
	}

	// === HEADER ===
	header := m.renderHeader(width)

	// === LEFT PANEL: Issue Info ===
	leftContent := m.renderLeftPanel(leftWidth-borderSize, contentHeight-borderSize)
	leftPanel := panelBorderStyle.
		Width(leftWidth - borderSize).
		Height(contentHeight - borderSize).
		Render(leftContent)

	// === RIGHT PANEL: Comments ===
	rightContent := m.renderRightPanel(rightWidth-borderSize, contentHeight-borderSize)
	rightBorder := focusedPanelBorderStyle
	if m.commentMode {
		rightBorder = panelBorderStyle // Unfocus when typing
	}
	rightPanel := rightBorder.
		Width(rightWidth - borderSize).
		Height(contentHeight - borderSize).
		Render(rightContent)

	// Join panels horizontally
	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)

	// === FOOTER ===
	footer := m.renderFooter(width)

	// Join everything vertically
	return lipgloss.JoinVertical(lipgloss.Left, header, panels, footer)
}

// renderHeader renders the top help bar
func (m DetailModel) renderHeader(width int) string {
	if m.confirmExit {
		return warningStyle.Render("Unsaved comment! [Y]discard [N]cancel [S]save and exit")
	}

	if m.commentMode {
		return dimStyle.Render("[Ctrl+S]save [ESC]cancel") + "  " +
			commentAuthorStyle.Render("Writing comment...")
	}

	var parts []string
	parts = append(parts, "[q]back")
	parts = append(parts, "[o]open")
	parts = append(parts, "[j/k]scroll")
	parts = append(parts, "[g/G]top/bottom")

	if m.card.ContentType == domain.ContentTypeIssue || m.card.ContentType == domain.ContentTypePullRequest {
		parts = append(parts, "[c]comment")
	}

	help := strings.Join(parts, " ")
	return dimStyle.Render(help)
}

// renderFooter renders the bottom status bar
func (m DetailModel) renderFooter(width int) string {
	var left, right string

	// Left: status messages
	if m.loading {
		left = m.spinner.View() + " " + m.loadingAction
	} else if m.successMsg != "" {
		left = lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Render("✓ " + m.successMsg)
	} else if m.errorMsg != "" {
		left = errorStyle.Render("✗ " + m.errorMsg)
	} else if m.commentMode {
		charCount := len(m.commentInput.Value())
		left = fmt.Sprintf("%d chars", charCount)
	}

	// Right: scroll position
	if len(m.comments) > 0 && !m.commentMode {
		scrollPct := int(m.viewport.ScrollPercent() * 100)
		if m.viewport.AtTop() {
			right = "TOP"
		} else if m.viewport.AtBottom() {
			right = "END"
		} else {
			right = fmt.Sprintf("%d%%", scrollPct)
		}
	}

	// Pad between left and right
	padding := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if padding < 1 {
		padding = 1
	}

	return dimStyle.Render(left) + strings.Repeat(" ", padding) + dimStyle.Render(right)
}

// renderLeftPanel renders the issue metadata panel
func (m DetailModel) renderLeftPanel(width, height int) string {
	var b strings.Builder

	// Type and number
	typeStr := m.card.ContentType
	if m.card.Number > 0 {
		typeStr = fmt.Sprintf("%s #%d", typeStr, m.card.Number)
	}
	b.WriteString(detailLabelStyle.Render(typeStr))
	b.WriteString("\n\n")

	// Title (wrapped)
	title := wordwrap.String(m.card.Title, width-2)
	b.WriteString(detailTitleStyle.Render(title))
	b.WriteString("\n\n")

	// Metadata fields
	if m.card.Repo != "" {
		b.WriteString(detailLabelStyle.Render("Repo: "))
		b.WriteString(detailValueStyle.Render(m.card.Repo))
		b.WriteString("\n")
	}

	if m.card.State != "" {
		b.WriteString(detailLabelStyle.Render("State: "))
		stateStyle := detailValueStyle
		switch m.card.State {
		case "OPEN":
			stateStyle = stateStyle.Foreground(lipgloss.Color("34"))
		case "CLOSED":
			stateStyle = stateStyle.Foreground(lipgloss.Color("196"))
		case "MERGED":
			stateStyle = stateStyle.Foreground(lipgloss.Color("141"))
		}
		b.WriteString(stateStyle.Render(m.card.State))
		b.WriteString("\n")
	}

	if len(m.card.Assignees) > 0 {
		b.WriteString(detailLabelStyle.Render("Assigned: "))
		assignees := strings.Join(m.card.Assignees, ", ")
		if len(assignees) > width-10 {
			assignees = assignees[:width-13] + "..."
		}
		b.WriteString(detailValueStyle.Render(assignees))
		b.WriteString("\n")
	}

	if len(m.card.Labels) > 0 {
		b.WriteString(detailLabelStyle.Render("Labels: "))
		labels := strings.Join(m.card.Labels, ", ")
		if len(labels) > width-10 {
			labels = labels[:width-13] + "..."
		}
		b.WriteString(detailValueStyle.Render(labels))
		b.WriteString("\n")
	}

	// Body preview
	if m.card.Body != "" {
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Description:"))
		b.WriteString("\n")
		body := m.card.Body
		// Limit body to fit in remaining space
		maxBodyLines := height - strings.Count(b.String(), "\n") - 2
		if maxBodyLines > 0 {
			wrapped := wordwrap.String(body, width-2)
			lines := strings.Split(wrapped, "\n")
			if len(lines) > maxBodyLines {
				lines = lines[:maxBodyLines-1]
				lines = append(lines, "...")
			}
			b.WriteString(strings.Join(lines, "\n"))
		}
	}

	return b.String()
}

// renderRightPanel renders the comments panel with viewport
func (m DetailModel) renderRightPanel(width, height int) string {
	var b strings.Builder

	// Panel title - include description as part of the discussion
	title := "Discussion"
	commentCount := len(m.comments)
	if m.card.Body != "" {
		// Description counts as the first entry
		title = fmt.Sprintf("Discussion (%d)", commentCount+1)
	} else if commentCount > 0 {
		title = fmt.Sprintf("Discussion (%d)", commentCount)
	}

	// Show scroll indicator if there's overflow
	scrollHint := ""
	if (m.card.Body != "" || len(m.comments) > 0) && !m.commentMode {
		totalLines := m.viewport.TotalLineCount()
		visibleLines := m.viewport.Height
		if totalLines > visibleLines {
			if m.viewport.AtTop() {
				scrollHint = " ↓"
			} else if m.viewport.AtBottom() {
				scrollHint = " ↑"
			} else {
				scrollHint = " ↕"
			}
		}
	}

	b.WriteString(detailLabelStyle.Render(title))
	b.WriteString(scrollIndicatorStyle.Render(scrollHint))
	b.WriteString("\n")

	// Loading state
	if m.loadingComments {
		b.WriteString("\n")
		b.WriteString(m.spinner.View() + " Loading comments...")
		return b.String()
	}

	// Error state
	if m.commentsError != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("Error: " + m.commentsError))
		return b.String()
	}

	// Comment mode - show input prominently
	if m.commentMode {
		b.WriteString("\n")
		b.WriteString(commentAuthorStyle.Render("New Comment"))
		b.WriteString("\n\n")
		b.WriteString(m.commentInput.View())
		b.WriteString("\n\n")
		b.WriteString(dimStyle.Render("Ctrl+S to save • ESC to cancel"))

		// Show existing comments below (condensed)
		if len(m.comments) > 0 {
			b.WriteString("\n\n")
			b.WriteString(detailLabelStyle.Render(fmt.Sprintf("── %d existing comments ──", len(m.comments))))
		}
		return b.String()
	}

	// Empty state (no description AND no comments)
	if m.card.Body == "" && len(m.comments) == 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("No description or comments"))
		if m.card.ContentType == domain.ContentTypeIssue || m.card.ContentType == domain.ContentTypePullRequest {
			b.WriteString("\n\n")
			b.WriteString(dimStyle.Render("Press 'c' to add a comment"))
		}
		return b.String()
	}

	// Viewport with comments
	b.WriteString(m.viewport.View())

	return b.String()
}

// updateViewportContent formats description and comments for viewport display
func (m *DetailModel) updateViewportContent() {
	var b strings.Builder
	wrapWidth := m.viewport.Width - 4
	if wrapWidth < 30 {
		wrapWidth = 30
	}

	hasContent := false

	// First: Show the issue/PR description (body) as the opening post
	if m.card.Body != "" {
		author := m.card.Author
		if author == "" {
			author = "Author"
		}
		timeAgo := formatTimeAgo(m.card.CreatedAt)

		// Description header with "OP" indicator
		b.WriteString(commentAuthorStyle.Render(author))
		b.WriteString(" ")
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Bold(true).
			Render("OP"))
		b.WriteString(" ")
		b.WriteString(commentTimeStyle.Render(timeAgo))
		b.WriteString("\n")

		// Description body with wrapping
		wrapped := wordwrap.String(m.card.Body, wrapWidth)
		b.WriteString(commentBodyStyle.Render(wrapped))
		hasContent = true
	}

	// Then: Show all comments
	for i, c := range m.comments {
		// Add separator before each comment (or after description)
		if hasContent || i > 0 {
			b.WriteString("\n\n")
			b.WriteString(dimStyle.Render(strings.Repeat("─", min(20, wrapWidth))))
			b.WriteString("\n\n")
		}

		author := c.Author
		if author == "" {
			author = "(deleted)"
		}
		timeAgo := formatTimeAgo(c.CreatedAt)

		// Comment header
		b.WriteString(commentAuthorStyle.Render(author))
		b.WriteString(" ")
		b.WriteString(commentTimeStyle.Render(timeAgo))
		b.WriteString("\n")

		// Comment body with wrapping
		wrapped := wordwrap.String(c.Body, wrapWidth)
		b.WriteString(commentBodyStyle.Render(wrapped))
		hasContent = true
	}

	m.viewport.SetContent(b.String())
}

// postComment creates a command to post a comment
func (m DetailModel) postComment(body string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.Split(m.card.Repo, "/")
		if len(parts) != 2 {
			return commentErrorMsg{err: fmt.Errorf("invalid repository format")}
		}

		err := m.client.AddComment(m.ctx, parts[0], parts[1], m.card.Number, body)
		if err != nil {
			return commentErrorMsg{err: err}
		}
		return commentPostedMsg{}
	}
}

// loadComments creates a command to load comments
func (m DetailModel) loadComments() tea.Cmd {
	return func() tea.Msg {
		parts := strings.Split(m.card.Repo, "/")
		if len(parts) != 2 {
			return commentsErrorMsg{err: fmt.Errorf("invalid repo format")}
		}
		comments, err := m.client.GetComments(m.ctx, parts[0], parts[1], m.card.Number)
		if err != nil {
			return commentsErrorMsg{err: err}
		}
		return commentsLoadedMsg{comments: comments}
	}
}

// formatTimeAgo converts ISO8601 timestamp to relative time
func formatTimeAgo(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		if len(timestamp) >= 10 {
			return timestamp[:10]
		}
		return timestamp
	}

	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	case duration < 30*24*time.Hour:
		weeks := int(duration.Hours() / 24 / 7)
		if weeks == 1 {
			return "1w ago"
		}
		return fmt.Sprintf("%dw ago", weeks)
	case duration < 365*24*time.Hour:
		months := int(duration.Hours() / 24 / 30)
		if months == 1 {
			return "1mo ago"
		}
		return fmt.Sprintf("%dmo ago", months)
	default:
		years := int(duration.Hours() / 24 / 365)
		if years == 1 {
			return "1y ago"
		}
		return fmt.Sprintf("%dy ago", years)
	}
}

// min returns the smaller of two ints
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Message types for detail view
type (
	closeDetailMsg    struct{}
	commentPostedMsg  struct{}
	commentErrorMsg   struct{ err error }
	commentsLoadedMsg struct{ comments []domain.Comment }
	commentsErrorMsg  struct{ err error }
)
