package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lithammer/fuzzysearch/fuzzy"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/reflow/wordwrap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Version information - will be set at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type recipe struct {
	name        string
	description string
	similarity  int // For fuzzy sort
}

type model struct {
	categories     []string
	recipesByCat   map[string][]recipe
	allRecipes     []recipe
	currentTab     int
	selectedRecipe int
	showCode       bool
	dualView       bool
	ready          bool
	mainViewport   viewport.Model
	codeViewport   viewport.Model
	height         int
	width          int
	actualWidth    int
	tooSmall       bool
	showSearch     bool
	textInput      textinput.Model
	searchQuery    string
	filteredRecipe []recipe
}

var runRecipe string

var (
	bazzitePurple         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "5", Light: "5"})         // Bazzite purple/magenta (terminal color 5)
	bazziteWhite          = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#faebd7", Light: "240"}) // Bazzite white
	bazziteBlue           = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#acd7e6", Light: "39"})  // Bazzite light blue
	bazziteGrey           = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Dark: "#a8a8a8", Light: "236"}) // Bazzite dark grey
	horizontalBorderStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true)
	titleStyle            = func() lipgloss.Style {
		b := lipgloss.NormalBorder()
		b.Right = "├"
		b.Left = "┤"
		return lipgloss.NewStyle().Bold(true).Border(b).Inherit(bazziteWhite)
	}()
	catMiddle      = lipgloss.NewStyle().Align(lipgloss.Center).Width(28) // The category is 78 wide and divided to 28 in middle,
	catSide        = lipgloss.NewStyle().Width(21).Inherit(catMiddle)     // 21 in each of the side,
	catArrow       = lipgloss.NewStyle().Width(4).Inherit(catMiddle)      // and 4 for each arrow
	catSelected    = lipgloss.NewStyle().Bold(true).Inherit(bazziteBlue).Underline(true)
	selectedRecipe = lipgloss.NewStyle().Inherit(bazzitePurple).Bold(true)
	recipeActive   = lipgloss.NewStyle().Inherit(selectedRecipe)
	recipeInactive = lipgloss.NewStyle().Inherit(bazziteGrey)
	descText       = lipgloss.NewStyle().Inherit(bazziteWhite).Border(lipgloss.NormalBorder(), false, true, true).Height(6).Padding(0, 1)
	controlStyle   = lipgloss.NewStyle().Inherit(bazziteGrey).Italic(true).Inherit(horizontalBorderStyle)
)

func (m model) divider() string {
	return "├" + strings.Repeat("─", m.width-2) + "┤" // -2 to account for border
}

func initialModel() model {
	recipeDir := "/usr/share/ublue-os/just"
	files, _ := filepath.Glob(filepath.Join(recipeDir, "*.just"))

	categories := []string{}
	recipesByCat := make(map[string][]recipe)
	allRecipes := []recipe{}

	recipeRegex := regexp.MustCompile(`^\s*([a-zA-Z0-9_-]+)\s*:\s*.*`)
	commentRegex := regexp.MustCompile(`^\s*#\s*(.*)`)

	for _, file := range files {
		base := filepath.Base(file)
		if strings.Contains(base, "picker") {
			continue
		}
		category := cleanCategoryName(base)

		f, _ := os.Open(file)
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var lastComment string
		found := false
		for scanner.Scan() {
			line := scanner.Text()

			if commentRegex.MatchString(line) {
				lastComment = commentRegex.FindStringSubmatch(line)[1]
				continue
			}

			if matches := recipeRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				if strings.HasPrefix(name, "_") || strings.Contains(line, "alias") || strings.Contains(line, "[private]") {
					continue
				}
				recipe := recipe{
					name:        name,
					description: lastComment,
				}
				recipesByCat[category] = append(recipesByCat[category], recipe)
				allRecipes = append(allRecipes, recipe)

				lastComment = ""
				found = true
			}
		}
		if found {
			categories = append(categories, category)
		}
	}

	sort.Strings(categories)
	sort.Slice(allRecipes, func(i, j int) bool {
		return allRecipes[i].name < allRecipes[j].name
	})

	tuiWidth := 80
	ti := textinput.New()
	ti.Width = tuiWidth - 5 // -2 (border) + -2 (textinput padding) + -1 (textinput prompt)
	return model{
		categories:     categories,
		recipesByCat:   recipesByCat,
		allRecipes:     allRecipes,
		currentTab:     0,
		selectedRecipe: 0,
		showCode:       false,
		ready:          false,
		height:         0,
		width:          tuiWidth,
		actualWidth:    0,
		tooSmall:       false,
		showSearch:     false,
		textInput:      ti,
		searchQuery:    "",
	}
}

