package ppool

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/y7ut/ppool/option"
)

// workerChanCap 工作通道的容量
var workerChanCap = func() int {
	if runtime.GOMAXPROCS(0) == 1 {
		return 0
	}
	return 1
}()

// Pool 协程池
type Pool[T WorkAble] struct {
	// stop 是否关闭
	stop bool
	// maxWorkerCount 工作协程的最大数量
	maxWorkerCount int
	// WorkerCount 当前工作协程的数量
	WorkerCount int

	// Worker的最大空闲时间, 超过这个时间的工作协程会被回收
	maxIdleWorkerDuration time.Duration

	// timeout 超时时间，超过这个时间的工作将会通过上下文来取消
	timeout time.Duration

	// readyToWork 用来存放工作通道的数组，他的本质就是一个队列
	// readyToWork 内部存放 Workchan 的顺序是最后使用时间从先到后
	readyToWork []*workerChan[T]

	// stopChannel 停止信号的通道
	stopChannel chan struct{}

	// waitBuffer 等待发起任务的通道，用于内部的简单阻塞调度
	waitBuffer chan *T

	// workChanPool 创建工作对象的对象池
	workChanPool sync.Pool

	// logger 日志器
	logger *log.Logger

	startOnce sync.Once

	// Lock 全局锁
	sync.Mutex
}

// workerChan 工作调度器
type workerChan[T WorkAble] struct {
	// lastUseTime 上次工作时间
	lastUseTime time.Time
	// ch 获取工作协程的通道
	ch chan *T
}

// CreatePool 创建工作池
func CreatePool[T WorkAble](options ...*option.Option) (*Pool[T], error) {
	// 获取默认配置
	option := option.DefaultOption()
	for _, opt := range options {
		opt.Apply(option)
	}
	if option.GetMaxWorkCount() < 2 {
		return nil, fmt.Errorf("maxWorkCount must be greater than 2")
	}

	if option.GetMaxIdleWorkerDuration() < time.Second {
		return nil, fmt.Errorf("maxIdleWorkerDuration must be greater than 1s")
	}

	// 创建工作池
	wp := &Pool[T]{
		maxWorkerCount:        option.GetMaxWorkCount(),
		maxIdleWorkerDuration: option.GetMaxIdleWorkerDuration(),
		timeout:               option.GetTimeOutDuration(),
		logger:                option.GetLogger(),
		workChanPool: sync.Pool{
			New: func() interface{} {
				return &workerChan[T]{
					ch: make(chan *T, workerChanCap),
				}
			},
		},
	}
	// 启动
	wp.start()
	return wp, nil
}

// start 启动
func (wp *Pool[T]) start() {
	wp.startOnce.Do(func() {
		wp.Lock()
		wp.stopChannel = make(chan struct{})
		wp.waitBuffer = make(chan *T)

		wp.Unlock()

		// 启动协程
		go func() {
			// 垃圾箱
			var scrath []*workerChan[T]
			ticker := time.NewTicker(wp.maxIdleWorkerDuration)
			for {
				select {
				case <-wp.stopChannel:
					wp.logger.Println("Master close")
					return
				case <-ticker.C:
					wp.clean(&scrath)
					if len(scrath) > 0 {
						wp.logger.Printf("clean %d worker, last used time is %s\n", len(scrath), scrath[len(scrath)-1].lastUseTime.Format("2006-01-02 15:04:05"))
					}
				}
			}
		}()

		go func() {
			for {
				select {
				case ch, ok := <-wp.waitBuffer:
					if !ok {
						return
					}
					if ch == nil {
						continue
					}

					afterTime := 1
					for {
						ok, err := wp.Serve(*ch)
						if ok {
							break
						}
						if err != nil {
							log.Println(err)
							break
						}
						time.Sleep(time.Duration(afterTime) * 500 * time.Millisecond)
						afterTime++
					}
				case <-wp.stopChannel:
					return
				}
			}
		}()
	})
}

// Stop 停止
func (wp *Pool[T]) Stop() {

	wp.Lock()
	if wp.stop {
		wp.Unlock()
		wp.logger.Println("pool has been stoped")
		return
	}
	close(wp.stopChannel)
	close(wp.waitBuffer)
	for i, ch := range wp.readyToWork {
		ch.ch <- nil
		wp.readyToWork[i] = nil
	}
	wp.readyToWork = wp.readyToWork[:0]
	wp.stop = true
	wp.Unlock()

	// 等待所有的工作协程退出
	for {
		if wp.WorkerCount == 0 {
			wp.logger.Println("all worker exit")
			return
		}
		wp.logger.Println("current worker count:", wp.WorkerCount)
		time.Sleep(1 * time.Second)
	}
}

