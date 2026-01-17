package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeAuthorSearch mode = iota
	modeLibrary
	modeBooks
	modeReader
	modeChapters
)

type authorItem struct {
	name string
}

func (a authorItem) Title() string       { return a.name }
func (a authorItem) Description() string { return "" }
func (a authorItem) FilterValue() string { return a.name }

type bookItem struct {
	title    string
	url      string
	subtitle string
	extra    string
}

func (b bookItem) Title() string { return b.title }
func (b bookItem) Description() string {
	parts := []string{}
	if b.subtitle != "" {
		parts = append(parts, b.subtitle)
	}
	if b.extra != "" {
		parts = append(parts, b.extra)
	}
	if b.url != "" {
		parts = append(parts, b.url)
	}
	return strings.Join(parts, " | ")
}
func (b bookItem) FilterValue() string { return b.title }

type libraryItem struct {
	title string
	path  string
}

func (l libraryItem) Title() string       { return l.title }
func (l libraryItem) Description() string { return l.path }
func (l libraryItem) FilterValue() string { return l.title }

type chapterItem struct {
	title string
	index int
}

func (c chapterItem) Title() string       { return c.title }
func (c chapterItem) Description() string { return "" }
func (c chapterItem) FilterValue() string { return c.title }

type errMsg struct{ err error }

type booksMsg struct {
	items []list.Item
	err   error
}

type bookLoadedMsg struct {
	book Book
	path string
	err  error
}

type model struct {
	mode         mode
	authorInput  textinput.Model
	authorList   list.Model
	authors      []string
	authorsLower []string
	libraryList  list.Model
	bookList     list.Model
	chapterList  list.Model
	currentBook  Book
	state        State
	config       Config
	status       string
	err          error
	width        int
	height       int
	pageWidth    int
	pageLines    int
	fontScale    int
}

func newModel(cfg Config, state State, authors []string) (model, error) {
	authorsLower := make([]string, len(authors))
	for i, name := range authors {
		authorsLower[i] = strings.ToLower(name)
	}

	authorInput := textinput.New()
	authorInput.Placeholder = "Author prefix (e.g. ab)"
	authorInput.Focus()
	authorInput.CharLimit = 80
	authorInput.Width = 40

	authorList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	authorList.Title = "Authors"
	authorList.SetFilteringEnabled(false)

	libraryItems, err := loadLibraryItems(cfg.BooksDir)
	if err != nil {
		return model{}, err
	}
	libraryList := list.New(libraryItems, list.NewDefaultDelegate(), 0, 0)
	libraryList.Title = "Library"
	libraryList.SetFilteringEnabled(true)

	bookList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	bookList.Title = "Books"
	bookList.SetFilteringEnabled(true)

	chapterList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	chapterList.Title = "Chapters"
	chapterList.SetFilteringEnabled(true)

	initialMode := modeAuthorSearch
	var currentBook Book
	if state.CurrentBook != "" {
		if _, err := os.Stat(state.CurrentBook); err == nil {
			book, err := loadBookFromHTML(state.CurrentBook, pageLineWidth, pageLineCount)
			if err == nil {
				currentBook = book
				state.Page = state.Pages[state.CurrentBook]
				initialMode = modeReader
			}
		}
	}
	if initialMode != modeReader && len(libraryItems) > 0 {
		initialMode = modeLibrary
	}
	if len(currentBook.Chapters) > 0 {
		chapterList.SetItems(buildChapterItems(currentBook))
	}

	m := model{
		mode:         initialMode,
		authorInput:  authorInput,
		authorList:   authorList,
		authors:      authors,
		authorsLower: authorsLower,
		libraryList:  libraryList,
		bookList:     bookList,
		chapterList:  chapterList,
		currentBook:  currentBook,
		state:        state,
		config:       cfg,
		pageWidth:    pageLineWidth,
		pageLines:    pageLineCount,
		fontScale:    0,
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case errMsg:
		m.err = msg.err
		m.status = msg.err.Error()
		return m, nil
	case booksMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
		}
		m.bookList.SetItems(msg.items)
		m.mode = modeBooks
		m.status = fmt.Sprintf("%d books", len(msg.items))
		return m, nil
	case bookLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.err.Error()
			return m, nil
		}
		m.currentBook = msg.book
		m.state.CurrentBook = msg.path
		m.state.Page = m.state.Pages[msg.path]
		m.mode = modeReader
		m.status = ""
		m.chapterList.SetItems(buildChapterItems(m.currentBook))
		items, _ := loadLibraryItems(m.config.BooksDir)
		m.libraryList.SetItems(items)
		return m, saveStateCmd(m.state, m.config.StateFile)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.authorList.SetSize(msg.Width, msg.Height)
		m.libraryList.SetSize(msg.Width, msg.Height)
		m.bookList.SetSize(msg.Width, msg.Height)
		m.chapterList.SetSize(msg.Width, msg.Height)
		pageWidth, pageLines := computePageLayout(msg.Width, msg.Height, m.fontScale)
		if pageWidth != m.pageWidth || pageLines != m.pageLines {
			oldTotal := len(m.currentBook.Pages)
			oldPage := m.state.Page
			m.pageWidth = pageWidth
			m.pageLines = pageLines
			if len(m.currentBook.Chapters) > 0 {
				m.currentBook.Pages, m.currentBook.Chapters = buildBookPagesForSize(m.currentBook, m.pageWidth, m.pageLines)
				if oldTotal > 0 && len(m.currentBook.Pages) > 0 {
					m.state.Page = remapPage(oldPage, oldTotal, len(m.currentBook.Pages))
				} else if len(m.currentBook.Pages) > 0 && m.state.Page >= len(m.currentBook.Pages) {
					m.state.Page = len(m.currentBook.Pages) - 1
				}
			}
			return m, saveStateCmd(m.state, m.config.StateFile)
		}
	}

	switch m.mode {
	case modeAuthorSearch:
		return m.updateAuthorSearch(msg)
	case modeLibrary:
		return m.updateLibrary(msg)
	case modeBooks:
		return m.updateBooks(msg)
	case modeReader:
		return m.updateReader(msg)
	case modeChapters:
		return m.updateChapters(msg)
	default:
		return m, nil
	}
}