func cleanCategoryName(filename string) string {
	name := strings.TrimSuffix(filename, ".just")
	name = regexp.MustCompile(`^[0-9]+-`).ReplaceAllString(name, "")
	parts := strings.Split(name, "-")
	for i, part := range parts {
		parts[i] = cases.Title(language.English).String(part)
	}
	return strings.Join(parts, " ")
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if zone.Get("mainViewport").InBounds(msg) {
				m.changeRecipeScroll("up")
			}
		case tea.MouseButtonWheelDown:
			if zone.Get("mainViewport").InBounds(msg) {
				m.changeRecipeScroll("down")
			}
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionRelease {
				break
			}

			if zone.Get("textInput").InBounds(msg) {
				m.textInput.Focus()
			} else {
				m.textInput.Blur()
			}
			if zone.Get("leftTab").InBounds(msg) {
				m.changeTab("left")
			} else if zone.Get("rightTab").InBounds(msg) {
				m.changeTab("right")
			} else if zone.Get("backButton").InBounds(msg) {
				m.showCode = false
			} else {
				for i := range m.mainViewport.Height {
					actualIndex := m.mainViewport.YOffset + i
					if zone.Get("recipe" + fmt.Sprint(actualIndex)).InBounds(msg) {
						if actualIndex == m.selectedRecipe {
							return m.runRecipe()
						}
						m.selectedRecipe = actualIndex
					}
				}
			}
		}

		// Handle mouse movement in code viewport
		if zone.Get("codeViewport").InBounds(msg) {
			m.codeViewport, cmd = m.codeViewport.Update(msg)
			cmds = append(cmds, cmd)
		}

		m.updateModel()
	case tea.KeyMsg:
		switch msg.String() {
		case "left":
			m.changeTab("left")
		case "right":
			m.changeTab("right")
		case "up":
			m.changeRecipeScroll("up")
		case "down":
			m.changeRecipeScroll("down")
		case "enter":
			return m.runRecipe()
		case "c":
			if m.textInput.Focused() { // Don't trigger hotkey when textinput is focused
				break
			}
			m.showCode = !m.showCode
		case "s":
			if m.textInput.Focused() || (!m.dualView && m.showCode) { // Don't trigger hotkey when textinput is focused or when in background
				break
			}
			m.selectedRecipe = 0
			m.showSearch = !m.showSearch
			if m.showSearch {
				m.textInput.Focus()
			} else {
				m.textInput.Blur()
			}
			return m, nil
		case "esc", "q", "ctrl+c":
			if m.textInput.Focused() && msg.String() == "esc" {
				m.textInput.Blur()
				break
			}
			return m, tea.Quit
		}
		m.updateModel()
	case tea.WindowSizeMsg:
		// Update terminal size
		m.height = msg.Height
		m.actualWidth = msg.Width

		// Main Viewport
		mainHeaderHeight := lipgloss.Height(m.headerView())
		mainFooterHeight := lipgloss.Height(m.footerView())
		verticalMarginHeight := mainHeaderHeight + mainFooterHeight
		mainViewportHeight := msg.Height - verticalMarginHeight

		// Code Viewport
		codeHeaderHeight := lipgloss.Height(m.codeHeaderView())
		codeViewportHeight := msg.Height - codeHeaderHeight

		// Check minimal size
		if mainViewportHeight <= 3 || msg.Width < m.width {
			m.tooSmall = true
		} else {
			m.tooSmall = false
		}
		// For code viewer
		m.dualView = m.actualWidth >= m.width*2

		if !m.ready {
			// Main Viewport Init
			m.mainViewport = viewport.New(m.width, mainViewportHeight)
			m.mainViewport = viewport.New(msg.Width, mainViewportHeight)
			m.mainViewport.Style = lipgloss.NewStyle().Width(m.width).Border(lipgloss.NormalBorder(), false, true)
			m.mainViewport.YPosition = mainHeaderHeight

			// Code Viewport Init
			m.codeViewport = viewport.New(m.width, codeViewportHeight)
			m.codeViewport.Style = lipgloss.NewStyle().Width(m.width).Border(lipgloss.NormalBorder(), false, true, true)

			m.ready = true
		} else {
			m.mainViewport.Width = msg.Width
			m.mainViewport.Height = mainViewportHeight
			m.codeViewport.Height = codeViewportHeight
		}
	}
	if m.showSearch {
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
		if m.searchQuery != m.textInput.Value() {
			m.searchQuery = m.textInput.Value()
			m.selectedRecipe = 0
			m.updateFilteredRecipe()
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) headerView() string {
	var header strings.Builder
	title := titleStyle.Render("Available ujust recipes")
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-2)) // -2 to account for border
	header.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, "\n┌\n│", title, line, "\n┐\n│") + "\n")

	if m.showSearch {
		header.WriteString(zone.Mark("textInput", horizontalBorderStyle.Render(m.textInput.View())) + "\n")
	} else {
		left := ""
		if m.currentTab > 0 {
			left = m.categories[m.currentTab-1]
		}
		center := catSelected.Render(m.categories[m.currentTab])
		right := ""
		if m.currentTab < len(m.categories)-1 {
			right = m.categories[m.currentTab+1]
		}
		leftSide := zone.Mark("leftTab", catArrow.Render("← ")+catSide.Render(left))
		rightSide := zone.Mark("rightTab", catSide.Render(right)+catArrow.Render(" →"))
		navLine := horizontalBorderStyle.Render(leftSide + catMiddle.Render(center) + rightSide)
		header.WriteString(navLine + "\n")
	}

	header.WriteString(m.divider() + "\n")
	topControlText := "← → Change Category | ↑ ↓ Navigate Recipes"
	bottomControlText := "s: Toggle Search | c: Toggle Code | Enter: Select | Esc: Exit"
	if m.showSearch {
		topControlText = "↑ ↓ Navigate Recipes"
		if m.textInput.Focused() {
			bottomControlText = "s: Toggle Search | c: Toggle Code | Enter: Select | Esc: Unfocus Search"
		}
	}
	// 2 (border) for total of 2, same for the function below
	header.WriteString(controlStyle.Render(m.renderTextBlockCustom(topControlText, 2, lipgloss.Center)) + "\n")
	header.WriteString(controlStyle.Render(m.renderTextBlockCustom(bottomControlText, 2, lipgloss.Center)) + "\n")
	header.WriteString(m.divider())
	return header.String()
}

