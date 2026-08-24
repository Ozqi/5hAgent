package runtime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Ozqi/walle/internal/logger"
	"github.com/Ozqi/walle/internal/toolevent"
)

// processWorkLog 串行化一个 AgentProcess 的终端输出和 Markdown 工作日志。
// mu 保护文件句柄、assistant 段状态和控制台写入，避免 token 回调与工具事件并发交错。
type processWorkLog struct {
	mu              sync.Mutex
	console         bool
	dataDir         string
	agentName       string
	startedAt       time.Time
	file            *os.File
	path            string
	printedHeader   bool
	fileInAssistant bool
}

func newProcessWorkLog(console bool, dataDir string, agentName string, startedAt time.Time) *processWorkLog {
	if agentName == "" {
		agentName = "walle"
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return &processWorkLog{console: console, dataDir: dataDir, agentName: agentName, startedAt: startedAt.UTC()}
}

// Start 创建日志文件并输出进程头。
func (l *processWorkLog) Start(processID string, processName string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	// 文件日志失败只降级为告警，不阻止 Agent 主流程。
	if err := l.openFileLocked(processID); err != nil {
		logger.WarnTag("RUN", "open process work log: %v", err)
	}
	l.writeFileHeaderLocked(processID, processName)

	if l.console {
		fmt.Fprintf(os.Stdout, "process: %s", processID)
		if processName != "" && processName != processID {
			fmt.Fprintf(os.Stdout, " | %s", processName)
		}
		fmt.Fprintln(os.Stdout)
		if l.path != "" {
			fmt.Fprintf(os.Stdout, "worklog: %s\n", l.path)
		}
	}
}

// Stop 关闭日志文件；可重复调用。
func (l *processWorkLog) Stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopLocked()
}

// End 写入最终状态并释放日志资源。
func (l *processWorkLog) End(runErr error) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if runErr != nil {
		if l.console {
			if l.printedHeader {
				fmt.Fprintln(os.Stdout)
			}
			fmt.Fprintf(os.Stdout, "status: failed (%v)\n", runErr)
		}
		l.writeFileStringLocked(fmt.Sprintf("\n## Status %s\n\nfailed: %v\n", worklogTime(), runErr))
		l.stopLocked()
		return
	}
	if l.console {
		if l.printedHeader {
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprintln(os.Stdout, "status: completed")
	}
	l.writeFileStringLocked(fmt.Sprintf("\n## Status %s\n\ncompleted\n", worklogTime()))
	l.stopLocked()
}

// OnToken 把 assistant 输出增量同步到控制台和日志文件。
func (l *processWorkLog) OnToken(token string) {
	if l == nil || token == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.console {
		l.ensureAssistantHeaderLocked()
		fmt.Fprint(os.Stdout, token)
	}
	l.ensureFileAssistantHeaderLocked()
	l.writeFileStringLocked(token)
}

// OnReasoning 只建立输出段边界，不持久化 reasoning 内容。
func (l *processWorkLog) OnReasoning(token string) {
	if l == nil || strings.TrimSpace(token) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.console {
		l.ensureAssistantHeaderLocked()
	}
	l.ensureFileAssistantHeaderLocked()
}

func (l *processWorkLog) printToolEvent(event toolevent.ToolEvent) {
	if l == nil {
		return
	}
	text := strings.TrimRight(event.Text, "\n")
	if text == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.console && l.printedHeader {
		fmt.Fprintln(os.Stdout)
		l.printedHeader = false
	}
	if l.console {
		fmt.Fprintln(os.Stdout, text)
	}
	l.writeFileStringLocked(fmt.Sprintf("\n## Tool Event %s\n\n```text\n", worklogTime()))
	l.writeFileStringLocked(stripANSI(text))
	l.writeFileStringLocked("\n```\n")
}

func (l *processWorkLog) ensureAssistantHeaderLocked() {
	if l.printedHeader {
		return
	}
	fmt.Fprintln(os.Stdout, "\nassistant:")
	l.printedHeader = true
}

func (l *processWorkLog) openFileLocked(processID string) error {
	if l.dataDir == "" {
		return nil
	}
	dir := filepath.Join(l.dataDir, "agents", safeName(l.agentName), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create work log dir: %w", err)
	}
	name := l.startedAt.Format("20060102-150405") + "-" + safeName(processID) + ".md"
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		name = l.startedAt.Format("20060102-150405.000000000") + "-" + safeName(processID) + ".md"
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

func (l *processWorkLog) writeFileHeaderLocked(processID string, processName string) {
	if l.file == nil {
		return
	}
	fmt.Fprintf(l.file, "# Process Work Log\n\n")
	fmt.Fprintf(l.file, "- agent: %s\n", l.agentName)
	fmt.Fprintf(l.file, "- process: %s\n", processID)
	if processName != "" {
		fmt.Fprintf(l.file, "- name: %s\n", processName)
	}
	fmt.Fprintf(l.file, "- started_at: %s\n\n", l.startedAt.Format(time.RFC3339))
}

func (l *processWorkLog) ensureFileAssistantHeaderLocked() {
	if l.file == nil || l.fileInAssistant {
		return
	}
	l.writeFileStringLocked(fmt.Sprintf("## Assistant %s\n\n", worklogTime()))
	l.fileInAssistant = true
}

func (l *processWorkLog) writeFileStringLocked(text string) {
	if l.file == nil || text == "" {
		return
	}
	if _, err := io.WriteString(l.file, text); err != nil {
		logger.WarnTag("RUN", "write process work log: %v", err)
	}
}

func (l *processWorkLog) stopLocked() {
	if l.file == nil {
		return
	}
	if err := l.file.Close(); err != nil {
		logger.WarnTag("RUN", "close process work log: %v", err)
	}
	l.file = nil
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(text string) string {
	return ansiPattern.ReplaceAllString(text, "")
}

func worklogTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}