func (m model) updateAuthorSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.authorInput.Value()
	var inputCmd tea.Cmd
	m.authorInput, inputCmd = m.authorInput.Update(msg)
	if m.authorInput.Value() != prev {
		m.authorList.SetItems(filterAuthors(m.authors, m.authorsLower, m.authorInput.Value(), 200))
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.authorList.SelectedItem().(authorItem); ok {
				m.status = "Searching books..."
				return m, fetchBooksCmd(item.name)
			}
			if strings.TrimSpace(m.authorInput.Value()) == "" {
				m.status = "Enter a prefix to search"
				return m, nil
			}
		case "b":
			m.mode = modeLibrary
			return m, nil
		case "esc", "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	var listCmd tea.Cmd
	m.authorList, listCmd = m.authorList.Update(msg)
	return m, tea.Batch(inputCmd, listCmd)
}

func (m model) updateLibrary(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.libraryList.SelectedItem().(libraryItem); ok {
				m.status = "Loading book..."
				return m, openBookCmd(item.path, m.pageWidth, m.pageLines)
			}
		case "s":
			m.mode = modeAuthorSearch
			m.authorInput.Focus()
			return m, nil
		case "b":
			if m.state.CurrentBook != "" && len(m.currentBook.Pages) > 0 {
				m.mode = modeReader
				return m, nil
			}
		case "c":
			if len(m.currentBook.Chapters) > 0 {
				m.mode = modeChapters
				return m, nil
			}
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.libraryList, cmd = m.libraryList.Update(msg)
	return m, cmd
}

func (m model) updateBooks(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.bookList.SelectedItem().(bookItem); ok {
				m.status = "Downloading book..."
				return m, downloadAndLoadCmd(item.url, item.subtitle, item.title, m.config.BooksDir, m.pageWidth, m.pageLines)
			}
		case "b":
			m.mode = modeLibrary
			return m, nil
		case "s":
			m.mode = modeAuthorSearch
			m.authorInput.Focus()
			return m, nil
		case "esc", "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.bookList, cmd = m.bookList.Update(msg)
	return m, cmd
}