func (m model) footerView() string {
	var descBlock string
	r := recipe{}
	if len(m.currentRecipes()) > m.selectedRecipe {
		r = m.currentRecipes()[m.selectedRecipe]
	}
	// 2 (border) + 2 (padding) for total of 4, same for two function below
	selected := m.renderTextBlockCustom("Selected: "+selectedRecipe.Render(r.name), 4, lipgloss.Left)
	desc := ""
	if r.description != "" {
		desc = m.renderTextBlockCustom(r.description, 4, lipgloss.Left)
	}
	descBlock = descText.Render(selected + "\n\n" + desc)
	return m.divider() + "\n" + descBlock
}

func (m model) codeHeaderView() string {
	var header strings.Builder
	backButton := ""
	if !m.dualView {
		backButton = zone.Mark("backButton", "← ")
	}
	name := ""
	if len(m.currentRecipes()) > m.selectedRecipe {
		name = m.currentRecipes()[m.selectedRecipe].name
	}
	title := titleStyle.Render(backButton + name)
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-2)) // -2 to account for border
	header.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, "\n┌\n│", title, line, "\n┐\n│"))
	return header.String()
}

func (m model) View() string {
	if m.tooSmall {
		// -2 to account for border
		style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Align(lipgloss.Center, lipgloss.Center).Height(m.height - 2).Width(min(m.actualWidth, m.width) - 2)
		return style.Render("Your terminal size is too small.\nPlease resize the terminal window.")
	}

	recipeList := m.currentRecipes()
	var recipeLines []string
	for i, r := range recipeList {
		line := r.name
		if i == m.selectedRecipe {
			recipeLines = append(recipeLines, zone.Mark("recipe"+fmt.Sprint(i), recipeActive.Render("▶ "+line)))
		} else {
			recipeLines = append(recipeLines, zone.Mark("recipe"+fmt.Sprint(i), recipeInactive.Render("  "+line)))
		}
	}
	m.mainViewport.SetContent(strings.Join(recipeLines, "\n"))

	if !m.ready {
		return "\n  Initializing..."
	}

	mainView := fmt.Sprintf("%s\n%s\n%s", m.headerView(), zone.Mark("mainViewport", m.mainViewport.View()), m.footerView())
	codeView := fmt.Sprintf("%s\n%s", m.codeHeaderView(), zone.Mark("codeViewport", m.codeViewport.View()))

	finalView := mainView

	if m.showCode && m.dualView {
		finalView = lipgloss.JoinHorizontal(lipgloss.Center, mainView, codeView)
	} else if m.showCode {
		finalView = codeView
	}

	return zone.Scan(finalView)
}

