---
name: debug-helper
description: Use when user asks to "debug", "troubleshoot", "find bug", "fix error", or mentions debugging methodology. Provides systematic debugging approach.
---

# Debug Helper

Systematic debugging methodology for identifying and fixing bugs.

## When to Use

This skill activates when:
- User asks to debug code or troubleshoot issues
- Error messages need investigation
- Bug reproduction is needed
- Root cause analysis is required

## Debugging Workflow

### Phase 1: Reproduce the Problem

1. **Confirm the issue**: Verify the problem exists and understand the expected vs actual behavior
2. **Identify trigger conditions**: What specific actions or inputs cause the issue?
3. **Check consistency**: Does it happen every time or intermittently?

### Phase 2: Gather Information

1. **Error messages**: Collect full error text, stack traces, and error codes
2. **Logs**: Check application logs, system logs, and debug output
3. **Environment**: Note OS, versions, dependencies, and configuration
4. **Recent changes**: What changed before the issue appeared?

### Phase 3: Form Hypotheses

1. **Analyze the evidence**: What do the errors and logs suggest?
2. **List possible causes**: Brainstorm potential root causes
3. **Prioritize**: Start with the most likely causes

### Phase 4: Test Hypotheses

1. **Create minimal reproduction**: Strip away unrelated code
2. **Test one change at a time**: Isolate variables
3. **Add logging**: Insert debug output at key points
4. **Use debugging tools**: Leverage debuggers, profilers, network inspectors

### Phase 5: Fix and Verify

1. **Implement the fix**: Make targeted changes
2. **Test thoroughly**: Verify the fix works in all scenarios
3. **Check for regressions**: Ensure no new issues were introduced
4. **Document**: Note the root cause and solution

## Common Debugging Techniques

- **Binary search**: Comment out half the code to narrow down the problem
- **Rubber duck debugging**: Explain the problem step-by-step
- **Compare working vs broken**: What's different?
- **Check assumptions**: Verify what you think is true actually is
- **Read error messages carefully**: They often point directly to the issue
