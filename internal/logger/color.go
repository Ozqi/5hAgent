// color.go - ANSI 颜色输出
// 功能：Red/Green/Yellow/Blue/Cyan/Magenta/Gray/Bold 颜色函数
// 导出函数：DisableColor, Red, Green, Yellow, Blue, Cyan, Magenta, Gray, Bold
package logger

import (
	"os"
)

// ANSI 颜色代码
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
	colorBold    = "\033[1m"
)

var levelNames = map[Level]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
}

var colorEnabled = true

func init() {
	// 检测是否支持颜色（简单判断）
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		colorEnabled = false
	}
}

// DisableColor 禁用颜色输出
func DisableColor() {
	colorEnabled = false
}

// colorize 给文本添加颜色
func colorize(color, text string) string {
	if !colorEnabled {
		return text
	}
	return color + text + colorReset
}

// 日志级别颜色
func colorLevel(level Level) string {
	if !colorEnabled {
		return levelNames[level]
	}

	switch level {
	case DEBUG:
		return colorBlue + levelNames[level] + colorReset
	case INFO:
		return colorGreen + levelNames[level] + colorReset
	case WARN:
		return colorYellow + levelNames[level] + colorReset
	case ERROR:
		return colorRed + colorBold + levelNames[level] + colorReset
	default:
		return levelNames[level]
	}
}

// 标签颜色
func colorTag(tag string) string {
	if !colorEnabled {
		return tag
	}

	switch tag {
	case "SYS":
		return colorMagenta + tag + colorReset
	case "REACT":
		return colorCyan + tag + colorReset
	case "LLM":
		return colorMagenta + tag + colorReset
	case "STREAM":
		return colorBlue + tag + colorReset
	case "TOOL":
		return colorCyan + colorBold + tag + colorReset
	case "CTX":
		return colorGray + tag + colorReset
	case "USER":
		return colorGreen + colorBold + tag + colorReset
	case "AGENT":
		return colorYellow + tag + colorReset
	default:
		return tag
	}
}

// 导出的颜色函数，供其他模块使用
func Red(text string) string {
	return colorize(colorRed, text)
}

func Green(text string) string {
	return colorize(colorGreen, text)
}

func Yellow(text string) string {
	return colorize(colorYellow, text)
}

func Blue(text string) string {
	return colorize(colorBlue, text)
}

func Cyan(text string) string {
	return colorize(colorCyan, text)
}

func Magenta(text string) string {
	return colorize(colorMagenta, text)
}

func Gray(text string) string {
	return colorize(colorGray, text)
}

func Bold(text string) string {
	if !colorEnabled {
		return text
	}
	return colorBold + text + colorReset
}
