package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

/**
 * 设计规范说明 (基于 Terminal Logic 设计系统):
 * -----------------------------------------
 * 背景色: #12131d (Dark Navy)
 * 主色/成功色: #9ece6a (Neon Green)
 * 强调色: #7aa2f7 (Blue)
 * 辅助色: #bb9af7 (Purple)
 * 提示/警告: #e0af68 (Yellow/Orange)
 * 错误: #f7768e (Red)
 * 边框/分割线: #1a1b26 (Deep Slate)
 * 文本色: #a9b1d6 (Light Gray)
 */

var (
	// 基础色彩定义
	colorBg      = lipgloss.Color("#12131d")
	colorSurface = lipgloss.Color("#1a1b26")
	colorGreen   = lipgloss.Color("#9ece6a")
	colorBlue    = lipgloss.Color("#7aa2f7")
	colorPurple  = lipgloss.Color("#bb9af7")
	colorYellow  = lipgloss.Color("#e0af68")
	colorText    = lipgloss.Color("#a9b1d6")
	colorGray    = lipgloss.Color("#565f89")

	// 布局容器样式
	// 左侧导航栏: 固定宽度 24，带右边框
	sidebarStyle = lipgloss.NewStyle().
			Width(24).
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(colorSurface).
			Padding(0, 1)

	// 右侧状态栏: 固定宽度 30，带左边框
	infoPanelStyle = lipgloss.NewStyle().
			Width(30).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(colorSurface).
			Padding(0, 1)

	// 中间主内容区: 填充剩余空间
	mainViewStyle = lipgloss.NewStyle().
			Padding(0, 2)

	// 标题与文字样式
	titleStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	sectionTitleStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			Width(10)

	valueStyle = lipgloss.NewStyle().
			Foreground(colorText)

	// 导航项样式
	navItemStyle = lipgloss.NewStyle().
			Foreground(colorGray).
			PaddingLeft(2)

	activeNavItemStyle = navItemStyle.Copy().
				Foreground(colorBg).
				Background(colorGreen).
				Bold(true)

	// 状态栏容器样式: 全宽，反显
	statusBarStyle = lipgloss.NewStyle().
			Background(colorBlue).
			Foreground(colorBg).
			Bold(true)

	// 进度条样式定义 (用于右侧状态栏)
	progFull  = lipgloss.NewStyle().Foreground(colorGreen).Render("█")
	progEmpty = lipgloss.NewStyle().Foreground(colorSurface).Render("░")
)

type model struct {
	viewport   viewport.Model // 处理主区域滚动
	cpuProg    progress.Model // CPU 进度条
	memProg    progress.Model // 内存进度条
	width      int            // 终端宽度
	height     int            // 终端高度
	ready      bool           // 界面是否已准备好渲染
	cursor     int            // 侧边栏当前选中索引
}

// 侧边栏菜单项
var menuItems = []string{"CHATS", "HISTORY", "LOGS", "AGENTS"}

