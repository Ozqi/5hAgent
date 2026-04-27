# TUI Display Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the runtime TUI so `internal/cli/tui.go` visually follows root `tui.go` while preserving the current agent interaction logic.

**Architecture:** Keep `AppModel` state, command flow, and snapshot generation intact, and replace the display layer with a `tui.go`-style three-column shell plus bottom status bar. Reuse the existing viewport, textarea, and conversation entry data, but remap them into the new sidebar, main pane, and status panel structure.

**Tech Stack:** Go, Bubble Tea, Bubbles viewport/textarea, Lip Gloss

---

### Task 1: Rebuild TUI Layout Shell

**Files:**
- Modify: `internal/cli/tui.go`

- [ ] Step 1: Replace the old two-pane view composition with a `tui.go`-style sidebar + main pane + right panel + bottom status bar layout.
- [ ] Step 2: Reuse or adapt the root `tui.go` style tokens in `internal/cli/tui.go`, including sidebar, panel, title, nav item, and status bar styles.
- [ ] Step 3: Keep the center pane backed by the existing viewport and input models, but render them inside the new shell instead of the old left-column/input-block layout.

### Task 2: Map Live App State Into The New Shell

**Files:**
- Modify: `internal/cli/tui.go`

- [ ] Step 1: Add sidebar menu state and render placeholder navigation items in the visual style of root `tui.go`.
- [ ] Step 2: Render the center header using the selected menu label and current runtime state.
- [ ] Step 3: Map `snapshot()` output into the right panel sections using live values where available and placeholders where mapping is still visual-only.
- [ ] Step 4: Render the bottom status bar with key hints and current state text.

### Task 3: Simplify Old Rendering Helpers

**Files:**
- Modify: `internal/cli/tui.go`
- Modify: `internal/cli/ui.go`

- [ ] Step 1: Rewrite conversation entry rendering as needed so it fits the new pane widths and labels cleanly.
- [ ] Step 2: Remove or rewrite helper functions that only existed for the old layout.
- [ ] Step 3: Delete stale commented display helpers that are no longer needed after the TUI refactor.

### Task 4: Verify Compile Surface

**Files:**
- Modify: none

- [ ] Step 1: Run `go build -o 5hagent cmd/5hagent/main.go` to verify the refactor compiles.
- [ ] Step 2: Do not add or update automated test code, per user request.

## Notes

- Per user instruction, skip adding test code and skip test maintenance as part of this task.
- Placeholder display text is acceptable for UI sections whose final content is not yet defined, as long as the application remains functional and visually aligned with root `tui.go`.
