// worklog.go - headless 工作日志输出
// 功能：在无头运行时把 assistant token 和工具事件输出到终端，并按 Agent 写入项目 .5hagent。
package runtime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lzq/5hAgent/internal/logger"
	"github.com/lzq/5hAgent/internal/task"
)

type headlessWorkLog struct {
	console         bool
	dataDir         string
	agentName       string
	startedAt       time.Time
	file            *os.File
	path            string
	printedHeader   bool
	fileInAssistant bool
}

func newHeadlessWorkLog(console bool, dataDir string, agentName string, startedAt time.Time) *headlessWorkLog {
	if agentName == "" {
		agentName = "5hAgent"
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return &headlessWorkLog{console: console, dataDir: dataDir, agentName: agentName, startedAt: startedAt.UTC()}
}

func (l *headlessWorkLog) Start(t *task.Task) {
	if l == nil {
		return
	}
	title := ""
	id := ""
	if t != nil {
		title = t.Title
		id = t.ID
	}

	if err := l.openFile(id); err != nil {
		logger.WarnTag("RUN", "open headless work log: %v", err)
	}
	l.writeFileHeader(id, title)

	if l.console {
		fmt.Fprintf(os.Stdout, "task: %s", id)
		if title != "" {
			fmt.Fprintf(os.Stdout, " | %s", title)
		}
		fmt.Fprintln(os.Stdout)
		if l.path != "" {
			fmt.Fprintf(os.Stdout, "worklog: %s\n", l.path)
		}
	}
	logger.SetToolEventSink(func(event logger.ToolEvent) {
		l.printToolEvent(event)
	})
}

func (l *headlessWorkLog) Stop() {
	if l == nil {
		return
	}
	logger.SetToolEventSink(nil)
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			logger.WarnTag("RUN", "close headless work log: %v", err)
		}
		l.file = nil
	}
}

func (l *headlessWorkLog) End(runErr error) {
	if l == nil {
		return
	}
	if runErr != nil {
		if l.console {
			if l.printedHeader {
				fmt.Fprintln(os.Stdout)
			}
			fmt.Fprintf(os.Stdout, "status: failed (%v)\n", runErr)
		}
		l.writeFileString(fmt.Sprintf("\n## Status\n\nfailed: %v\n", runErr))
		l.Stop()
		return
	}
	if l.console {
		if l.printedHeader {
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprintln(os.Stdout, "status: completed")
	}
	l.writeFileString("\n## Status\n\ncompleted\n")
	l.Stop()
}

func (l *headlessWorkLog) OnToken(token string) {
	if l == nil || token == "" {
		return
	}
	if l.console {
		l.ensureAssistantHeader()
		fmt.Fprint(os.Stdout, token)
	}
	l.ensureFileAssistantHeader()
	l.writeFileString(token)
}

func (l *headlessWorkLog) OnReasoning(token string) {
	if l == nil || strings.TrimSpace(token) == "" {
		return
	}
	if l.console {
		l.ensureAssistantHeader()
	}
	l.ensureFileAssistantHeader()
}

func (l *headlessWorkLog) printToolEvent(event logger.ToolEvent) {
	if l == nil {
		return
	}
	if l.console && l.printedHeader {
		fmt.Fprintln(os.Stdout)
		l.printedHeader = false
	}
	text := strings.TrimRight(event.Text, "\n")
	if text == "" {
		return
	}
	if l.console {
		fmt.Fprintln(os.Stdout, text)
	}
	l.writeFileString("\n## Tool Event\n\n```text\n")
	l.writeFileString(stripANSI(text))
	l.writeFileString("\n```\n")
}

func (l *headlessWorkLog) ensureAssistantHeader() {
	if l.printedHeader {
		return
	}
	fmt.Fprintln(os.Stdout, "\nassistant:")
	l.printedHeader = true
}

func (l *headlessWorkLog) openFile(taskID string) error {
	if l.dataDir == "" {
		return nil
	}
	dir := filepath.Join(l.dataDir, "agents", safeName(l.agentName), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create work log dir: %w", err)
	}
	name := l.startedAt.Format("20060102-150405") + "-" + safeName(taskID) + ".md"
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		name = l.startedAt.Format("20060102-150405.000000000") + "-" + safeName(taskID) + ".md"
		path = filepath.Join(dir, name)
		file, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	}
	if err != nil {
		return fmt.Errorf("create work log file: %w", err)
	}
	l.file = file
	l.path = path
	return nil
}

func (l *headlessWorkLog) writeFileHeader(taskID string, title string) {
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "# Headless Work Log\n\n")
	fmt.Fprintf(l.file, "- agent: %s\n", l.agentName)
	fmt.Fprintf(l.file, "- task: %s\n", taskID)
	if title != "" {
		fmt.Fprintf(l.file, "- title: %s\n", title)
	}
	fmt.Fprintf(l.file, "- started_at: %s\n\n", l.startedAt.Format(time.RFC3339))
}

func (l *headlessWorkLog) ensureFileAssistantHeader() {
	if l.file == nil || l.fileInAssistant {
		return
	}
	l.writeFileString("## Assistant\n\n")
	l.fileInAssistant = true
}

func (l *headlessWorkLog) writeFileString(text string) {
	if l.file == nil || text == "" {
		return
	}
	if _, err := io.WriteString(l.file, text); err != nil {
		logger.WarnTag("RUN", "write headless work log: %v", err)
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}
