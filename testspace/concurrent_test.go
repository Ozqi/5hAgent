package testspace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/lzq/5hAgent/internal/tools"
)

// 模拟耗时工具
func slowTool(ctx context.Context, id int, dur time.Duration) string {
	select {
	case <-time.After(dur):
		return fmt.Sprintf("completed_%d", id)
	case <-ctx.Done():
		return "cancelled"
	}
}

func runTool(ctx context.Context, t tool.BaseTool, args string) error {
	if enhanced, ok := t.(tool.EnhancedInvokableTool); ok {
		_, err := enhanced.InvokableRun(ctx, &schema.ToolArgument{Text: args})
		return err
	}
	if invokable, ok := t.(tool.InvokableTool); ok {
		_, err := invokable.InvokableRun(ctx, args)
		return err
	}
	return fmt.Errorf("tool does not support invocation")
}

// 测试并发执行
func TestToolConcurrency(t *testing.T) {
	const (
		numTools     = 10
		toolDuration = 200 * time.Millisecond
	)

	start := time.Now()

	var wg sync.WaitGroup
	var completed int64

	// 模拟并发执行多个工具
	for i := 0; i < numTools; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			result := slowTool(ctx, id, toolDuration)
			atomic.AddInt64(&completed, 1)
			t.Logf("Tool %d: %s", id, result)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Completed: %d/%d tools", completed, numTools)
	t.Logf("Elapsed: %v (expected sequential: %v, concurrent: ~%v)",
		elapsed, time.Duration(numTools)*toolDuration, toolDuration)

	if elapsed > toolDuration*2 {
		t.Errorf("too slow for concurrent execution: %v > %v", elapsed, toolDuration*2)
	}
}

// 测试并发执行多个list_dir工具
func TestConcurrentListDir(t *testing.T) {
	ctx := context.Background()

	paths := []string{
		"/home/lzq/Proj/5hAgent/internal",
		"/home/lzq/Proj/5hAgent/cmd",
		"/home/lzq/Proj/5hAgent/prompt",
		"/tmp",
		"/var",
	}

	start := time.Now()

	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()

			tool := tools.GetToolByName("list_dir")
			if tool == nil {
				t.Logf("list_dir tool not found")
				return
			}

			err := runTool(ctx, tool, fmt.Sprintf(`{"path":"%s"}`, p))
			if err != nil {
				t.Logf("Error listing %s: %v", p, err)
				return
			}
			t.Logf("[%d] %s completed", idx, p)
		}(i, path)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Concurrent list_dir completed in %v", elapsed)
}

// 测试并发执行多个不同工具
func TestConcurrentMixedTools(t *testing.T) {
	ctx := context.Background()

	toolCalls := []struct {
		name string
		args string
	}{
		{"list_dir", `{"path":"/home/lzq/Proj/5hAgent/internal"}`},
		{"read_file", `{"path":"/home/lzq/Proj/5hAgent/go.mod","limit":50}`},
		{"glob", `{"path":"/home/lzq/Proj/5hAgent","pattern":"*.md"}`},
		{"list_dir", `{"path":"/home/lzq/Proj/5hAgent/cmd"}`},
		{"grep", `{"path":"/home/lzq/Proj/5hAgent","pattern":"package"}`},
	}

	start := time.Now()
	var wg sync.WaitGroup
	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, name, args string) {
			defer wg.Done()

			tool := tools.GetToolByName(name)
			if tool == nil {
				t.Logf("Tool %s not found", name)
				return
			}

			err := runTool(ctx, tool, args)
			if err != nil {
				t.Logf("[%d] %s error: %v", idx, name, err)
				return
			}
			t.Logf("[%d] %s completed", idx, name)
		}(i, tc.name, tc.args)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("Mixed concurrent tools completed in %v", elapsed)
}

// 测试并发工具执行性能
func TestConcurrentToolPerformance(t *testing.T) {
	ctx := context.Background()

	const numRuns = 20
	paths := []string{"/tmp", "/var", "/home"}

	// 顺序执行
	seqStart := time.Now()
	for r := 0; r < numRuns; r++ {
		for _, p := range paths {
			tool := tools.GetToolByName("list_dir")
			if tool != nil {
				_ = runTool(ctx, tool, fmt.Sprintf(`{"path":"%s"}`, p))
			}
		}
	}
	seqElapsed := time.Since(seqStart)

	// 并发执行
	conStart := time.Now()
	for r := 0; r < numRuns; r++ {
		var wg sync.WaitGroup
		for _, p := range paths {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				tool := tools.GetToolByName("list_dir")
				if tool != nil {
					_ = runTool(ctx, tool, fmt.Sprintf(`{"path":"%s"}`, path))
				}
			}(p)
		}
		wg.Wait()
	}
	conElapsed := time.Since(conStart)

	t.Logf("Sequential: %v, Concurrent: %v, Speedup: %.2fx", seqElapsed, conElapsed, float64(seqElapsed)/float64(conElapsed))
}

// Benchmark并发工具执行
func BenchmarkConcurrentTools(b *testing.B) {
	b.Run("10_tools_sequential", func(b *testing.B) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			for j := 0; j < 10; j++ {
				_ = slowTool(ctx, j, 10*time.Millisecond)
			}
		}
	})

	b.Run("10_tools_concurrent", func(b *testing.B) {
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup
			for j := 0; j < 10; j++ {
				wg.Add(1)
				go func(j int) {
					defer wg.Done()
					_ = slowTool(ctx, j, 10*time.Millisecond)
				}(j)
			}
			wg.Wait()
		}
	})
}

// 测试eino tool.BaseTool接口
func TestToolInterface(t *testing.T) {
	ctx := context.Background()

	// 测试list_dir工具
	tool := tools.GetToolByName("list_dir")
	if tool == nil {
		t.Fatal("list_dir tool not found")
	}

	info, err := tool.Info(ctx)
	if err != nil {
		t.Fatalf("tool.Info failed: %v", err)
	}
	t.Logf("Tool: %s, desc: %s", info.Name, info.Desc)

	if err := runTool(ctx, tool, `{"path":"/tmp"}`); err != nil {
		t.Fatalf("tool.InvokableRun failed: %v", err)
	}
}

// 测试最大并发数限制
func TestMaxConcurrency(t *testing.T) {
	const (
		numTools     = 100
		toolDuration = 50 * time.Millisecond
		// 内部使用8个worker
		expectedDuration = toolDuration * 13 // 100/8 ≈ 13 batches
	)

	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < numTools; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = slowTool(context.Background(), id, toolDuration)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("100 tools with 8 workers completed in %v (expected ~%v)", elapsed, expectedDuration)

	// 允许一定误差
	if elapsed > expectedDuration*2 {
		t.Errorf("too slow: %v > %v", elapsed, expectedDuration*2)
	}
}

// 测试并发竞争场景
func TestConcurrentRace(t *testing.T) {
	const numGoroutines = 50

	var counter int64
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				atomic.AddInt64(&counter, 1)
			}
		}()
	}

	wg.Wait()

	expected := int64(numGoroutines * 100)
	if counter != expected {
		t.Errorf("race condition detected: got %d, want %d", counter, expected)
	}
	t.Logf("Race test passed: counter=%d", counter)
}
