package ppool

import (
	"context"

	"github.com/y7ut/ppool/option"
)

// CommonWorker 通用的Worker
type CommonWorker struct {
	workFunc func(context.Context)
}

// Work 通用的Work的实现
func (c *CommonWorker) Work(ctx context.Context) {
	c.workFunc(ctx)
}

// commonPool 全局的通用的协程池
var commonPool *Pool[*CommonWorker]

// commonOptions 全局的通用的配置
var commonOptions []*option.Option

// Apply 应用全局配置
func Apply(options ...*option.Option) {
	commonOptions = append(commonOptions, options...)
}

// Do 使用全局的通用的协程池，阻塞直到开始工作
// 如果之前关闭过，会创建一个新的
func Do(f func(context.Context)) (err error) {
	if commonPool == nil {
		commonPool, err = CreatePool[*CommonWorker](commonOptions...)
		if err != nil {
			return err
		}
	}
	return commonPool.Do(&CommonWorker{workFunc: f})
}

// Serve 使用全局的通用的协程池，尝试创建一个任务，如果成功返回true，失败返回false， 如果有pool已停止则返回错误
func Serve(f func(context.Context)) (ok bool, err error) {
	if commonPool == nil {
		commonPool, err = CreatePool[*CommonWorker](commonOptions...)
		if err != nil {
			return false, err
		}
	}
	return commonPool.Serve(&CommonWorker{workFunc: f})
}

// Close 关闭全局的通用的协程池，会等待所有的工作协程退出
// 下次使用会重新创建
func Close() {
	if commonPool != nil {
		commonPool.Stop()
		commonPool = nil
	}
}
