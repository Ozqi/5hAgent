// codex_command.go - Codex/ChatGPT 账户额度接入诊断。
// 该命令只检查本机 Codex CLI 和 5hAgent provider 配置，不读取或打印任何 token。
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/joho/godotenv"
	"github.com/lzq/5hAgent/internal/utils"
	"github.com/spf13/cobra"
)

func newCodexCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "codex",
		Short: "Check Codex account/proxy readiness",
		Long:  "Check local Codex CLI auth and 5hAgent LLM_CODEX_* OpenAI-compatible proxy configuration without reading credentials.",
		RunE:  runCodexStatus,
	}
}

func runCodexStatus(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "CHECK\tSTATUS\tDETAIL")
	printCodexCLI(writer)
	printCodexLogin(writer)
	printCodexProvider(writer)
	if err := writer.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "5hAgent does not read Codex/ChatGPT tokens directly.")
	fmt.Fprintln(out, "Use a trusted local OpenAI-compatible Codex proxy, then configure:")
	fmt.Fprintln(out, "  LLM_MODEL=codex/gpt-5.1")
	fmt.Fprintln(out, "  LLM_CODEX_FORMAT=openai")
	fmt.Fprintln(out, "  LLM_CODEX_BASE_URL=http://127.0.0.1:8787/v1")
	fmt.Fprintln(out, "  LLM_CODEX_API_KEY=<proxy-client-key-or-placeholder>")
	return nil
}

func printCodexCLI(writer *tabwriter.Writer) {
	path, err := exec.LookPath("codex")
	if err != nil {
		fmt.Fprintln(writer, "codex cli\tmissing\tinstall official Codex CLI first")
		return
	}
	version := strings.TrimSpace(runShort(time.Second, "codex", "--version"))
	if version == "" {
		version = filepath.Base(path)
	}
	fmt.Fprintf(writer, "codex cli\tok\t%s (%s)\n", version, path)
}

func printCodexLogin(writer *tabwriter.Writer) {
	if _, err := exec.LookPath("codex"); err != nil {
		fmt.Fprintln(writer, "codex login\tskipped\tcodex cli missing")
		return
	}
	cmd := exec.Command("codex", "login", "status")
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "codex login\tmissing\t%s\n", firstLine(buffer.String(), "run codex login"))
		return
	}
	fmt.Fprintf(writer, "codex login\tok\t%s\n", firstLine(buffer.String(), "credentials present"))
}

func printCodexProvider(writer *tabwriter.Writer) {
	env := loadUserEnv()
	model := envOrProcess(env, "LLM_MODEL")
	format := envOrProcess(env, "LLM_CODEX_FORMAT")
	baseURL := envOrProcess(env, "LLM_CODEX_BASE_URL")
	apiKey := envOrProcess(env, "LLM_CODEX_API_KEY")
	if strings.HasPrefix(model, "codex/") {
		fmt.Fprintf(writer, "LLM_MODEL\tok\t%s\n", model)
	} else {
		fmt.Fprintf(writer, "LLM_MODEL\tinfo\tcurrent=%s; use codex/<model> to spend Codex proxy quota\n", fallback(model, "unset"))
	}
	if format == "openai" && baseURL != "" {
		fmt.Fprintf(writer, "LLM_CODEX\tok\tformat=openai base_url=%s api_key_present=%v\n", baseURL, apiKey != "")
		return
	}
	fmt.Fprintf(writer, "LLM_CODEX\tmissing\tformat=%s base_url_present=%v api_key_present=%v\n", fallback(format, "unset"), baseURL != "", apiKey != "")
}

func runShort(timeout time.Duration, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	if err := cmd.Start(); err != nil {
		return ""
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
		return buffer.String()
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return ""
	}
}

func loadUserEnv() map[string]string {
	configDir, err := utils.GetConfigDir()
	if err != nil {
		return nil
	}
	env, err := godotenv.Read(filepath.Join(configDir, ".env"))
	if err != nil {
		return nil
	}
	return env
}

func envOrProcess(env map[string]string, key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return strings.TrimSpace(env[key])
}

func firstLine(text string, fallbackText string) string {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return fallbackText
}

func fallback(value string, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
