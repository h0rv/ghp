package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/h0rv/ghp/internal/domain"
	"github.com/h0rv/ghp/internal/gh"
	"github.com/h0rv/ghp/internal/store"
	"github.com/pkg/browser"
)

// Layout constants
const (
	minColumnWidth = 20
	maxColumnWidth = 35
	headerLines    = 1  // Single header line with title + status
	pageJumpSize   = 10 // Number of items to jump with Ctrl+D/U
)

// Styles for the board view - base styles without width/height (set dynamically)
var (
	columnHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	cardStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	selectedCardStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	titleStyle = lipgloss.NewStyle().
			Bold(true)

	moveModeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("205")).
			Foreground(lipgloss.Color("0")).
			Padding(0, 1)
)

// BoardModel represents the main kanban board view
type BoardModel struct {
	// Dependencies
	store  *store.Store
	client *gh.Client
	ctx    context.Context

	// UI components
	keymap      KeyMap
	help        HelpModel
	spinner     spinner.Model
	filterInput textinput.Model

	// Board state
	columns        []string            // Column IDs in order
	columnNames    map[string]string   // Column ID -> display name
	filteredCards  map[string][]string // Column ID -> card IDs
	selectedColumn int                 // Currently selected column
	columnOffset   int                 // Horizontal scroll offset (first visible column index)
	selectedCard   map[string]int      // Column ID -> selected card index
	scrollOffset   map[string]int      // Column ID -> scroll offset

	// View state
	width        int
	height       int
	showHelp     bool
	filterMode   bool
	filterText   string
	filterMyOnly bool // Toggle to show only items assigned to me
	moveMode     bool
	loading      bool
	loadingMore  bool   // True while loading more pages in background
	nextCursor   string // Cursor for next page, empty if all loaded
	errorToast   string
}

// NewBoardModel creates a new board model
func NewBoardModel(s *store.Store, client *gh.Client, ctx context.Context) BoardModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "Filter..."
	ti.Prompt = "/ "

	return BoardModel{
		store:         s,
		client:        client,
		ctx:           ctx,
		keymap:        DefaultKeyMap(),
		help:          NewHelpModel(DefaultKeyMap()),
		spinner:       sp,
		filterInput:   ti,
		columns:       []string{},
		columnNames:   make(map[string]string),
		filteredCards: make(map[string][]string),
		selectedCard:  make(map[string]int),
		scrollOffset:  make(map[string]int),
	}
}

// boardInitMsg triggers initial column build
type boardInitMsg struct{}

// Init initializes the board and starts background loading
func (m BoardModel) Init() tea.Cmd {
	// Always rebuild columns (even if empty) and start loading
	return tea.Batch(
		m.spinner.Tick,
		tea.WindowSize(),
		func() tea.Msg { return boardInitMsg{} },
		m.loadNextPage(""), // Start loading first page immediately
	)
}

// Update handles messages
func (m BoardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case boardInitMsg:
		(&m).rebuildColumns()
		(&m).applyFilter()
		return m, nil

	case itemsLoadedMsg:
		m.loading = false
		m.loadingMore = false
		(&m).rebuildColumns()
		(&m).applyFilter()
		return m, nil

	case pageLoadedMsg:
		// Handle lazy-loaded page
		if msg.err != nil {
			m.loadingMore = false
			m.errorToast = fmt.Sprintf("Load failed: %v", msg.err)
			return m, nil
		}

		// Add cards to store
		m.store.UpsertCards(msg.cards)
		(&m).rebuildColumns()
		(&m).applyFilter()

		// If more pages, continue loading
		if msg.hasMore && msg.nextCursor != "" {
			m.loadingMore = true
			m.nextCursor = msg.nextCursor
			return m, m.loadNextPage(msg.nextCursor)
		}

		// All done
		m.loadingMore = false
		m.nextCursor = ""
		return m, nil

	case moveSuccessMsg:
		m.moveMode = false
		(&m).rebuildColumns()
		(&m).applyFilter()
		return m, nil

	case moveErrorMsg:
		m.store.RollbackMove()
		(&m).rebuildColumns()
		(&m).applyFilter()
		m.errorToast = fmt.Sprintf("Move failed: %v", msg.err)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	}

	return m, nil
}

