package ppool

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/y7ut/ppool/option"
)

type SimpleWork struct {
	name          string
	TrueSpendTime time.Duration
	finished      bool
}

func (sw *SimpleWork) Work(ctx context.Context) {
	ticker := time.NewTicker(sw.TrueSpendTime * time.Second)
	defer ticker.Stop()
	select {
	case <-ctx.Done():
		fmt.Println("do work done", ctx.Err())
		return
	case <-ticker.C:
		sw.finished = true
		return
	}
}

// func (sw *SimpleWork) Do(ctx context.Context) {
// 	time.Sleep(sw.TrueSpendTime * time.Second)
// 	sw.finished = true
// 	log.Println("do work done")
// }

var nullLogger *log.Logger

func init() {
	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open /dev/null: %v", err)
	}
	nullLogger = log.New(nullFile, "", log.LstdFlags)
}

func TestPool_Start(t *testing.T) {
	defaultPool := &Pool[*SimpleWork]{
		maxWorkerCount:        3,
		maxIdleWorkerDuration: 5 * time.Second,
		timeout:               5 * time.Second,
		logger:                log.Default(),
		workChanPool: sync.Pool{
			New: func() interface{} {
				return &workerChan[*SimpleWork]{
					ch: make(chan **SimpleWork, workerChanCap),
				}
			},
		},
	}
	t.Run("start pool success test", func(t *testing.T) {
		defaultPool.start()
		assertExist(t, defaultPool.stopChannel, "stop channel must init after start")
		assertExist(t, defaultPool.waitBuffer, "buffer channel must init after start")
	})
	work := &SimpleWork{name: "worker", TrueSpendTime: 4}
	t.Run("wait buffer read test", func(t *testing.T) {
		defaultPool.waitBuffer <- &work
		time.Sleep((work.TrueSpendTime + 1) * time.Second)
		assertEqual(t, work.finished, true, "work must be serve in wait buffer, and finish")
		assertEqual(t, len(defaultPool.readyToWork), 1, "pool must have work in readyToWork list")
	})

	//
	t.Run("clean work test", func(t *testing.T) {
		// clean event need dispatch twice , this time worker has been clean
		time.Sleep(defaultPool.maxIdleWorkerDuration)
		assertEqual(t, len(defaultPool.readyToWork), 0, "readyToWork must clean after maxIdleWorkerDuration")
		assertEqual(t, defaultPool.WorkerCount, 0, "current worker count must be 0")
	})

	t.Run("stop test", func(t *testing.T) {
		close(defaultPool.stopChannel)
		close(defaultPool.waitBuffer)
		defaultPool.stop = true
		work := generateSimpleWork("work_to_test_start", 0)
		err := defaultPool.Do(work)
		assertEqual(t, err, fmt.Errorf("WP is Close"), "stop pool still send work to do must return error")
	})
}

func TestPool_Stop(t *testing.T) {
	defaultPool := &Pool[*SimpleWork]{
		maxWorkerCount:        3,
		maxIdleWorkerDuration: 10 * time.Second,
		timeout:               10 * time.Second,
		logger:                nullLogger,
		workChanPool: sync.Pool{
			New: func() interface{} {
				return &workerChan[*SimpleWork]{
					ch: make(chan **SimpleWork, workerChanCap),
				}
			},
		},
	}
	defaultPool.start()

	workers := make([]*SimpleWork, 0)
	for i := 0; i < 5; i++ {
		cw := generateSimpleWork(fmt.Sprintf("worker-mwl-%d", i), 3)
		ok, err := defaultPool.Serve(cw)
		if err != nil {
			t.Fatalf("Error serving worker-mwl-%d: %v", i, err)
		}
		if ok {
			workers = append(workers, cw)
		}
	}
	// 防止过快退出
	time.Sleep(100 * time.Millisecond)

	assertEqual(t, len(workers), defaultPool.maxWorkerCount, "only 3 workers should be accepted")

	startAt := time.Now()
	defaultPool.Stop()

	// 获得最大的花费时间
	slices.SortFunc(workers, func(a, b *SimpleWork) int {
		return int(a.TrueSpendTime - b.TrueSpendTime)
	})
	maxSpend := int(workers[len(workers)-1].TrueSpendTime)

	assertEqual(t, int(time.Since(startAt)/time.Second), maxSpend, "stop pool must cost max spend time")
	// 检查所有 worker 是否未完成（模拟状态验证）
	for _, work := range workers {
		if !work.finished {
			t.Errorf("Worker %s was not finished, but should", work.name)
		}
	}
	assertEqual(t, defaultPool.WorkerCount, 0, "stop pool must clean all work")
	assertEqual(t, len(defaultPool.readyToWork), 0, "stop pool must clean all readyToWork")
	assertEqual(t, defaultPool.stop, true, "stop pool must set stop to true")

	ok, err := defaultPool.Serve(generateSimpleWork("unable to serve", 1))
	assertEqual(t, ok, false, "unable to serve must return false")
	assertEqual(t, err, fmt.Errorf("WP is Close"), "unable to serve must return error")

	err = defaultPool.Do(generateSimpleWork("unable to do", 1))
	assertEqual(t, err, fmt.Errorf("WP is Close"), "unable to do must return error")
}

