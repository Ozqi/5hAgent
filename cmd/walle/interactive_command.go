package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Ozqi/walle/internal/agentd"
	"github.com/Ozqi/walle/internal/utils"
)

// startInteractiveClient 启动脱离当前终端的 daemon，并等待其 Unix Socket 可接入。
func startInteractiveClient(ctx context.Context) (*agentd.ProcessClient, error) {
	// 1. 解析可执行文件和运行目录，优先复用已就绪的交互 daemon。
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil, err
	}
	runDir := filepath.Join(configDir, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, fmt.Errorf("create daemon run dir: %w", err)
	}
	openReq, err := interactiveOpenRequest()
	if err != nil {
		return nil, err
	}
	if client, err := agentd.OpenProcess(runDir, openReq); err == nil {
		return client, nil
	} else if !agentd.IsSupervisorUnavailable(err) {
		return nil, err
	}

	// 2. daemon 不存在时脱离当前终端启动，并把 stdout/stderr 追加到固定日志。
	logFile, err := os.OpenFile(filepath.Join(runDir, "interactive.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon log: %w", err)
	}
	args := interactiveDaemonArgs()
	process := exec.Command(executable, args...)
	process.Stdout, process.Stderr = logFile, logFile
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := process.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start interactive daemon: %w", err)
	}
	_ = logFile.Close()
	exited := make(chan error, 1)
	go func() {
		exited <- process.Wait()
	}()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	// 3. 轮询 open，直到 Socket 就绪并成功创建本次 workspace Runtime。
	for {
		client, err := agentd.OpenProcess(runDir, openReq)
		if err == nil {
			return client, nil
		}
		if !agentd.IsSupervisorUnavailable(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-exited:
			return nil, daemonStartError(runDir, fmt.Sprintf("interactive daemon exited before ready: %v", err))
		case <-deadline.C:
			return nil, daemonStartError(runDir, "interactive daemon did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func daemonStartError(runDir string, summary string) error {
	logPath := filepath.Join(runDir, "interactive.log")
	data, err := os.ReadFile(logPath)
	if err != nil || len(data) == 0 {
		return fmt.Errorf("%s; see %s", summary, logPath)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return fmt.Errorf("%s; see %s", summary, logPath)
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return fmt.Errorf("%s; see %s\n%s", summary, logPath, strings.Join(lines, "\n"))
}

func interactiveOpenRequest() (agentd.OpenRequest, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return agentd.OpenRequest{}, fmt.Errorf("get workspace: %w", err)
	}
	return agentd.OpenRequest{
		Workspace: cwd, Continue: continueLast, SessionID: sessionID,
		ModelRef: modelRef, LLMFormat: llmFormat, LLMModel: llmModel,
		Debug: debugMode, PromptBase: "tui",
	}, nil
}

func interactiveDaemonArgs() []string {
	args := make([]string, 0, 2)
	if debugMode {
		args = append(args, "--debug")
	}
	return append(args, "daemon")
}