// handleKeyPress processes keyboard input
func (m BoardModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Help overlay
	if m.showHelp {
		if msg.String() == "?" || msg.String() == "q" || msg.String() == "esc" {
			m.showHelp = false
		}
		return m, nil
	}

	// Filter mode
	if m.filterMode {
		switch msg.String() {
		case "enter":
			m.filterMode = false
			m.filterText = m.filterInput.Value()
			(&m).applyFilter()
			return m, nil
		case "esc":
			m.filterMode = false
			m.filterInput.SetValue(m.filterText)
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			return m, cmd
		}
	}

	// Move mode
	if m.moveMode {
		return m.handleMoveMode(msg)
	}

	// Normal navigation
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "/":
		m.filterMode = true
		m.filterInput.Focus()
	case "h", "left":
		if m.selectedColumn > 0 {
			m.selectedColumn--
			(&m).adjustColumnScroll()
		}
	case "l", "right":
		if m.selectedColumn < len(m.columns)-1 {
			m.selectedColumn++
			(&m).adjustColumnScroll()
		}
	case "j", "down":
		(&m).moveCardSelection(1)
	case "k", "up":
		(&m).moveCardSelection(-1)
	case "g":
		// Go to top of current column (vim: gg)
		(&m).jumpToCard(0)
	case "G":
		// Go to bottom of current column (vim: G)
		(&m).jumpToCard(-1)
	case "ctrl+d":
		// Page down (half screen in vim, we use fixed jump)
		(&m).moveCardSelection(pageJumpSize)
	case "ctrl+u":
		// Page up
		(&m).moveCardSelection(-pageJumpSize)
	case "m":
		if m.getSelectedCard() != nil {
			m.moveMode = true
		}
	case "o":
		card := m.getSelectedCard()
		if card != nil && card.URL != "" {
			_ = browser.OpenURL(card.URL)
		}
	case "r":
		m.loading = true
		return m, m.loadAllItems()
	case "f":
		// Change group field (was 'g', now 'f' for "field")
		return m, func() tea.Msg { return changeGroupFieldMsg{} }
	case "a":
		// Toggle "assigned to me" filter
		m.filterMyOnly = !m.filterMyOnly
		(&m).applyFilter()
	case "enter":
		// Open card detail view
		card := m.getSelectedCard()
		if card != nil {
			return m, func() tea.Msg { return openDetailMsg{card: card} }
		}
	}

	return m, nil
}

// handleMoveMode handles key presses in move mode
func (m BoardModel) handleMoveMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.moveMode = false
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.Runes[0] - '1')
		if idx >= 0 && idx < len(m.columns) {
			return m, m.moveCardToColumn(m.columns[idx])
		}
	}
	return m, nil
}

