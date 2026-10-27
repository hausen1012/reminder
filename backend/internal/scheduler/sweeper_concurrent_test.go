package scheduler

import (
	"sync"
	"testing"
	"time"
)

// IsRunning / Start / Stop 会在请求 goroutine 与 main 的启停流程里并发访问，
// 本用例在 -race 下验证没有数据竞争。
func TestSweeperConcurrentStatusAccess(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	// 扫描间隔给到 1 小时：测试期间不会真的扫库
	s := NewSweeper(db, engine, testTZ(loc), time.Hour, time.Hour, time.Hour)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 一组 goroutine 持续读状态（模拟 /api/health 与监控页轮询）
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = s.IsRunning()
					_ = s.Interval()
				}
			}
		}()
	}

	// 另一组反复启停
	for i := 0; i < 3; i++ {
		s.Start()
		s.Stop()
	}

	close(stop)
	wg.Wait()

	if s.IsRunning() {
		t.Fatal("全部 Stop 之后不应 running")
	}
}

// 并发 Start 只应产生一个后台 goroutine：重复 Start 必须幂等。
func TestSweeperStartIdempotentUnderConcurrency(t *testing.T) {
	db := openSchedulerTestDB(t)
	loc := time.FixedZone("CST", 8*3600)
	engine := NewEngine(db, &fakeDispatcher{}, testTZ(loc))
	t.Cleanup(engine.Stop)

	s := NewSweeper(db, engine, testTZ(loc), time.Hour, time.Hour, time.Hour)
	t.Cleanup(s.Stop)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Start()
		}()
	}
	wg.Wait()

	if !s.IsRunning() {
		t.Fatal("并发 Start 后应为 running")
	}
}
