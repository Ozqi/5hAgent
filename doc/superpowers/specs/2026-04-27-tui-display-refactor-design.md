# TUI Display Refactor Design

## Goal

Refactor `internal/cli/tui.go` so the running terminal UI keeps the layout format and visual design of the root `tui.go`, while reusing the real agent/chat logic already implemented in `internal/cli/tui.go`.

This change is display-focused. Existing runtime behaviors such as message submission, streaming assistant output, tool event display, task and skill state, and context stats remain functional.

## Scope

In scope:

- Rebuild the Bubble Tea layout in `internal/cli/tui.go` around the `tui.go` three-column skeleton.
- Reuse the `tui.go` color palette, title styles, side panel styles, and bottom status bar style.
- Keep the current `AppModel` state machine and command flow.
- Replace old conversation/status rendering with the new layout structure.
- Allow placeholder content in UI regions where final data mapping is not yet settled.

Out of scope:

- Adding new product features or commands.
- Implementing real navigation for sidebar items.
- Broad refactors outside the TUI display layer.

## Design Constraints

- `internal/cli/tui.go` is the real application UI; root `tui.go` is a visual reference, not the runtime entrypoint.
- The new UI should feel like `tui.go`, not merely reuse some colors.
- Behavior should be preserved where possible; this is not a logic rewrite.
- Old display code can be deleted when the new layout fully replaces it.

## Layout Mapping

### Left Sidebar

Keep the `tui.go` sidebar structure and tone:

- Top title block.
- Static menu list such as `CHATS`, `HISTORY`, `LOGS`, `AGENTS`.
- Highlight state for the current item.
- Bottom online/system indicator.

Initial implementation can keep the sidebar mostly presentational. If no real navigation behavior is wired yet, the selected item can remain a UI-local state.

### Main Column

Use the `tui.go` main pane layout:

- Header line showing product title and current section.
- Scrollable main content body backed by the existing `viewport`.
- Input area visually integrated into the center column rather than rendered as the current separate block style.

Conversation content mapping:

- `roleUser`, `roleAssistant`, `roleTool`, `roleSystem` continue to render from `entries`.
- Streaming assistant updates continue to append into the current assistant entry.
- Tool events remain visible in the conversation timeline.

If exact final message card styling is not decided during implementation, a simpler placeholder style is acceptable as long as the layout shell matches `tui.go`.

### Right Status Panel

Keep the `tui.go` status-panel framing and section rhythm, but replace demo data with live app data.

Target mapping:

- `SYSTEM_RESOURCES`: busy/state, tool call count, last tool.
- `ENVIRONMENT_CTX`: token usage, message count, summary count.
- `PROCESS_TREE`-style area: enabled skills, task summary, highlighted tasks.

If some labels do not map cleanly, use placeholders or renamed labels that still preserve the `tui.go` visual cadence.

### Bottom Status Bar

Keep the full-width reversed status bar format from `tui.go`.

Show a compact mix of:

- key hints such as quit/submit/scroll;
- current transient status such as `thinking`, `streaming`, `busy`;
- optional placeholder items if needed to keep the bar balanced.

## Code Structure

Keep existing behavior-oriented methods where possible:

- `Update`
- `submit`
- `runAgent`
- `snapshot`
- tool event handling
- streaming message handling

Rework display-oriented parts aggressively:

- `View`
- `resize`
- conversation rendering helpers
- status panel rendering helpers
- any style helpers tied to the old layout

If needed for clarity, move pure rendering helpers into a separate file under `internal/cli/`, but only if that keeps the change cleaner. Avoid introducing new abstraction layers beyond what the new layout requires.

## Deletion Plan

Delete or rewrite old UI code that is specific to the current split layout once the `tui.go`-style structure is in place.

Safe to remove:

- old left-column/input-block composition specific to the current layout;
- old status panel formatting that conflicts with the new shell;
- legacy commented display helpers that are no longer relevant.

Not to remove:

- message and tool event state;
- command handling;
- tests that still validate real behavior, though assertions may need updates for new labels/text.

## Error Handling

- Existing agent/tool errors continue to append into conversation entries.
- Layout code should degrade safely before window size is known by using current default dimensions.
- Placeholder text is acceptable for incomplete display sections, but the UI must remain renderable.

## Testing Strategy

Update tests to validate preserved behavior instead of old wording.

Expected test updates:

- rendering tests should assert for the new `tui.go`-style labels and sections;
- submit/typing/streaming tests should continue to verify user input, assistant creation, spinner state, and height constraints;
- if menu/sidebar state is introduced, add minimal tests only for the implemented behavior.

Verification commands after implementation:

- `go test ./internal/cli`
- `go test ./...`
- `go build -o 5hagent cmd/5hagent/main.go` if startup wiring or compile surface is affected

## Implementation Summary

The implementation should preserve the current TUI logic engine and swap the presentation layer so the runtime UI closely matches the structure, spacing, and visual identity of root `tui.go`. Placeholder display content is acceptable where the live data mapping is not yet finalized, but the overall shell should already look and behave like the target design.
