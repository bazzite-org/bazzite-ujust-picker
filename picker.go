package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

type model struct {
	categories     []string
	recipesByCat   map[string][]recipe
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
	debug          string
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
	catMiddle      = lipgloss.NewStyle().Align(lipgloss.Center).Width(28)
	catSide        = lipgloss.NewStyle().Width(21).Inherit(catMiddle)
	catArrow       = lipgloss.NewStyle().Width(4).Inherit(catMiddle)
	catSelected    = lipgloss.NewStyle().Bold(true).Inherit(bazziteBlue).Underline(true)
	selectedRecipe = lipgloss.NewStyle().Inherit(bazzitePurple).Bold(true)
	recipeActive   = lipgloss.NewStyle().Inherit(selectedRecipe)
	recipeInactive = lipgloss.NewStyle().Inherit(bazziteGrey)
	descText       = lipgloss.NewStyle().Inherit(bazziteWhite).Border(lipgloss.NormalBorder(), false, true, true).Height(6).Padding(0, 1)
	controlStyle   = lipgloss.NewStyle().Inherit(bazziteGrey).Italic(true).Inherit(horizontalBorderStyle)
)

func (m model) divider() string {
	return "├" + strings.Repeat("─", m.width-2) + "┤"
}

func initialModel() model {
	recipeDir := "/usr/share/ublue-os/just"
	files, _ := filepath.Glob(filepath.Join(recipeDir, "*.just"))

	categories := []string{}
	recipesByCat := make(map[string][]recipe)

	recipeRegex := regexp.MustCompile(`^\s*([a-zA-Z0-9_-]+)\s*:\s*.*`)
	commentRegex := regexp.MustCompile(`^\s*#\s*(.*)`)
	groupRegex := regexp.MustCompile(`^\s*\[group\(["']([^"']+)["']\)\]`)

	for _, file := range files {
		base := filepath.Base(file)
		if strings.Contains(base, "picker") {
			continue
		}

		f, _ := os.Open(file)
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var lastComment string
		var currentGroup string = "General" // Default group for recipes without [group]
		
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if groupMatches := groupRegex.FindStringSubmatch(line); groupMatches != nil {
				currentGroup = strings.TrimSpace(groupMatches[1])
				if currentGroup != "" {
					currentGroup = cases.Title(language.English).String(currentGroup)
				} else {
					currentGroup = "General" // Fallback for empty group names
				}
				continue
			}

			if commentRegex.MatchString(line) {
				lastComment = commentRegex.FindStringSubmatch(line)[1]
				continue
			}

			if matches := recipeRegex.FindStringSubmatch(line); matches != nil {
				name := matches[1]
				if strings.HasPrefix(name, "_") || strings.Contains(line, "alias") || strings.Contains(line, "[private]") {
					lastComment = ""
					continue
				}
				
				// Add recipe to its group
				recipesByCat[currentGroup] = append(recipesByCat[currentGroup], recipe{
					name:        name,
					description: lastComment,
				})
				
				// Add category to list if not already present
				categoryExists := false
				for _, cat := range categories {
					if cat == currentGroup {
						categoryExists = true
						break
					}
				}
				if !categoryExists {
					categories = append(categories, currentGroup)
				}
				
				lastComment = ""
			}
		}
	}

	sort.Strings(categories)
	return model{
		categories:     categories,
		recipesByCat:   recipesByCat,
		currentTab:     0,
		selectedRecipe: 0,
		showCode:       false,
		ready:          false,
		height:         0,
		width:          80,
		actualWidth:    0,
		tooSmall:       false,
		debug:          "",
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
			if msg.X < 80 {
				if m.showCode && !m.dualView {
					break // Dont scroll if single view and showing code
				}
				m.changeRecipeScroll("up")
			}
		case tea.MouseButtonWheelDown:
			if msg.X < 80 {
				if m.showCode && !m.dualView {
					break // Dont scroll if single view and showing code
				}
				m.changeRecipeScroll("down")
			}
		case tea.MouseButtonLeft:
			// m.debug = fmt.Sprintf("(X: %d, Y: %d) %s", msg.X, msg.Y, tea.MouseEvent(msg))
			if msg.Action != tea.MouseActionRelease {
				break
			}
			if msg.Y == 3 { // Tab Position
				if msg.X <= 23 { // Left Tab Position
					m.changeTab("left")
				} else if msg.X >= 53 && msg.X <= 80 { // Right Tab Position
					m.changeTab("right")
				}
			} else if msg.Y >= 8 && msg.X < 80 { // Recipe List Position
				clickedRecipeIndex := msg.Y - 8 + m.mainViewport.YOffset
				if clickedRecipeIndex == m.selectedRecipe {
					return m.runRecipe()
				}
				if clickedRecipeIndex < len(m.currentRecipes()) {
					m.selectedRecipe = clickedRecipeIndex
				}
			} else if m.showCode && !m.dualView {
				if msg.X == 2 && msg.Y == 1 { // Back Button
					m.showCode = false
				}
			}
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
			m.showCode = !m.showCode
		case "esc", "q", "ctrl+c":
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

	codeShowInSingleView := m.showCode && !m.dualView
	if msg, ok := msg.(tea.MouseMsg); ok && (msg.X > 80 && msg.X < 160) || codeShowInSingleView { // Only send command in code view
		m.codeViewport, cmd = m.codeViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) headerView() string {
	var header strings.Builder
	title := titleStyle.Render("Available ujust recipes")
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-2))
	header.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, "\n┌\n│", title, line, "\n┐\n│") + "\n")

	left := ""
	if m.currentTab > 0 {
		left = m.categories[m.currentTab-1]
	}
	center := catSelected.Render(m.categories[m.currentTab])
	right := ""
	if m.currentTab < len(m.categories)-1 {
		right = m.categories[m.currentTab+1]
	}

	navLine := horizontalBorderStyle.Render(catArrow.Render("← ") + catSide.Render(left) + catMiddle.Render(center) + catSide.Render(right) + catArrow.Render(" →"))
	header.WriteString(navLine + "\n")
	header.WriteString(m.divider() + "\n")
	topControlText := "← → Change Category | ↑ ↓ Navigate Recipes"
	bottomControlText := "c: Toggle Code | Enter: Select | Esc: Exit"
	header.WriteString(controlStyle.Render(m.renderTextBlockCustom(topControlText, 2, lipgloss.Center)) + "\n")
	header.WriteString(controlStyle.Render(m.renderTextBlockCustom(bottomControlText, 2, lipgloss.Center)) + "\n")
	header.WriteString(m.divider())
	return header.String()
}