func TestPool_GetAvaliableWorkerChan(t *testing.T) {
	defaultPool := &Pool[*SimpleWork]{
		maxWorkerCount:        2,
		maxIdleWorkerDuration: 20 * time.Second,
		timeout:               15 * time.Second,
		logger:                log.Default(),
		workChanPool: sync.Pool{
			New: func() interface{} {
				return &workerChan[*SimpleWork]{
					ch: make(chan **SimpleWork, workerChanCap),
				}
			},
		},
	}
	defaultPool.start()
	assertEqual(t, len(defaultPool.readyToWork), 0, "when pool start, readyToWork count must be 0")

	w1 := &SimpleWork{name: "worker", TrueSpendTime: 4}
	t.Run("send first work to pool", func(t *testing.T) {
		ch := defaultPool.getAvailableCh()
		assertExist(t, ch, "get available ch from pool must success")
		ch.ch <- &w1
		assertEqual(t, defaultPool.WorkerCount, 1, "current worker count must be 1")
		// 经历clean事件后，才会添加到readyToWork
		assertEqual(t, len(defaultPool.readyToWork), 0, "pool readyToWork list after create worker len must be 0")
	})

	w2 := &SimpleWork{name: "worker", TrueSpendTime: 2}
	t.Run("send second work to pool", func(t *testing.T) {
		ch := defaultPool.getAvailableCh()
		assertExist(t, ch, "get available ch from pool must success")
		ch.ch <- &w2
		assertEqual(t, defaultPool.WorkerCount, 2, "current Worker count must be 2")
		assertEqual(t, len(defaultPool.readyToWork), 0, "pool readyToWork list after create worker len still 0")
	})

	t.Run("Send third work, should return nil", func(t *testing.T) {
		ch := defaultPool.getAvailableCh()
		if ch != nil {
			t.Fatal("getAvailableCh must return nil")
		}
		assertEqual(t, defaultPool.WorkerCount, 2, "current Worker still be 2")
	})

	// w1最慢4秒结束，等待都结束
	time.Sleep(w1.TrueSpendTime*time.Second + 100*time.Millisecond)
	// 结束后，目前还没有clean事件，所以readyToWork里面有2个worker
	assertEqual(t, len(defaultPool.readyToWork), 2, "After w1&w2 finish, readyToWork count must be 2")

	// 这次不需要初始化worker，直接从readyToWork里面拿就可以
	ch5 := defaultPool.getAvailableCh()
	assertExist(t, ch5, "After w1 finish, getAvailableCh must success")
	assertEqual(t, len(defaultPool.readyToWork), 1, "this time get ch from readyToWork, readyToWork count must be 1")
	ch5.ch <- nil

	defaultPool.Stop()

}