func (m model) updateReader(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "b":
			m.mode = modeLibrary
			return m, nil
		case "s":
			m.mode = modeAuthorSearch
			m.authorInput.Focus()
			return m, nil
		case "c":
			if len(m.currentBook.Chapters) > 0 {
				m.mode = modeChapters
				return m, nil
			}
		case "+", "=":
			m.fontScale++
			m.applyFontScale()
			return m, saveStateCmd(m.state, m.config.StateFile)
		case "-":
			m.fontScale--
			m.applyFontScale()
			return m, saveStateCmd(m.state, m.config.StateFile)
		case "enter", " ", "right", "down", "pgdown":
			if m.state.Page < len(m.currentBook.Pages)-1 {
				m.state.Page++
				m.state.Pages[m.state.CurrentBook] = m.state.Page
				return m, saveStateCmd(m.state, m.config.StateFile)
			}
		case "left", "up", "pgup":
			if m.state.Page > 0 {
				m.state.Page--
				m.state.Pages[m.state.CurrentBook] = m.state.Page
				return m, saveStateCmd(m.state, m.config.StateFile)
			}
		case "home":
			m.state.Page = 0
			m.state.Pages[m.state.CurrentBook] = m.state.Page
			return m, saveStateCmd(m.state, m.config.StateFile)
		case "end":
			if len(m.currentBook.Pages) > 0 {
				m.state.Page = len(m.currentBook.Pages) - 1
				m.state.Pages[m.state.CurrentBook] = m.state.Page
				return m, saveStateCmd(m.state, m.config.StateFile)
			}
		}
	}
	return m, nil
}

func (m model) updateChapters(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.chapterList.SelectedItem().(chapterItem); ok {
				if item.index >= 0 && item.index < len(m.currentBook.Chapters) {
					m.state.Page = m.currentBook.Chapters[item.index].StartPage
					m.state.Pages[m.state.CurrentBook] = m.state.Page
					m.mode = modeReader
					return m, saveStateCmd(m.state, m.config.StateFile)
				}
			}
		case "b", "esc":
			m.mode = modeReader
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.chapterList, cmd = m.chapterList.Update(msg)
	return m, cmd
}

func (m model) View() string {
	switch m.mode {
	case modeAuthorSearch:
		return m.authorSearchView()
	case modeLibrary:
		return m.libraryView()
	case modeBooks:
		return m.bookListView()
	case modeReader:
		return m.readerView()
	case modeChapters:
		return m.chapterListView()
	default:
		return ""
	}
}

func (m model) authorSearchView() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Render("Gutenberg Reader")
	prompt := "Search authors by prefix"
	status := m.status
	if status == "" {
		status = "Type to filter, enter to select, b: library, q: quit"
	}
	listView := m.authorList.View()
	return strings.Join([]string{title, "", prompt, m.authorInput.View(), "", listView, "", status}, "\n")
}

func (m model) libraryView() string {
	return m.libraryList.View() + "\n" + helpLine("enter: open  s: search  c: chapters  b: back  q: quit")
}

func (m model) bookListView() string {
	return m.bookList.View() + "\n" + helpLine("enter: download/read  b: library  s: search  q: quit")
}

func (m model) chapterListView() string {
	return m.chapterList.View() + "\n" + helpLine("enter: open  b/esc: back  q: quit")
}

func (m model) readerView() string {
	if len(m.currentBook.Pages) == 0 {
		return "No pages available."
	}
	page := m.currentBook.Pages[m.state.Page]

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	header := titleStyle.Render(m.currentBook.Title)
	status := metaStyle.Render(fmt.Sprintf("Page %d/%d", m.state.Page+1, len(m.currentBook.Pages)))

	contentWidth := m.pageWidth
	if contentWidth == 0 {
		contentWidth = pageLineWidth
	}

	content := lipgloss.NewStyle().Width(contentWidth).Render(page)
	footer := footerStyle.Render("Enter/Espacio: next  pgup: prev  +/-: size  c: chapters  b: library  s: search  q: quit")

	return strings.Join([]string{header, status, "", content, "", footer}, "\n")
}

func helpLine(msg string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(msg)
}

func fetchBooksCmd(author string) tea.Cmd {
	return func() tea.Msg {
		books, err := fetchBooks(author)
		if err != nil {
			return booksMsg{err: err}
		}
		items := make([]list.Item, 0, len(books))
		for _, b := range books {
			items = append(items, bookItem{title: b.Title, url: b.URL, subtitle: b.Subtitle, extra: b.Extra})
		}
		return booksMsg{items: items}
	}
}