func (m model) footerView() string {
	var descBlock string
	if len(m.currentRecipes()) > 0 {
		r := m.currentRecipes()[m.selectedRecipe]
		selected := m.renderTextBlockCustom("Selected: "+selectedRecipe.Render(r.name), 4, lipgloss.Left)
		desc := ""
		if r.description != "" {
			desc = m.renderTextBlockCustom(r.description, 4, lipgloss.Left)
			if m.debug != "" {
				desc = m.renderTextBlockCustom(m.debug, 4, lipgloss.Left)
			}
		}
		descBlock = descText.Render(selected + "\n\n" + desc)
	}
	return m.divider() + "\n" + descBlock
}

func (m model) codeHeaderView() string {
	var header strings.Builder
	backButton := ""
	if m.actualWidth < m.width*2 {
		backButton = "← "
	}
	title := titleStyle.Render(backButton + m.currentRecipes()[m.selectedRecipe].name)
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-2))
	header.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, "\n┌\n│", title, line, "\n┐\n│"))
	return header.String()
}

func (m model) View() string {
	if m.tooSmall {
		style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Align(lipgloss.Center, lipgloss.Center).Height(m.height - 2).Width(min(m.actualWidth, m.width) - 2)
		return style.Render("Your terminal size is too small.\nPlease resize the terminal window.")
	}

	var recipeLines []string
	for i, r := range m.currentRecipes() {
		line := r.name
		if i == m.selectedRecipe {
			recipeLines = append(recipeLines, recipeActive.Render("▶ "+line))
		} else {
			recipeLines = append(recipeLines, recipeInactive.Render("  "+line))
		}
	}
	m.mainViewport.SetContent(strings.Join(recipeLines, "\n"))

	if !m.ready {
		return "\n  Initializing..."
	}

	mainView := fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.mainViewport.View(), m.footerView())
	codeView := fmt.Sprintf("%s\n%s", m.codeHeaderView(), m.codeViewport.View())

	if m.showCode && m.actualWidth >= m.width*2 {
		return lipgloss.JoinHorizontal(lipgloss.Center, mainView, codeView)
	} else if m.showCode {
		return codeView
	}

	return mainView
}

func (m model) currentRecipes() []recipe {
	return m.recipesByCat[m.categories[m.currentTab]]
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