// View renders the board - fills entire terminal exactly
func (m BoardModel) View() string {
	// Use sensible defaults if dimensions not yet set
	width := m.width
	height := m.height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}

	// Layout: header (1) + optional secondHeader (1) + optional filter (1) + optional move (1) + board
	// No footer - all status is in header now

	var sections []string

	// === HEADER (title + status) ===
	header := m.renderHeader(width)
	sections = append(sections, header)

	// === SECOND HEADER LINE (navigation hints + position) ===
	secondHeader := m.renderSecondHeader(width)
	sections = append(sections, secondHeader)

	// === FILTER INPUT (if active) ===
	if m.filterMode {
		sections = append(sections, m.filterInput.View())
	}

	// === MOVE MODE BANNER ===
	if m.moveMode {
		moveBar := moveModeStyle.Render("MOVE") + " Press 1-9 to select column, ESC to cancel"
		sections = append(sections, moveBar)
	}

	// Calculate board height:
	// total height - header(1) - secondHeader(1) - optional filter(1) - optional move(1)
	boardHeight := height - 2 // header + second header
	if m.filterMode {
		boardHeight--
	}
	if m.moveMode {
		boardHeight--
	}
	if boardHeight < 5 {
		boardHeight = 5
	}

	// === MAIN CONTENT ===
	var mainContent string
	if m.showHelp {
		helpContent := m.help.View(width)
		helpLines := strings.Split(helpContent, "\n")
		// Truncate help to fit in available space
		if len(helpLines) > boardHeight {
			helpLines = helpLines[:boardHeight]
		}
		mainContent = strings.Join(helpLines, "\n")
	} else if m.loading && len(m.store.GetAllCards()) == 0 {
		loadingMsg := m.spinner.View() + " Loading..."
		mainContent = lipgloss.Place(width, boardHeight, lipgloss.Center, lipgloss.Center, loadingMsg)
	} else if len(m.columns) == 0 {
		emptyMsg := "No columns available. Press 'r' to refresh."
		mainContent = lipgloss.Place(width, boardHeight, lipgloss.Center, lipgloss.Center, emptyMsg)
	} else {
		// Render kanban board - boardHeight includes space for column borders
		mainContent = m.renderBoard(width, boardHeight)
	}
	sections = append(sections, mainContent)

	// Join all sections vertically
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderSecondHeader renders navigation hints and position info
func (m BoardModel) renderSecondHeader(width int) string {
	// Build left side: navigation hints
	left := "h/l:col j/k:card m:move o:open enter:view"

	// Build right side: error toast or position info
	right := ""
	if m.errorToast != "" {
		right = errorStyle.Render(m.errorToast)
	} else if len(m.columns) > 0 {
		colID := m.columns[m.selectedColumn]
		cards := m.filteredCards[colID]

		// Show column position (col X/Y)
		colPos := fmt.Sprintf("col %d/%d", m.selectedColumn+1, len(m.columns))

		// Show card position within column
		if len(cards) > 0 {
			cardIdx := m.selectedCard[colID] + 1
			right = fmt.Sprintf("%s | card %d/%d", colPos, cardIdx, len(cards))
		} else {
			right = colPos
		}
	}

	// Calculate padding
	leftLen := len(left)
	rightLen := lipgloss.Width(right)
	padding := width - leftLen - rightLen - 2
	if padding < 1 {
		padding = 1
	}

	return dimStyle.Render(left) + strings.Repeat(" ", padding) + right
}

// renderHeader renders a single header line with title on left and status on right
func (m BoardModel) renderHeader(width int) string {
	project := m.store.GetProject()
	groupField := m.store.GetGroupField()
	if project == nil || groupField == nil {
		return ""
	}

	// Left side: project title
	title := fmt.Sprintf("%s/%d - %s (by %s)", project.Owner, project.Number, project.Title, groupField.Name)

	// Right side: status info
	var statusParts []string

	// Loading indicator
	if m.loadingMore {
		statusParts = append(statusParts, m.spinner.View()+"loading")
	}

	// Item count
	totalItems := 0
	for _, cards := range m.filteredCards {
		totalItems += len(cards)
	}
	statusParts = append(statusParts, fmt.Sprintf("%d items", totalItems))

	// Filter indicators
	if m.filterMyOnly {
		statusParts = append(statusParts, "@me")
	}
	if m.filterText != "" {
		statusParts = append(statusParts, fmt.Sprintf("/%s", m.filterText))
	}

	// Help hint
	statusParts = append(statusParts, "[a]@me [?]help")

	status := strings.Join(statusParts, " | ")

	// Calculate padding to right-align status
	leftLen := len(title)
	rightLen := len(status)
	padding := width - leftLen - rightLen - 2 // 2 for some breathing room
	if padding < 1 {
		padding = 1
	}

	// Build header line
	return titleStyle.Render(title) + strings.Repeat(" ", padding) + dimStyle.Render(status)
}

