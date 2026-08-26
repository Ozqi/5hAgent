package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Ozqi/walle/internal/agentd"
	"github.com/Ozqi/walle/internal/tui"
	"github.com/Ozqi/walle/internal/utils"
	"github.com/spf13/cobra"
)

func newPSCommand() *cobra.Command {
	return &cobra.Command{Use: "ps", Short: "List running Agent processes", RunE: runPS}
}

func newAttachCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <process-id>",
		Short: "Follow a running Agent process",
		Args:  cobra.ExactArgs(1),
		RunE:  runAttach,
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		processes, err := runningProcesses()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var matches []string
		for _, proc := range processes {
			if strings.HasPrefix(proc.ID, prefix) {
				matches = append(matches, proc.ID+"\t"+processLabel(proc))
			}
		}
		return matches, cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runPS(cmd *cobra.Command, args []string) error {
	processes, err := runningProcesses()
	if err != nil {
		return err
	}
	if len(processes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No running Agent processes.")
		return nil
	}
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "PROCESS\tSTATE\tNAME\tSTARTED\tMODEL\tSESSION\tWORKSPACE")
	for _, proc := range processes {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", proc.ID, proc.State, processLabel(proc), proc.StartedAt.Local().Format("15:04:05"), emptyDash(proc.Model), emptyDash(proc.SessionID), proc.Workspace)
	}
	return writer.Flush()
}

func runAttach(cmd *cobra.Command, args []string) error {
	// 先从控制目录解析目标进程，再按交互能力选择 Socket 或文件跟随通道。
	processes, err := runningProcesses()
	if err != nil {
		return err
	}
	for _, proc := range processes {
		if proc.ID != args[0] {
			continue
		}
		if proc.Interactive {
			configDir, err := utils.GetConfigDir()
			if err != nil {
				return err
			}
			client, err := agentd.AttachProcess(configDir+"/run", proc.ID)
			if err != nil {
				return err
			}
			return tui.LaunchAttachedTUI(cmd.Context(), client)
		}
		if proc.WorkLogPath == "" {
			return fmt.Errorf("process %s has not opened its worklog yet", proc.ID)
		}
		return followFile(cmd.Context(), cmd.OutOrStdout(), proc.WorkLogPath)
	}
	return fmt.Errorf("running process %q not found", args[0])
}

func runningProcesses() ([]agentd.ProcessSnapshot, error) {
	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil, err
	}
	return agentd.ListProcesses(configDir + "/run")
}

func processLabel(proc agentd.ProcessSnapshot) string {
	if proc.Name != "" {
		return proc.Name
	}
	return "-"
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func followFile(ctx context.Context, out io.Writer, path string) error {
	// 1. 从文件当前位置持续读取新增日志，I/O 错误直接返回调用方。
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open process worklog %s: %w", path, err)
	}
	defer file.Close()
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := file.Read(buffer)
		if n > 0 {
			if _, err := out.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("read process worklog %s: %w", path, readErr)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
		processes, err := runningProcesses()
		if err != nil {
			return err
		}
		running := false
		for _, proc := range processes {
			if proc.WorkLogPath == path {
				running = true
				break
			}
		}
		if !running {
			// 2. 进程退出后读尽文件尾部，避免遗漏退出前最后一次写入。
			for {
				n, _ := file.Read(buffer)
				if n == 0 {
					return nil
				}
				_, _ = out.Write(buffer[:n])
			}
		}
	}
}
