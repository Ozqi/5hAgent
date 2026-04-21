---
name: superpower
description: Use when user asks to "boost productivity", "work faster", "optimize workflow", "supercharge development", or mentions developer superpowers. Provides advanced techniques and shortcuts.
---

# Superpower

Advanced techniques and shortcuts to supercharge your development workflow.

## When to Use

This skill activates when:
- User wants to boost productivity
- Looking for advanced techniques
- Optimizing development workflow
- Learning power-user features

## Development Superpowers

### 1. Command Line Mastery

**Essential Shortcuts:**
- `Ctrl+R`: Reverse search command history
- `!!`: Repeat last command
- `!$`: Last argument of previous command
- `cd -`: Go back to previous directory
- `Ctrl+A/E`: Jump to start/end of line
- `Ctrl+U/K`: Delete to start/end of line

**Power Commands:**
```bash
# Find and execute
find . -name "*.go" -exec go fmt {} \;

# Parallel execution
ls *.txt | xargs -P 4 -I {} process {}

# Watch for changes
watch -n 1 'git status --short'

# Quick file editing
vim +/pattern file.txt  # Open at pattern
```

### 2. Git Superpowers

**Efficient Workflows:**
```bash
# Interactive staging
git add -p

# Fixup commits
git commit --fixup <commit>
git rebase -i --autosquash

# Search commit history
git log -S "function_name" --source --all

# Blame with context
git blame -L 10,20 file.go

# Stash with message
git stash push -m "WIP: feature X"
```

### 3. Editor Superpowers

**Multi-cursor Editing:**
- Select all occurrences: `Ctrl+Shift+L` (VS Code)
- Add cursor above/below: `Ctrl+Alt+Up/Down`
- Column selection: `Alt+Shift+Drag`

**Navigation:**
- Go to symbol: `Ctrl+Shift+O`
- Go to file: `Ctrl+P`
- Go to line: `Ctrl+G`
- Jump to definition: `F12`
- Find references: `Shift+F12`

**Refactoring:**
- Rename symbol: `F2`
- Extract function: `Ctrl+Shift+R`
- Organize imports: `Shift+Alt+O`

### 4. Debugging Superpowers

**Conditional Breakpoints:**
```javascript
// Break only when condition is true
if (user.id === 123) {
  debugger;
}
```

**Watch Expressions:**
- Monitor variable changes
- Evaluate expressions in context
- Track object mutations

**Time-Travel Debugging:**
- Record and replay execution
- Step backwards through code
- Inspect historical state

### 5. Testing Superpowers

**Fast Feedback:**
```bash
# Watch mode
npm test -- --watch

# Run only changed tests
jest --onlyChanged

# Parallel execution
go test -parallel 8 ./...
```

**Test Patterns:**
- Table-driven tests for multiple cases
- Snapshot testing for UI
- Property-based testing for edge cases
- Mutation testing for test quality

### 6. Productivity Hacks

**Automation:**
- Pre-commit hooks for linting/formatting
- Git aliases for common commands
- Shell functions for repetitive tasks
- Makefile for project commands

**Focus Techniques:**
- Pomodoro: 25min work, 5min break
- Deep work blocks: 2-4 hours uninterrupted
- Batch similar tasks together
- Time-box exploratory work

**Learning Shortcuts:**
- Read source code of libraries you use
- Contribute to open source
- Pair program with experienced developers
- Build side projects in new technologies

### 7. Code Generation

**Snippets:**
- Create custom code snippets
- Use template engines
- Generate boilerplate with tools

**AI Assistance:**
- Use copilot for repetitive patterns
- Generate tests from implementation
- Refactor with AI suggestions

### 8. Performance Optimization

**Profiling:**
```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=.

# Memory profiling
go test -memprofile=mem.prof -bench=.

# Flame graphs
perf record -g ./app
perf script | stackcollapse-perf.pl | flamegraph.pl > flame.svg
```

**Benchmarking:**
- Measure before optimizing
- Focus on hot paths
- Use realistic data
- Compare alternatives

## Power User Mindset

1. **Automate repetitive tasks**: If you do it twice, script it
2. **Learn keyboard shortcuts**: Mouse is slow
3. **Master your tools**: Deep knowledge > many tools
4. **Read documentation**: RTFM saves time
5. **Measure, don't guess**: Profile before optimizing
6. **Fail fast**: Quick feedback loops
7. **Share knowledge**: Teaching reinforces learning

## Resources

- **Cheat sheets**: Keep handy references
- **Dotfiles**: Version control your configs
- **Tool documentation**: Read the manual
- **Community**: Learn from others' workflows