func downloadAndLoadCmd(bookURL, author, title, outDir string, width, lines int) tea.Cmd {
	return func() tea.Msg {
		path, err := downloadBookHTML(bookURL, author, title, outDir)
		if err != nil {
			return bookLoadedMsg{err: err}
		}
		book, err := loadBookFromHTML(path, width, lines)
		if err != nil {
			return bookLoadedMsg{err: err}
		}
		return bookLoadedMsg{book: book, path: path}
	}
}

func buildChapterItems(book Book) []list.Item {
	items := make([]list.Item, 0, len(book.Chapters))
	for i, ch := range book.Chapters {
		title := ch.Title
		if title == "" {
			title = fmt.Sprintf("Chapter %d", i+1)
		}
		items = append(items, chapterItem{title: fmt.Sprintf("%3d. %s", i+1, title), index: i})
	}
	return items
}

func openBookCmd(path string, width, lines int) tea.Cmd {
	return func() tea.Msg {
		book, err := loadBookFromHTML(path, width, lines)
		if err != nil {
			return bookLoadedMsg{err: err}
		}
		return bookLoadedMsg{book: book, path: path}
	}
}

func loadLibraryItems(dir string) ([]list.Item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".html.images") {
			continue
		}
		title := strings.TrimSuffix(name, ".html")
		title = strings.TrimSuffix(title, ".images")
		title = strings.ReplaceAll(title, "_", " ")
		items = append(items, libraryItem{
			title: title,
			path:  filepath.Join(dir, name),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].(libraryItem).title < items[j].(libraryItem).title
	})
	return items, nil
}

func filterAuthors(authors []string, authorsLower []string, prefix string, limit int) []list.Item {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" {
		return nil
	}
	start := sort.Search(len(authorsLower), func(i int) bool {
		return authorsLower[i] >= prefix
	})

	items := make([]list.Item, 0, limit)
	for i := start; i < len(authorsLower); i++ {
		if !strings.HasPrefix(authorsLower[i], prefix) {
			break
		}
		items = append(items, authorItem{name: authors[i]})
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items
}

func saveStateCmd(state State, path string) tea.Cmd {
	return func() tea.Msg {
		if err := saveState(path, state); err != nil {
			return errMsg{err: err}
		}
		return nil
	}
}

func (m *model) applyFontScale() {
	if m.fontScale > 5 {
		m.fontScale = 5
	}
	if m.fontScale < -5 {
		m.fontScale = -5
	}
	pageWidth, pageLines := computePageLayout(m.width, m.height, m.fontScale)
	if pageWidth != m.pageWidth || pageLines != m.pageLines {
		oldTotal := len(m.currentBook.Pages)
		oldPage := m.state.Page
		m.pageWidth = pageWidth
		m.pageLines = pageLines
		if len(m.currentBook.Chapters) > 0 {
			m.currentBook.Pages, m.currentBook.Chapters = buildBookPagesForSize(m.currentBook, m.pageWidth, m.pageLines)
			if oldTotal > 0 && len(m.currentBook.Pages) > 0 {
				m.state.Page = remapPage(oldPage, oldTotal, len(m.currentBook.Pages))
			} else if len(m.currentBook.Pages) > 0 && m.state.Page >= len(m.currentBook.Pages) {
				m.state.Page = len(m.currentBook.Pages) - 1
			}
		}
	}
}

func remapPage(oldPage, oldTotal, newTotal int) int {
	if oldTotal <= 0 || newTotal <= 0 {
		return 0
	}
	progress := float64(oldPage) / float64(oldTotal)
	newPage := int(progress * float64(newTotal))
	if newPage < 0 {
		newPage = 0
	}
	if newPage >= newTotal {
		newPage = newTotal - 1
	}
	return newPage
}

func computePageLayout(width, height, scale int) (int, int) {
	baseWidth := pageLineWidth
	baseLines := pageLineCount
	if width > 0 {
		baseWidth = width - 4
	}
	if height > 0 {
		baseLines = height - 8
	}
	pageWidth := baseWidth - (scale * 4)
	pageLines := baseLines - (scale * 2)
	if pageWidth < 40 {
		pageWidth = 40
	}
	if pageLines < 10 {
		pageLines = 10
	}
	return pageWidth, pageLines
}
