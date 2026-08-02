// tmux_commands.go - 5hAgent 实例发现和接入命令。
// 当前用 tmux 作为最小进程载体；main.go 只负责注册 Cobra 子命令。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/lzq/5hAgent/internal/cli"
	"github.com/spf13/cobra"
)

func newPSCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List running 5hAgent instances",
		Run:   runPS,
	}
}

func newAttachCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <target>",
		Short: "Attach or switch to a tmux target",
		Args:  cobra.ExactArgs(1),
		Run:   runAttach,
	}
}

func runPS(cmd *cobra.Command, args []string) {
	output, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{session_name}:#{window_index}.#{pane_index}\t#{pane_current_command}\t#{window_name}\t#{pane_current_path}").Output()
	if err != nil {
		cli.PrintError(fmt.Errorf("tmux list panes: %w", err))
		os.Exit(1)
	}
	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Split(line, "\t")
		// pane_current_command 用于确认前台确实是 5hAgent；普通 shell 和其他 CLI 不属于这里。
		if len(fields) == 4 && fields[1] == "5hagent" {
			if count == 0 {
				fmt.Fprintln(writer, "TARGET\tSTATE\tWORK\tCWD")
			}
			fmt.Fprintf(writer, "%s\trunning\t%s\t%s\n", fields[0], fields[2], fields[3])
			count++
		}
	}
	if count == 0 {
		fmt.Fprintln(writer, "No running 5hAgent instances.")
	}
	_ = writer.Flush()
}

func runAttach(cmd *cobra.Command, args []string) {
	target := args[0]
	session := attachSession(target)
	// 先选中目标 pane，再切换或接入 session；runtime 继续留在 tmux 中运行。
	_ = exec.Command("tmux", "select-window", "-t", target).Run()
	_ = exec.Command("tmux", "select-pane", "-t", target).Run()
	if os.Getenv("TMUX") != "" {
		if err := exec.Command("tmux", "switch-client", "-t", session).Run(); err != nil {
			cli.PrintError(fmt.Errorf("tmux switch client: %w", err))
			os.Exit(1)
		}
		return
	}
	attach := exec.Command("tmux", "attach-session", "-t", session)
	attach.Stdin, attach.Stdout, attach.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := attach.Run(); err != nil {
		cli.PrintError(fmt.Errorf("tmux attach session: %w", err))
		os.Exit(1)
	}
}

func attachSession(target string) string {
	if idx := strings.Index(target, ":"); idx > 0 {
		return target[:idx]
	}
	return target
}