// renderBoard renders the kanban columns within the given dimensions
// Implements horizontal scrolling (carousel) when columns overflow
func (m BoardModel) renderBoard(totalWidth, totalHeight int) string {
	numCols := len(m.columns)
	if numCols == 0 {
		return ""
	}

	// totalHeight is the total lines available for the board (columns with borders)
	// lipgloss Border adds 2 lines (top + bottom) to the content height
	// So: totalHeight = contentHeight + 2
	// Therefore: contentHeight = totalHeight - 2
	colContentHeight := totalHeight - 2
	if colContentHeight < 3 {
		colContentHeight = 3
	}

	// Calculate how many columns can fit at minimum width
	maxVisibleCols := totalWidth / minColumnWidth
	if maxVisibleCols < 1 {
		maxVisibleCols = 1
	}

	// How many columns will we actually show?
	visibleCols := maxVisibleCols
	if visibleCols > numCols {
		visibleCols = numCols
	}

	// Calculate column width to fill available space evenly
	colWidth := totalWidth / visibleCols
	if colWidth > maxColumnWidth {
		colWidth = maxColumnWidth
	}
	if colWidth < minColumnWidth {
		colWidth = minColumnWidth
	}

	// Content width inside column (minus border and padding: 2 border + 2 padding = 4)
	innerWidth := colWidth - 4
	if innerWidth < 10 {
		innerWidth = 10
	}

	// Calculate how many card lines fit inside the column
	// Reserve: 1 line for header, potentially 2 for scroll indicators (up/down)
	maxCardLines := colContentHeight - 1 // Just header
	if maxCardLines < 1 {
		maxCardLines = 1
	}

	// Determine visible column range based on columnOffset
	startCol := m.columnOffset
	endCol := startCol + visibleCols
	if endCol > numCols {
		endCol = numCols
		startCol = endCol - visibleCols
		if startCol < 0 {
			startCol = 0
		}
	}

	// Build only visible columns
	columnViews := make([]string, 0, visibleCols)

	// Left scroll indicator if there are hidden columns to the left
	if startCol > 0 {
		indicator := lipgloss.NewStyle().
			Width(2).
			Height(colContentHeight+2).
			Foreground(lipgloss.Color("205")).
			Align(lipgloss.Center, lipgloss.Center).
			Render("◀")
		columnViews = append(columnViews, indicator)
	}

	for i := startCol; i < endCol; i++ {
		colID := m.columns[i]
		isSelected := i == m.selectedColumn
		columnViews = append(columnViews, m.renderColumn(colID, isSelected, colWidth, colContentHeight, innerWidth, maxCardLines, i+1))
	}

	// Right scroll indicator if there are hidden columns to the right
	if endCol < numCols {
		indicator := lipgloss.NewStyle().
			Width(2).
			Height(colContentHeight+2).
			Foreground(lipgloss.Color("205")).
			Align(lipgloss.Center, lipgloss.Center).
			Render("▶")
		columnViews = append(columnViews, indicator)
	}

	// Join horizontally with alignment at top
	return lipgloss.JoinHorizontal(lipgloss.Top, columnViews...)
}

// renderColumn renders a single column with proper sizing
// height is the inner height (content area, not including border)
// maxCardLines is the max lines available for cards (excluding header)
func (m BoardModel) renderColumn(colID string, selected bool, width, innerHeight, innerWidth, maxCardLines, colNum int) string {
	cards := m.filteredCards[colID]
	name := m.columnNames[colID]

	// Header: [N] Name (count)
	headerText := fmt.Sprintf("[%d] %s (%d)", colNum, name, len(cards))
	if len(headerText) > innerWidth {
		headerText = headerText[:innerWidth-1] + "…"
	}

	// Get scroll state
	scrollOffset := m.scrollOffset[colID]
	selectedIdx := m.selectedCard[colID]

	// Calculate how many card slots we have
	// maxCardLines is total lines minus header (1 line)
	cardSlots := maxCardLines - 1 // -1 for header line
	if cardSlots < 1 {
		cardSlots = 1
	}

	// Check if we need scroll indicators
	needUpIndicator := scrollOffset > 0
	needDownIndicator := false

	// Adjust card slots for indicators
	availableSlots := cardSlots
	if needUpIndicator {
		availableSlots--
	}

	// Calculate how many cards we can show
	endIdx := scrollOffset + availableSlots
	if endIdx > len(cards) {
		endIdx = len(cards)
	}

	// Check if we need down indicator
	if endIdx < len(cards) {
		needDownIndicator = true
		availableSlots--
		endIdx = scrollOffset + availableSlots
		if endIdx > len(cards) {
			endIdx = len(cards)
		}
	}

	// Build column content with exact line count
	var lines []string

	// Line 1: Header
	lines = append(lines, columnHeaderStyle.Render(headerText))

	// Scroll up indicator
	if needUpIndicator {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("↑ %d more", scrollOffset)))
	}

	// Render visible cards
	for i := scrollOffset; i < endIdx; i++ {
		cardID := cards[i]
		card, err := m.store.GetCard(cardID)
		if err != nil {
			continue
		}

		cardText := m.formatCardText(card, innerWidth-3) // 3 for "> " or "  " prefix
		if selected && i == selectedIdx {
			lines = append(lines, selectedCardStyle.Render("> "+cardText))
		} else {
			lines = append(lines, cardStyle.Render("  "+cardText))
		}
	}

	// Scroll down indicator
	remaining := len(cards) - endIdx
	if needDownIndicator && remaining > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("↓ %d more", remaining)))
	}

	// Empty column placeholder
	if len(cards) == 0 {
		lines = append(lines, dimStyle.Render("(empty)"))
	}

	// Join lines with newlines
	content := strings.Join(lines, "\n")

	// Create column style - the height here is for the CONTENT area inside the border
	borderColor := lipgloss.Color("240")
	if selected {
		borderColor = lipgloss.Color("205")
	}

	// Width includes border (2) + padding (2) = content width + 4
	// Height(innerHeight) sets content height, border adds 2 more lines
	// DO NOT use MaxHeight - it truncates the border!
	colStyle := lipgloss.NewStyle().
		Width(width-2).      // Subtract border width
		Height(innerHeight). // Inner content height (border adds 2 to total)
		Padding(0, 1).       // 1 char padding left/right
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	return colStyle.Render(content)
}