func (m model) currentRecipes() []recipe {
	if m.showSearch {
		if m.searchQuery == "" {
			return m.allRecipes
		}
		return m.filteredRecipe
	}
	return m.recipesByCat[m.categories[m.currentTab]]
}

func (m *model) updateFilteredRecipe() {
	filteredRecipe := []recipe{}
	for _, r := range m.allRecipes {
		rank := fuzzy.RankMatchFold(m.searchQuery, r.name)
		if rank > -1 {
			r.similarity = rank
			filteredRecipe = append(filteredRecipe, r)
		}
	}
	slices.SortFunc(filteredRecipe, func(i, j recipe) int {
		return i.similarity - j.similarity
	})
	m.filteredRecipe = filteredRecipe
}

func wrap(s string, limit int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}

	var result strings.Builder
	line := ""

	for _, word := range words {
		if len(line)+len(word)+1 > limit {
			result.WriteString(line + "\n")
			line = word
		} else {
			if line != "" {
				line += " "
			}
			line += word
		}
	}

	result.WriteString(line)
	return result.String()
}

func (m *model) changeTab(direction string) {
	if !m.dualView && m.showCode { // Dont change tab when in background
		return
	}
	if direction == "left" {
		if m.currentTab > 0 {
			m.currentTab--
			m.selectedRecipe = 0
		}
	} else if direction == "right" {
		if m.currentTab < len(m.categories)-1 {
			m.currentTab++
			m.selectedRecipe = 0
		}
	}
}
func (m *model) changeRecipeScroll(direction string) {
	if !m.dualView && m.showCode { // Dont change recipe when in background
		return
	}
	if direction == "up" {
		if m.selectedRecipe > 0 {
			m.selectedRecipe--
		}
	} else if direction == "down" {
		if m.selectedRecipe < len(m.currentRecipes())-1 {
			m.selectedRecipe++
		}
	}
}

func (m model) runRecipe() (tea.Model, tea.Cmd) {
	selected := m.currentRecipes()[m.selectedRecipe]
	runRecipe = selected.name
	return m, tea.Quit
}

func (m *model) updateModel() {
	m.scrollViewport()
	m.fetchRecipeCode()
}

func (m *model) scrollViewport() {
	if m.mainViewport.Height < len(m.currentRecipes()) {
		visibleIndexStart := m.mainViewport.YOffset
		visibleIndexStop := visibleIndexStart + m.mainViewport.Height - 1
		if m.selectedRecipe < visibleIndexStart {
			m.mainViewport.YOffset--
		}
		if m.selectedRecipe > visibleIndexStop {
			m.mainViewport.YOffset++
		}
	}
}

func (m *model) fetchRecipeCode() {
	if !m.showCode {
		return
	}

	if m.selectedRecipe >= len(m.currentRecipes()) || m.selectedRecipe < 0 {
		return
	}

	selected := m.currentRecipes()[m.selectedRecipe]
	cmd := exec.Command("ujust", "-n", selected.name)
	output, err := cmd.CombinedOutput()

	if err != nil {
		m.codeViewport.SetContent(err.Error())
		return
	}

	m.codeViewport.SetContent(wordwrap.String(string(output), 78))
}

func (m model) renderTextBlockCustom(s string, padding int, position lipgloss.Position) string {
	width := m.width - padding // Account for border
	return lipgloss.PlaceHorizontal(width, position, wrap(s, width))
}

func main() {
	// Check for version flag
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version") {
		fmt.Printf("ujust-picker version %s, commit %s, built at %s\n", version, commit, date)
		return
	}

	// Init bubblezone
	zone.NewGlobal()

	p := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}

	if runRecipe != "" {
		fmt.Print("\033[2J\033[H")
		fmt.Printf("Running recipe: %s...\n\n", runRecipe)
		cmd := exec.Command("ujust", runRecipe)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}
}