func initialModel() model {
	cp := progress.New(progress.WithDefaultGradient())
	cp.Width = 20
	mp := progress.New(progress.WithDefaultGradient())
	mp.Width = 20

	return model{
		cpuProg: cp,
		memProg: mp,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(menuItems)-1 {
				m.cursor++
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// 精确计算主视图区域
		// 侧边栏 (24) + 右侧面板 (30) + 边框与 Padding
		headerHeight := 3
		footerHeight := 1
		verticalMargin := headerHeight + footerHeight
		sidebarWidth := 24
		rightPanelWidth := 30

		if !m.ready {
			// 初始化视口 (Viewport)
			m.viewport = viewport.New(msg.Width-sidebarWidth-rightPanelWidth-6, msg.Height-verticalMargin)
			m.viewport.SetContent(m.renderMainContent())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - sidebarWidth - rightPanelWidth - 6
			m.viewport.Height = msg.Height - verticalMargin
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if !m.ready {
		return "\n  正在初始化系统界面..."
	}

	// 1. 组装左侧导航栏
	sidebarItems := []string{
		titleStyle.MarginBottom(1).Render("NAVIGATOR"),
	}
	for i, item := range menuItems {
		icon := " "
		if i == 0 { icon = "" } 
		label := fmt.Sprintf("%s %s", icon, item)
		
		if i == m.cursor {
			sidebarItems = append(sidebarItems, activeNavItemStyle.Render(label))
		} else {
			sidebarItems = append(sidebarItems, navItemStyle.Render(label))
		}
	}
	sidebarItems = append(sidebarItems, "\n", lipgloss.NewStyle().Foreground(colorGreen).Render("● SYSTEM ONLINE"))
	sidebar := sidebarStyle.Height(m.height - 1).Render(lipgloss.JoinVertical(lipgloss.Left, sidebarItems...))

	// 2. 组装右侧状态面板
	rightPanelItems := []string{
		sectionTitleStyle.Render("SYSTEM_RESOURCES"),
		renderMetric("CPU_USAGE", "42%", 0.42),
		renderMetric("MEM_ALLOC", "1.2GB / 4GB", 0.30),
		"\n",
		sectionTitleStyle.Render("ENVIRONMENT_CTX"),
		renderRow("OS", "LINUX_6.1"),
		renderRow("ARCH", "ARM64"),
		renderRow("LATENCY", "12ms"),
		"\n",
		sectionTitleStyle.Render("PROCESS_TREE"),
		lipgloss.NewStyle().Foreground(colorGray).Render("└─ agent_go [8821]\n   ├─ llm_bridge [8823]\n   └─ metrics [8824]"),
	}
	rightPanel := infoPanelStyle.Height(m.height - 1).Render(lipgloss.JoinVertical(lipgloss.Left, rightPanelItems...))

	// 3. 组装中间主视图
	header := titleStyle.Foreground(colorBlue).Render(fmt.Sprintf("AGENT_GO_v1.0 / %s", menuItems[m.cursor]))
	mainView := mainViewStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			header,
			"\n",
			m.viewport.View(),
		),
	)

	// 4. 水平拼接布局 (左侧栏 + 主内容 + 右侧栏)
	layout := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainView, rightPanel)

	// 5. 生成状态栏内容
	statusKeys := []string{"^C EXIT", "^N NEW", "^S SAVE", "^H HELP"}
	statusText := " " + strings.Join(statusKeys, "  ")
	w := m.width - lipgloss.Width(statusText)
	if w < 0 { w = 0 }
	statusBar := statusBarStyle.Width(m.width).Render(statusText + strings.Repeat(" ", w))

	// 6. 最终垂直拼接
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Height(m.height-1).Render(layout),
		statusBar,
	)
}

// 辅助函数: 渲染单行键值对
func renderRow(label, value string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render(label), valueStyle.Render(value))
}

// 辅助函数: 渲染带简易进度条的指标
func renderMetric(label, value string, percent float64) string {
	barWidth := 20
	full := int(percent * float64(barWidth))
	if full > barWidth { full = barWidth }
	
	bar := strings.Repeat(progFull, full) + strings.Repeat(progEmpty, barWidth-full)
	
	return lipgloss.JoinVertical(lipgloss.Left,
		renderRow(label, value),
		bar,
	)
}

func (m model) renderMainContent() string {
	userHeader := lipgloss.NewStyle().Foreground(colorPurple).Render("USER_ROOT 14:20:05")
	agentHeader := lipgloss.NewStyle().Foreground(colorGreen).Render("AGENT_GO 14:20:07")
	
	codeStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface).
		Padding(0, 1).
		Foreground(colorText)

	code := codeStyle.Render(
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, Agent\")\n}",
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		userHeader,
		"> 请帮我生成一个并发工作的 Worker Pool 示例代码。",
		"",
		agentHeader,
		"好的，我已经为您合成了一个基于 `sync.WaitGroup` 和 `channel` 的并发模式实现：",
		"\n",
		code,
		"\n",
		lipgloss.NewStyle().Foreground(colorGray).Italic(true).Render("-- 提示: 使用 J/K 或 方向键 切换菜单 --"),
	)
}

func main() {
	p := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),       
		tea.WithMouseCellMotion(), 
	)
	if _, err := p.Run(); err != nil {
		fmt.Printf("运行出错: %v", err)
		os.Exit(1)
	}
}