// formatCardText formats a card for display with max width
// Right-aligns the issue ID/suffix
func (m BoardModel) formatCardText(card *domain.Card, maxWidth int) string {
	title := card.Title

	// Determine suffix (issue number or type indicator)
	suffix := ""
	switch card.ContentType {
	case domain.ContentTypeIssue, domain.ContentTypePullRequest:
		if card.Number > 0 {
			suffix = fmt.Sprintf("#%d", card.Number)
		}
	case domain.ContentTypeDraftIssue:
		suffix = "(draft)"
	case domain.ContentTypePrivate:
		suffix = "(pvt)"
	}

	suffixLen := len(suffix)
	if suffixLen == 0 {
		// No suffix, just truncate title
		if len(title) > maxWidth {
			title = title[:maxWidth-1] + "…"
		}
		return title
	}

	// Calculate available space for title (leave room for suffix + 1 space gap)
	availableForTitle := maxWidth - suffixLen - 1
	if availableForTitle < 5 {
		availableForTitle = 5
	}

	// Truncate title if needed
	if len(title) > availableForTitle {
		title = title[:availableForTitle-1] + "…"
	}

	// Calculate padding to right-align suffix
	titleLen := len(title)
	padding := maxWidth - titleLen - suffixLen
	if padding < 1 {
		padding = 1
	}

	return title + strings.Repeat(" ", padding) + dimStyle.Render(suffix)
}

// rebuildColumns rebuilds column structure from store
func (m *BoardModel) rebuildColumns() {
	groupField := m.store.GetGroupField()
	if groupField == nil {
		return
	}

	m.columns = make([]string, 0, len(groupField.Options)+1)
	m.columnNames = make(map[string]string)

	for _, opt := range groupField.Options {
		m.columns = append(m.columns, opt.ID)
		m.columnNames[opt.ID] = opt.Name
	}

	// Add "No Status" column
	m.columns = append(m.columns, store.NoStatusKey)
	m.columnNames[store.NoStatusKey] = "No Status"

	// Ensure selected column is valid
	if m.selectedColumn >= len(m.columns) {
		m.selectedColumn = 0
	}
}