func TestPool_WorkerListening(t *testing.T) {
	defaultPool := &Pool[*SimpleWork]{
		maxWorkerCount:        2,
		maxIdleWorkerDuration: 10 * time.Second,
		timeout:               10 * time.Second,
		logger:                nullLogger,
		workChanPool: sync.Pool{
			New: func() interface{} {
				return &workerChan[*SimpleWork]{
					ch: make(chan **SimpleWork, workerChanCap),
				}
			},
		},
	}
	defaultPool.start()
	defer defaultPool.Stop()

	w1 := &SimpleWork{name: "worker", TrueSpendTime: 3}
	t.Run("Send first work", func(t *testing.T) {
		ch := defaultPool.getAvailableCh()
		assertExist(t, ch, "getAvailableCh must success")
		ch.ch <- &w1
		assertEqual(t, defaultPool.WorkerCount, 1, "current Worker count must be 1")
		assertEqual(t, len(defaultPool.readyToWork), 0, "getAvailableCh by create worker, readyToWork count must be 0")
	})

	time.Sleep(w1.TrueSpendTime*time.Second + 100*time.Millisecond)

	t.Run("Send nil", func(t *testing.T) {
		assertEqual(t, len(defaultPool.readyToWork), 1, "before send nil, readyToWork count must be 1")
		ch := defaultPool.getAvailableCh()
		assertEqual(t, len(defaultPool.readyToWork), 0, "getAvailableCh from readyToWork, current readyToWork count must be 0")
		assertExist(t, ch, "getAvailableCh must success")
		ch.ch <- nil
		// send nil, stop worker listening
		time.Sleep(100 * time.Millisecond)
		assertEqual(t, defaultPool.WorkerCount, 0, "current Worker count must be reduce to 0")
		assertEqual(t, len(defaultPool.readyToWork), 0, "getAvailableCh by create worker, readyToWork count must be 0")
	})

	t.Run("Send two work", func(t *testing.T) {
		ch := defaultPool.getAvailableCh()
		assertExist(t, ch, "getAvailableCh must success")
		ch.ch <- &w1
		assertEqual(t, defaultPool.WorkerCount, 1, "current Worker count must be 1")
		assertEqual(t, len(defaultPool.readyToWork), 0, "getAvailableCh by create worker, readyToWork count must be 0")

		ch = defaultPool.getAvailableCh()
		ch.ch <- &w1
		assertEqual(t, defaultPool.WorkerCount, 2, "current Worker count must be 1")
		assertEqual(t, len(defaultPool.readyToWork), 0, "getAvailableCh by create worker, readyToWork count must be 0")

		time.Sleep(w1.TrueSpendTime*time.Second + 100*time.Millisecond)
		assertEqual(t, len(defaultPool.readyToWork), 2, "After w1&w2 finish, readyToWork count must be 2")
	})

}

