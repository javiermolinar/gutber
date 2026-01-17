package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"

	"github.com/mattn/go-runewidth"
)

const (
	pageLineCount  = 25
	pageLineWidth  = 80
	paragraphBreak = "\n\n"
)

type Chapter struct {
	Title     string
	Text      string
	StartPage int
}

type Book struct {
	Title    string
	Chapters []Chapter
	Pages    []string
}

type State struct {
	CurrentBook string         `json:"current_book,omitempty"`
	Pages       map[string]int `json:"pages,omitempty"`
	Page        int            `json:"page"`
}

type Config struct {
	BooksDir  string
	StateFile string
}

type bookResult struct {
	Title    string
	URL      string
	Subtitle string
	Extra    string
}

func fetchBooks(query string) ([]bookResult, error) {
	searchURL := "https://www.gutenberg.org/ebooks/search/?query=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gutberg-cli/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	root, err := xhtml.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	var books []bookResult
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "a" && hasClass(n, "link") {
			if href, ok := attr(n, "href"); ok && strings.HasPrefix(href, "/ebooks/") {
				title := findSpanText(n, "title")
				if title == "" {
					return
				}
				books = append(books, bookResult{
					Title:    strings.TrimSpace(title),
					Subtitle: strings.TrimSpace(findSpanText(n, "subtitle")),
					Extra:    strings.TrimSpace(findSpanText(n, "extra")),
					URL:      "https://www.gutenberg.org" + href,
				})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	return books, nil
}

func findSpanText(n *xhtml.Node, class string) string {
	var out string
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "span" && hasClass(node, class) {
			out = textContent(node)
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if out != "" {
				return
			}
		}
	}
	walk(n)
	return out
}

func downloadBookHTML(idOrURL, author, title, outDir string) (string, error) {
	ebookURL := normalizeEbookURL(idOrURL)
	req, err := http.NewRequest(http.MethodGet, ebookURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gutberg-cli/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	root, err := xhtml.Parse(resp.Body)
	if err != nil {
		return "", err
	}

	readNowURL := findReadNowURL(root)
	if readNowURL == "" {
		return "", fmt.Errorf("read online link not found")
	}

	fullURL := "https://www.gutenberg.org" + readNowURL
	req, err = http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gutberg-cli/1.0")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	fileName := buildBookFileName(author, title, readNowURL)
	if fileName == "" {
		fileName = "book.html"
	}
	outPath := filepath.Join(outDir, fileName)
	outFile, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return "", err
	}

	return outPath, nil
}

func normalizeEbookURL(idOrURL string) string {
	if strings.HasPrefix(idOrURL, "http://") || strings.HasPrefix(idOrURL, "https://") {
		return idOrURL
	}
	idOrURL = strings.TrimSpace(idOrURL)
	if strings.HasPrefix(idOrURL, "/ebooks/") {
		return "https://www.gutenberg.org" + idOrURL
	}
	return "https://www.gutenberg.org/ebooks/" + idOrURL
}

func findReadNowURL(root *xhtml.Node) string {
	var href string
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "a" {
			titleVal, _ := attr(n, "title")
			text := strings.TrimSpace(textContent(n))
			hrefVal, _ := attr(n, "href")
			if strings.Contains(strings.ToLower(titleVal), "read online") {
				if isReadableHTML(hrefVal) {
					href = hrefVal
					return
				}
			}
			if strings.EqualFold(text, "Read now!") || strings.EqualFold(text, "Read now") || strings.EqualFold(text, "Read online") {
				if isReadableHTML(hrefVal) {
					href = hrefVal
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if href != "" {
				return
			}
		}
	}
	walk(root)
	return href
}

func fileNameFromURL(href string) string {
	parts := strings.Split(strings.TrimRight(href, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func buildBookFileName(author, title, href string) string {
	author = sanitizeFilename(author)
	title = sanitizeFilename(title)
	if author != "" && title != "" {
		return fmt.Sprintf("%s-%s.html", author, title)
	}
	if title != "" {
		return title + ".html"
	}
	return fileNameFromURL(href)
}

func sanitizeFilename(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	name := b.String()
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "_")
	return name
}

func isReadableHTML(href string) bool {
	if href == "" {
		return false
	}
	if strings.Contains(href, "/cache/epub/") {
		return true
	}
	if strings.HasSuffix(href, ".html") || strings.HasSuffix(href, ".html.images") {
		return true
	}
	return false
}

func attr(n *xhtml.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

func hasClass(n *xhtml.Node, class string) bool {
	value, ok := attr(n, "class")
	if !ok {
		return false
	}
	for _, part := range strings.Fields(value) {
		if part == class {
			return true
		}
	}
	return false
}

func textContent(n *xhtml.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func loadBookFromHTML(path string, width, lines int) (Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Book{}, err
	}

	title := extractTitle(data)
	if title == "" {
		title = "Untitled"
	}

	chapters := extractChaptersFromHTML(data)
	if len(chapters) == 0 {
		text := cleanHTMLToText(string(data))
		chapters = []Chapter{{Title: title, Text: text, StartPage: 0}}
	}
	pages, chapters := buildBookPagesForSize(Book{Title: title, Chapters: chapters}, width, lines)

	return Book{Title: title, Chapters: chapters, Pages: pages}, nil
}

func extractTitle(data []byte) string {
	re := regexp.MustCompile(`(?is)<title>(.*?)</title>`)
	m := re.FindSubmatch(data)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(stripTags(string(m[1]))))
}

func extractChaptersFromHTML(data []byte) []Chapter {
	re := regexp.MustCompile(`(?is)<h[1-3][^>]*>(.*?)</h[1-3]>`)
	matches := re.FindAllSubmatchIndex(data, -1)
	if len(matches) == 0 {
		return nil
	}

	chapters := make([]Chapter, 0, len(matches))
	for i, m := range matches {
		title := cleanInlineText(string(data[m[2]:m[3]]))
		start := m[1]
		end := len(data)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		chunk := string(data[start:end])
		text := cleanHTMLToText(chunk)
		if strings.TrimSpace(text) == "" {
			continue
		}
		chapters = append(chapters, Chapter{Title: title, Text: text})
	}
	if len(chapters) <= 1 {
		return nil
	}
	return chapters
}

func cleanInlineText(input string) string {
	text := stripTags(input)
	text = html.UnescapeString(text)
	return strings.TrimSpace(text)
}

func loadAuthorsFromEmbedded(data string) ([]string, error) {
	var authors []string
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			authors = append(authors, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return authors, nil
}

func buildBookPagesForSize(book Book, width, lines int) ([]string, []Chapter) {
	pages := []string{}
	chapters := book.Chapters
	if width < 20 {
		width = 20
	}
	if lines < 5 {
		lines = 5
	}
	for i := range chapters {
		chapters[i].StartPage = len(pages)
		header := fmt.Sprintf("%s\n\n", chapters[i].Title)
		text := strings.TrimSpace(header + chapters[i].Text)
		chapterPages := paginate(text, lines, width)
		pages = append(pages, chapterPages...)
	}
	return pages, chapters
}

func cleanHTMLToText(input string) string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")

	normalized = stripHTMLSection(normalized, `(?is)<style[^>]*>.*?</style>`)
	normalized = stripHTMLSection(normalized, `(?is)<div\\s+id=\"pg-header\".*?</div>`)
	normalized = stripHTMLSection(normalized, `(?is)<div\\s+id=\"pg-footer\".*?</div>`)

	normalized = replaceAllTag(normalized, "br", "\n")
	normalized = replaceAllTag(normalized, "/p", paragraphBreak)
	normalized = replaceAllTag(normalized, "p", "")
	normalized = replaceAllTag(normalized, "hr", "\n")

	text := stripTags(normalized)
	text = html.UnescapeString(text)
	text = normalizeWhitespace(text)
	text = stripGutenbergBoilerplate(text)
	return text
}

func stripHTMLSection(input, pattern string) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(input, "")
}

func stripGutenbergBoilerplate(text string) string {
	if text == "" {
		return text
	}

	startRe := regexp.MustCompile(`(?i)\\*\\*\\*\\s*START OF THE PROJECT GUTENBERG.*?\\*\\*\\*`)
	if loc := startRe.FindStringIndex(text); loc != nil {
		text = text[loc[1]:]
	}

	endRe := regexp.MustCompile(`(?i)\\*\\*\\*\\s*END OF THE PROJECT GUTENBERG.*?\\*\\*\\*`)
	if loc := endRe.FindStringIndex(text); loc != nil {
		text = text[:loc[0]]
	}

	headerRe := regexp.MustCompile(`(?m)^The Project Gutenberg eBook of.*$`)
	text = headerRe.ReplaceAllString(text, "")
	text = normalizeWhitespace(text)
	return text
}

func replaceAllTag(input, tag, replacement string) string {
	re := regexp.MustCompile(`(?i)<\s*` + regexp.QuoteMeta(tag) + `\b[^>]*>`)
	return re.ReplaceAllString(input, replacement)
}

func stripTags(input string) string {
	var b strings.Builder
	b.Grow(len(input))
	inTag := false
	for _, r := range input {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func normalizeWhitespace(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(compactSpaces(line))
	}
	output := strings.Join(lines, "\n")

	re := regexp.MustCompile(`\n{3,}`)
	output = re.ReplaceAllString(output, paragraphBreak)
	return strings.TrimSpace(output)
}

func compactSpaces(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func paginate(text string, linesPerPage, lineWidth int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	wrapped := wrapText(text, lineWidth)
	lines := strings.Split(wrapped, "\n")
	pages := []string{}
	for i := 0; i < len(lines); i += linesPerPage {
		end := i + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		page := strings.Join(lines[i:end], "\n")
		pages = append(pages, strings.TrimSpace(page))
	}
	return pages
}

func wrapText(text string, width int) string {
	parts := strings.Split(text, paragraphBreak)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, wrapParagraph(p, width))
	}
	return strings.Join(out, paragraphBreak)
}

func wrapParagraph(text string, width int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var b strings.Builder
	lineLen := 0
	for _, w := range words {
		wordWidth := runewidth.StringWidth(w)
		if lineLen == 0 {
			b.WriteString(w)
			lineLen = wordWidth
			continue
		}
		if lineLen+1+wordWidth > width {
			b.WriteByte('\n')
			b.WriteString(w)
			lineLen = wordWidth
			continue
		}
		b.WriteByte(' ')
		b.WriteString(w)
		lineLen += 1 + wordWidth
	}

	return b.String()
}

func loadState(path string) (State, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{Page: 0, Pages: make(map[string]int)}, nil
		}
		return State{}, err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return State{}, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if state.Pages == nil {
		state.Pages = make(map[string]int)
	}
	return state, nil
}

func loadConfig() (Config, error) {
	configDir, err := defaultConfigDir()
	if err != nil {
		return Config{}, err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return Config{}, err
	}

	defaultCfg := Config{
		BooksDir:  filepath.Join(configDir, "books"),
		StateFile: filepath.Join(configDir, "state.json"),
	}

	configPath := filepath.Join(configDir, "gutberg.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := writeConfig(configPath, defaultCfg); err != nil {
			return Config{}, err
		}
	} else if err == nil {
		loaded, err := readConfig(configPath)
		if err != nil {
			return Config{}, err
		}
		if loaded.BooksDir != "" {
			defaultCfg.BooksDir = loaded.BooksDir
		}
		if loaded.StateFile != "" {
			defaultCfg.StateFile = loaded.StateFile
		}
	}

	if err := os.MkdirAll(defaultCfg.BooksDir, 0o755); err != nil {
		return Config{}, err
	}
	return defaultCfg, nil
}

func defaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gutberg"), nil
}

func writeConfig(path string, cfg Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "books_dir = %q\nstate_file = %q\n", cfg.BooksDir, cfg.StateFile)
	return err
}

func readConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	var cfg Config
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"")
		switch key {
		case "books_dir":
			cfg.BooksDir = val
		case "state_file":
			cfg.StateFile = val
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func saveState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