// applyFilter filters cards and groups them by column
func (m *BoardModel) applyFilter() {
	storeColumns, err := m.store.GetColumns()
	if err != nil {
		storeColumns = make(map[string][]string)
	}

	m.filteredCards = make(map[string][]string)

	// Initialize all columns
	for _, colID := range m.columns {
		m.filteredCards[colID] = []string{}
	}

	// Get current user login for "my items" filter
	viewerLogin := m.store.GetViewerLogin()

	// Populate with filtered cards
	for colID, cardIDs := range storeColumns {
		filtered := make([]string, 0)
		for _, itemID := range cardIDs {
			card, err := m.store.GetCard(itemID)
			if err != nil {
				continue
			}

			// Text filter
			if m.filterText != "" && !strings.Contains(strings.ToLower(card.Title), strings.ToLower(m.filterText)) {
				continue
			}

			// "Assigned to me" filter
			if m.filterMyOnly && viewerLogin != "" {
				isAssignedToMe := false
				for _, assignee := range card.Assignees {
					if strings.EqualFold(assignee, viewerLogin) {
						isAssignedToMe = true
						break
					}
				}
				if !isAssignedToMe {
					continue
				}
			}

			filtered = append(filtered, itemID)
		}
		m.filteredCards[colID] = filtered
	}

	// Reset scroll offsets and selection when filter changes
	// to avoid showing "↑ N more" when results fit on screen
	for colID := range m.filteredCards {
		m.scrollOffset[colID] = 0
		// Clamp selected card to valid range
		if m.selectedCard[colID] >= len(m.filteredCards[colID]) {
			if len(m.filteredCards[colID]) > 0 {
				m.selectedCard[colID] = len(m.filteredCards[colID]) - 1
			} else {
				m.selectedCard[colID] = 0
			}
		}
	}
}

// moveCardSelection moves the card selection up or down by delta
func (m *BoardModel) moveCardSelection(delta int) {
	if len(m.columns) == 0 {
		return
	}

	colID := m.columns[m.selectedColumn]
	cards := m.filteredCards[colID]
	if len(cards) == 0 {
		return
	}

	currentIdx := m.selectedCard[colID]
	newIdx := currentIdx + delta

	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(cards) {
		newIdx = len(cards) - 1
	}

	m.selectedCard[colID] = newIdx
	m.adjustScroll(colID)
}

// jumpToCard jumps to a specific card index. Use -1 to jump to last card.
func (m *BoardModel) jumpToCard(idx int) {
	if len(m.columns) == 0 {
		return
	}

	colID := m.columns[m.selectedColumn]
	cards := m.filteredCards[colID]
	if len(cards) == 0 {
		return
	}

	if idx < 0 {
		// -1 means last card
		idx = len(cards) - 1
	}
	if idx >= len(cards) {
		idx = len(cards) - 1
	}

	m.selectedCard[colID] = idx
	m.adjustScroll(colID)
}

// adjustScroll ensures the selected card is visible
func (m *BoardModel) adjustScroll(colID string) {
	selectedIdx := m.selectedCard[colID]
	scrollOffset := m.scrollOffset[colID]

	// Calculate visible cards based on current dimensions
	contentHeight := m.height - headerLines - 2 // 2 for column borders
	if m.moveMode {
		contentHeight--
	}
	if m.filterMode {
		contentHeight--
	}
	cardsHeight := contentHeight - 3 // header + potential scroll indicators
	if cardsHeight < 3 {
		cardsHeight = 3
	}

	visibleCards := cardsHeight

	// Scroll up if needed
	if selectedIdx < scrollOffset {
		m.scrollOffset[colID] = selectedIdx
	}

	// Scroll down if needed
	if selectedIdx >= scrollOffset+visibleCards {
		m.scrollOffset[colID] = selectedIdx - visibleCards + 1
	}
}

// adjustColumnScroll ensures the selected column is visible (horizontal carousel)
func (m *BoardModel) adjustColumnScroll() {
	if len(m.columns) == 0 || m.width == 0 {
		return
	}

	// Calculate how many columns fit on screen (same logic as renderBoard)
	visibleCols := m.width / minColumnWidth
	if visibleCols < 1 {
		visibleCols = 1
	}
	if visibleCols > len(m.columns) {
		visibleCols = len(m.columns)
	}

	// Scroll left if selected column is before visible range
	if m.selectedColumn < m.columnOffset {
		m.columnOffset = m.selectedColumn
	}

	// Scroll right if selected column is after visible range
	if m.selectedColumn >= m.columnOffset+visibleCols {
		m.columnOffset = m.selectedColumn - visibleCols + 1
	}
}