func TestPool_Clean(t *testing.T) {
	defaultPool := &Pool[*SimpleWork]{
		maxWorkerCount: 5,
		// 不执行自动清理，手动清理
		maxIdleWorkerDuration: 1000 * time.Second,
		timeout:               10 * time.Second,
		logger:                nullLogger,
		workChanPool: sync.Pool{
			New: func() interface{} {
				return &workerChan[*SimpleWork]{
					ch: make(chan **SimpleWork, workerChanCap),
				}
			},
		},
	}
	defaultPool.start()
	time.Sleep(3 * time.Second)
	for i := 0; i < 5; i++ {
		cw := &SimpleWork{name: fmt.Sprintf("worker-test-clean-%d", i), TrueSpendTime: 3}
		ok, err := defaultPool.Serve(cw)
		if err != nil {
			t.Fatalf("Error serving worker-mwl-%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("worker-mwl-%d is refused", i)
		}
	}
	assertEqual(t, defaultPool.WorkerCount, 5, "before done, current Worker count must be 5")
	assertEqual(t, len(defaultPool.readyToWork), 0, "before done, readyToWork count must be 0")
	time.Sleep(3*time.Second + 500*time.Millisecond)

	assertEqual(t, defaultPool.WorkerCount, 5, "after done, current Worker count must be 5")
	assertEqual(t, len(defaultPool.readyToWork), 5, "after done, readyToWork count must be 5")

	go func() {
		// 这里使用的就是readyToWork 中的chan
		for i := 0; i < 3; i++ {
			cw := &SimpleWork{name: fmt.Sprintf("worker-test-clean-%d", i), TrueSpendTime: 3}
			defaultPool.Do(cw)
		}
	}()

	// 4s后手动清理，但是5个里应还有3个重复使用了，不至于长时间没被使用，被清空
	var scrath []*workerChan[*SimpleWork]
	time.Sleep(4 * time.Second)
	defaultPool.maxIdleWorkerDuration = 3 * time.Second
	defaultPool.clean(&scrath)
	// 回收需要时间，需要等待一些毫秒再去判断
	time.Sleep(500 * time.Millisecond)
	assertEqual(t, len(scrath), 2, "after clean, scrath count must be 2")
	assertEqual(t, defaultPool.WorkerCount, 3, "after clean, current Worker count must be 0")
	assertEqual(t, len(defaultPool.readyToWork), 3, "after clean, readyToWork count must be 0")

	time.Sleep(4 * time.Second)
	defaultPool.clean(&scrath)
	// 回收需要时间，需要等待一些毫秒再去判断
	time.Sleep(500 * time.Millisecond)
	assertEqual(t, len(scrath), 3, "after clean again, scrath count must be 3")
	assertEqual(t, defaultPool.WorkerCount, 0, "after clean again, current Worker count must be 0")
	assertEqual(t, len(defaultPool.readyToWork), 0, "after clean again, readyToWork count must be 0")
	defer defaultPool.Stop()
}

// 测试默认的 `CreatePool` 配置
func TestCreatePool_DefaultConfig(t *testing.T) {
	pool, _ := CreatePool[*SimpleWork]()

	assertEqual(t, pool.maxWorkerCount, 10, "default maxWorkerCount")
	assertEqual(t, pool.maxIdleWorkerDuration, 5*time.Second, "default maxIdleWorkerDuration")
	assertEqual(t, pool.timeout, 5*time.Second, "default timeout")
	assertEqual(t, pool.logger, log.Default(), "default logger")

	pool.Stop()
}

func TestCreatePool_UnvalidConfig(t *testing.T) {
	_, err := CreatePool[*SimpleWork](
		option.WithMaxWorkCount(-1),
		option.WithMaxIdleWorkerDuration(-1),
		option.WithTimeout(-1),
	)

	if err == nil {
		t.Fatal("bad config must return error")
	}
	_, err = CreatePool[*SimpleWork](
		option.WithMaxWorkCount(1),
		option.WithMaxIdleWorkerDuration(1),
		option.WithTimeout(0),
	)

	if err == nil {
		t.Fatal("bad config must return error")
	}
}

// 测试自定义配置的 `CreatePool`
func TestCreatePool_CustomConfig(t *testing.T) {

	pool, err := CreatePool[*SimpleWork](
		option.WithMaxWorkCount(5),
		option.WithMaxIdleWorkerDuration(10*time.Second),
		option.WithTimeout(5*time.Second),
		option.WithLogger(nullLogger),
	)

	assertEqual(t, err, nil, "create pool must success")
	assertEqual(t, pool.maxWorkerCount, 5, "custom maxWorkerCount")
	assertEqual(t, pool.maxIdleWorkerDuration, 10*time.Second, "custom maxIdleWorkerDuration")
	assertEqual(t, pool.timeout, 5*time.Second, "custom timeout")
	assertEqual(t, pool.logger, nullLogger, "custom logger")

	// 测试是否可以成功添加 worker
	for i := 0; i < 5; i++ {
		worker := generateSimpleWork(fmt.Sprintf("worker-%d", i), i)
		ok, err := pool.Serve(worker)
		if err != nil {
			t.Fatalf("Error serving worker-%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("Expected worker-%d to be accepted, but it was refused", i)
		}
	}
	pool.Stop()
}

// 测试最大 worker 数限制, 5/3
func TestCreatePool_MaxWorkerLimit(t *testing.T) {
	pool, _ := CreatePool[*SimpleWork](
		option.WithMaxWorkCount(3),
		option.WithMaxIdleWorkerDuration(10*time.Second),
		option.WithTimeout(5*time.Second),
		option.WithLogger(nullLogger),
	)
	workers := make([]*SimpleWork, 0)
	for i := 0; i < 5; i++ {
		workers = append(workers, generateSimpleWork(fmt.Sprintf("worker-mwl-%d", i), 3))
		ok, err := pool.Serve(workers[i])
		if err != nil {
			t.Fatalf("Error serving worker-mwl-%d: %v", i, err)
		}
		if i >= 3 && ok {
			t.Fatalf("worker-mwl-%d should have been refused, but was accepted", i)
		}
		if i < 3 && !ok {
			t.Fatalf("worker-mwl-%d should have been accepted, but was refused", i)
		}
	}
	pool.Stop()
	// 检查所有 worker 是否未完成（模拟状态验证）
	for i, worker := range workers {
		if i < 3 {
			assertEqual(t, worker.finished, true, fmt.Sprintf("%s num small then 3 should be finished", worker.name))
			continue
		}
		assertEqual(t, worker.finished, false, fmt.Sprintf("%s num large then 3 should not be finished", worker.name))
	}
}

// 辅助断言函数
func assertEqual(t *testing.T, got, want interface{}, fieldName string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s mismatch: got %v, want %v", fieldName, got, want)
	}
}

func assertExist(t *testing.T, got interface{}, fieldName string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s mismatch: %s must exist, but nil", fieldName, got)
	}
}

func generateSimpleWork(name string, spend int) *SimpleWork {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	duration := time.Duration(spend + r.Intn(4))
	if duration == 0 {
		duration = 1
	}
	return &SimpleWork{name: name, TrueSpendTime: duration}
}