// Serve 尝试获取工作协程，返回的参数第一个表示是否有空闲的工作协程，第二个表示错误
func (wp *Pool[T]) Serve(c T) (bool, error) {

	if wp.stop {
		return false, fmt.Errorf("WP is Close")
	}

	AvailableCh := wp.getAvailableCh()
	if AvailableCh == nil {
		return false, nil
	}
	AvailableCh.ch <- &c
	return true, nil
}

// Do 获取工作协程去运行, 会阻塞直到任务获取到协程去执行
func (wp *Pool[T]) Do(c T) error {
	if wp.stop {
		return fmt.Errorf("WP is Close")
	}
	select {
	case wp.waitBuffer <- &c:
		return nil
	case <-wp.stopChannel:
		return fmt.Errorf("WP is Close")
	}
}

// getAvailableCh 获取一个可用的Worker
func (wp *Pool[T]) getAvailableCh() (ch *workerChan[T]) {
	couldCreateWorker := false
	wp.Lock()
	// 从ReadyToWork的后方获取WorkerChan
	n := len(wp.readyToWork) - 1
	if n < 0 {
		// 如果没有空闲的工作协程通道，则需要创建一个
		if wp.WorkerCount < wp.maxWorkerCount {
			couldCreateWorker = true
			wp.WorkerCount++
		}
	} else {
		// 有空闲的工作协程通道，接下来可以直接返回了
		ch = wp.readyToWork[n]
		wp.readyToWork[n] = nil
		wp.readyToWork = wp.readyToWork[:n]
	}

	wp.Unlock()
	if ch == nil {
		if !couldCreateWorker {
			return nil
		}

		vch := wp.workChanPool.Get()
		ch = vch.(*workerChan[T])

		go wp.startWorkerChanListening(ch)
	}
	// 顺利返回workerchan
	return ch
}

// startWorkerListening 启动WorkerChan监听
func (wp *Pool[T]) startWorkerChanListening(ch *workerChan[T]) {
	if ch == nil {
		panic("the workerChan to start is nil")
	}
	// 启动 WorkerChan，接收工作请求
	for c := range ch.ch {
		if c == nil {
			// 有两种情况会停止 WorkerChan 的监听
			// 1. workerChan长期不使用,被回收
			// 2. Pool被关闭
			break
		}
		ctx, cancel := context.WithTimeout(context.Background(), wp.timeout)

		(*c).Work(ctx)

		cancel()

		// 使用完毕后再将 WorkerChan 注册回可用队列
		if !wp.release(ch) {
			break
		}
	}

	// 离开的时候，减少WorkerCount
	wp.Lock()
	wp.WorkerCount--
	wp.Unlock()
	wp.workChanPool.Put(ch)
}

// release 释放
func (wp *Pool[T]) release(ch *workerChan[T]) bool {
	// 记录一下时间
	ch.lastUseTime = time.Now()

	wp.Lock()
	if wp.stop {
		wp.Unlock()
		return false
	}
	wp.readyToWork = append(wp.readyToWork, ch)
	wp.Unlock()

	return true
}

// clean 清理长时间不使用的Worker
func (wp *Pool[T]) clean(scratch *[]*workerChan[T]) {
	maxIdleWorkerDuration := wp.maxIdleWorkerDuration
	current := time.Now()

	wp.Lock()
	ready := wp.readyToWork
	n := len(ready)

	i := 0

	for i < n && current.Sub(ready[i].lastUseTime) > maxIdleWorkerDuration {
		i++
	}

	*scratch = append((*scratch)[:0], ready[:i]...)
	if i > 0 {
		m := copy(ready, ready[i:])

		for i = m; i < n; i++ {
			ready[i] = nil
		}
		wp.readyToWork = ready[:m]

	}
	wp.Unlock()

	// 最后别忘了 解散那个worker channel!!!
	tmp := *scratch
	for i := range tmp {
		// 收到nil会退出
		close(tmp[i].ch)
		tmp[i] = nil
	}
}
