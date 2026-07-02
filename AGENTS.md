# AGENTS.md

## Overview

`5hAgent` is a lightweight Go + Eino agent framework.

```text
cmd/5hagent/main.go
  -> internal/llm
  -> internal/utils
  -> internal/agent
     -> internal/agent/tool_use.go
     -> internal/context
     -> internal/skill
  -> internal/tools
  -> internal/commands
```

Use this file as the working contract for agentic contributors in this repo.

## Repository Facts

- Main entrypoint: `cmd/5hagent/main.go`
- Main loop: `internal/agent/agent.go`
- Tool execution: `internal/agent/tool_use.go`
- Task persistence: `internal/agent/tasklist.go`
- Tool implementations: `internal/tools/*.go`
- Slash commands: `internal/commands/*.go`
- Prompt loader: `internal/utils/utils.go`
- Skill loader: `internal/skill/skill.go`
- Docs live in `doc/`

## Build And Run

- Run without building: `go run cmd/5hagent/main.go`
- Build binary: `go build -o 5hagent cmd/5hagent/main.go`
- Run built binary: `./5hagent`
- Run debug mode: `./5hagent --debug`

## Test Commands

- Run all Go tests: `go test ./...`
- Run one package: `go test ./internal/tools`
- Run one test by name: `go test ./internal/tools -run TestReadFileTool`
- Run one test with verbose output: `go test -v ./internal/utils -run TestLoad`
- Run llm tests only: `go test ./internal/llm -run TestNewClient`

## SWE-bench Scripts

- Run one root SWE-bench task: `./test_swebench.sh 1`
- Run all root SWE-bench tasks: `./test_swebench.sh all`
- Run one testspace task: `./testspace/test_runner.sh 1`
- Run all testspace tasks: `./testspace/test_runner.sh all`

These scripts expect a built `./5hagent` binary and a configured `.env`.

## Lint And Formatting

- Format code: `gofmt -w <file-or-dir>`
- If you touched multiple Go files, prefer: `gofmt -w ./cmd ./internal`
- There is currently no checked-in `Makefile`, `golangci-lint` config, Cursor rule set, or Copilot instruction file.
- Do not invent repo-local lint commands in docs or commits unless you add them intentionally.

## Required Workflow

- First inspect the relevant code before editing.
- Prefer the smallest correct change.
- Reuse existing functions and types whenever possible.
- Do not create new helpers or abstractions unless the current code clearly needs them.
- Do not revert unrelated worktree changes you did not make.
- Keep docs in sync with code in the same change.

## Design-Phase Rule

When the work is still in design stage:

- Write only code comments and function signatures.
- Do not write concrete implementation yet.
- Function header comments should state:
  - what the function does
  - its parameters
  - what it calls
  - the main steps it performs
- Function names should be short but clearly distinguishable.

## Implementation-Phase Rule

When the work is in implementation stage:

- Prefer reusing existing functions.
- Do not casually introduce brand new functions.
- Follow the current package structure.
- Keep files focused; avoid opportunistic refactors.

## Go Style

- Use `gofmt` formatting. Do not hand-format against Go conventions.
- Keep imports grouped by `gofmt`: stdlib, blank line, third-party/local.
- Package names are short and lowercase.
- Exported names use Go PascalCase.
- Unexported names use camelCase.
- Acronyms should follow existing local style; do not rename broadly just for style.
- Keep structs and JSON tags explicit and stable.
- Prefer typed structs for tool inputs and outputs.

## Comments And Docstrings

- Exported types and functions should have a leading Go comment.
- Comments in this repo are often bilingual or Chinese-heavy; preserve the local style of the file.
- Do not add noisy comments that restate obvious code.
- For docs in `doc/`, keep them concise and architecture-first.

## Error Handling

- Return errors instead of panicking in normal flows.
- Wrap errors with context using `fmt.Errorf("...: %w", err)`.
- Validate inputs early and return specific errors.
- Preserve actionable file path or parameter details in tool errors.
- If an error is intentionally non-fatal, log it or continue explicitly.

## Tooling Conventions In Code

- Tool registration is centralized in `internal/tools/registry.go` via `InitRegistry(...)`.
- LLM-facing tools currently include:
  - `read_file`
  - `write_file`
  - `edit`
  - `glob`
  - `grep`
  - `list_dir`
  - `exec_shell`
  - `task`
  - `skill`
- Slash commands `/task` and `/skill` are separate CLI handlers in `internal/commands/`.

## Current Implementation Notes

- Skills are loaded from `~/.5hAgent/skills/*/SKILL.md`.
- Prompts are loaded from `~/.5hAgent/prompt/*.md`.
- The main system prompt comes from `~/.5hAgent/prompt/main.md` via `internal/utils/utils.go`.
- Context compression is simple truncation, not summary-based compression.
- `internal/agent/tool_use.go` still classifies old task tool names in its read-only map; keep that mismatch in mind when changing task-tool concurrency behavior.

## Documentation Rules

- Documentation must match code.
- Keep docs under `doc/` aligned with the module they describe.
- Prefer short architecture diagrams at the top when useful.
- Mention key functions by name.
- Add file links when they make navigation easier.
- For agent-facing progress docs like `task.md`, dense high-signal writing is acceptable.
- Avoid redundant historical narrative unless it changes how contributors should work.

## Verification Before Finishing

- If you changed Go code, run relevant `go test` commands.
- If you changed startup wiring, also run `go build -o 5hagent cmd/5hagent/main.go`.
- If you changed only docs, at minimum verify commands, paths, and filenames against the current tree.
- Do not claim support for tools, skills, prompts, or scripts that are not present.

## Rules File Status

- No `AGENTS.md` existed before this file.
- No `.cursor/rules/` directory was found.
- No `.cursorrules` file was found.
- No `.github/copilot-instructions.md` file was found.

If any of those files are added later, update this document and fold their instructions in.