// getSelectedCard returns the currently selected card
func (m BoardModel) getSelectedCard() *domain.Card {
	if len(m.columns) == 0 {
		return nil
	}

	colID := m.columns[m.selectedColumn]
	cards := m.filteredCards[colID]
	if len(cards) == 0 {
		return nil
	}

	cardIdx := m.selectedCard[colID]
	if cardIdx >= len(cards) {
		cardIdx = 0
	}

	card, err := m.store.GetCard(cards[cardIdx])
	if err != nil {
		return nil
	}

	return card
}

// moveCardToColumn moves the selected card to a target column
func (m BoardModel) moveCardToColumn(targetColID string) tea.Cmd {
	card := m.getSelectedCard()
	if card == nil {
		return nil
	}

	newOptionID := targetColID
	if targetColID == store.NoStatusKey {
		newOptionID = ""
	}

	// Optimistic update
	err := m.store.MoveCard(card.ItemID, newOptionID)
	if err != nil {
		return func() tea.Msg { return moveErrorMsg{err: err} }
	}

	// Send mutation to API
	return func() tea.Msg {
		project := m.store.GetProject()
		groupField := m.store.GetGroupField()
		if project == nil || groupField == nil {
			return moveErrorMsg{err: fmt.Errorf("missing project or field")}
		}

		err := m.client.UpdateItemField(m.ctx, project.ID, card.ItemID, groupField.ID, newOptionID)
		if err != nil {
			return moveErrorMsg{err: err}
		}
		return moveSuccessMsg{}
	}
}

// loadNextPage fetches the next page of items (for lazy loading)
func (m BoardModel) loadNextPage(cursor string) tea.Cmd {
	return func() tea.Msg {
		project := m.store.GetProject()
		groupField := m.store.GetGroupField()
		if project == nil || groupField == nil {
			return pageLoadedMsg{err: fmt.Errorf("missing project or field")}
		}

		cards, nextCursor, hasMore, err := m.client.GetItems(m.ctx, project.ID, groupField.Name, cursor, 100)
		if err != nil {
			return pageLoadedMsg{err: err}
		}

		cardPtrs := make([]*domain.Card, len(cards))
		for i := range cards {
			cardPtrs[i] = &cards[i]
		}

		return pageLoadedMsg{
			cards:      cardPtrs,
			nextCursor: nextCursor,
			hasMore:    hasMore,
		}
	}
}

// loadAllItems fetches ALL items from GitHub (blocking - used for refresh)
func (m BoardModel) loadAllItems() tea.Cmd {
	return func() tea.Msg {
		project := m.store.GetProject()
		groupField := m.store.GetGroupField()
		if project == nil || groupField == nil {
			return itemsErrorMsg{err: fmt.Errorf("missing project or field")}
		}

		m.store.Clear()

		var allCards []*domain.Card
		cursor := ""
		pageSize := 100

		// Keep loading until we have all items
		for {
			cards, nextCursor, hasMore, err := m.client.GetItems(m.ctx, project.ID, groupField.Name, cursor, pageSize)
			if err != nil {
				return itemsErrorMsg{err: err}
			}

			for i := range cards {
				allCards = append(allCards, &cards[i])
			}

			if !hasMore || nextCursor == "" {
				break
			}
			cursor = nextCursor
		}

		m.store.UpsertCards(allCards)
		m.store.SetPagination("", false)

		return itemsLoadedMsg{}
	}
}

// Message types
type (
	itemsLoadedMsg      struct{}
	itemsErrorMsg       struct{ err error }
	moveSuccessMsg      struct{}
	moveErrorMsg        struct{ err error }
	changeGroupFieldMsg struct{}
	openDetailMsg       struct{ card *domain.Card }
	pageLoadedMsg       struct {
		cards      []*domain.Card
		nextCursor string
		hasMore    bool
		err        error
	}
)

// renderCard is kept for test compatibility
func (m BoardModel) renderCard(card *domain.Card) string {
	return m.formatCardText(card, 30)
}

// renderAllColumns is kept for test compatibility
func (m BoardModel) renderAllColumns() string {
	return m.renderBoard(m.width, m.height-headerLines)
}
