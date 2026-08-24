package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Ozqi/walle/internal/systemd"
	"github.com/Ozqi/walle/internal/utils"
)

// startInteractiveClient 启动脱离当前终端的 daemon，并等待其 Unix Socket 可接入。
func startInteractiveClient(ctx context.Context) (*systemd.ProcessClient, error) {
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
	if client, err := systemd.AttachProcess(runDir, "interactive"); err == nil {
		return client, nil
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
	target := "interactive"
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	// 3. 轮询 Socket 就绪状态，同时响应调用方取消和启动超时。
	for {
		client, err := systemd.AttachProcess(runDir, target)
		if err == nil {
			return client, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-exited:
			return nil, fmt.Errorf("interactive daemon exited before ready: %w; see %s", err, filepath.Join(runDir, "interactive.log"))
		case <-deadline.C:
			return nil, fmt.Errorf("interactive daemon %s did not become ready; see %s", target, filepath.Join(runDir, "interactive.log"))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func interactiveDaemonArgs() []string {
	args := make([]string, 0, 12)
	if debugMode {
		args = append(args, "--debug")
	}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	if continueLast {
		args = append(args, "--continue")
	}
	if llmFormat != "" {
		args = append(args, "--llm-format", llmFormat)
	}
	if llmModel != "" {
		args = append(args, "--llm-model", llmModel)
	}
	if modelRef != "" {
		args = append(args, "--model", modelRef)
	}
	return append(args, "daemon")
}
