# Pp0ol

`Pp0ol` 是一个使用起来思想负担很小的协程池，只需要简单的设定最大协程的数量，单次操作的超时时间等配置，就可以立即开始使用了。
 🤓👆 (就是这么写的)

## 快速开始

我们可以通过提供的一个全局的Do方法，来创建一个协程池中的协程，Do方法会阻塞直到任务创建成功
还可以通过Serve方法，来创建，这时会立即返现是否创建成功
接下来协程池中定时回收资源， 我们还可以通过Close方法, 关闭并重置Pool。

注意：Close方法同样会阻塞等待直到Pool中的协程全部执行完毕或者超时.

```go

package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "time"

    "github.com/y7ut/ppool"
    "github.com/y7ut/ppool/option"
)

func Example() {
    // 设定最大同时运行的协程数量为5
    // 任务的超时时间为3秒
    ppool.Apply(
        option.WithMaxWorkCount(5),
        option.WithTimeout(3*time.Second),
    )

    // 使用 Do 方法创建一个协程，尝试在协程池中执行，会阻塞直到开始执行
    ppool.Do(doSomeThing)
    // 使用 Serve 方法尝试在协程池中执行任务，如果没有超过最大运行数量会返回false, 错误是空
    // 如果协程池被关闭了，则仍会返回 false, 错误会提示 pool has stoped
    ok, err := ppool.Serve(doSomeThing)
    if err != nil {
        panic(err)
    }
    if ok {
        fmt.Println("serve success")
    }
    time.Sleep(5 * time.Second)
    ppool.Close()
    fmt.Println("ppool closed")
}

func doSomeThing(ctx context.Context) {
    r := rand.New(rand.NewSource(time.Now().UnixNano()))
    duration := time.Duration(1 + r.Intn(4))
    ticker := time.NewTicker(duration * time.Second)
    defer ticker.Stop()
    log.Printf("start work need: %d\n", duration)

    select {
    case <-ctx.Done():
        log.Printf("cancel work reason: %s", ctx.Err())
        return
    case <-ticker.C:
        log.Printf("done work speed: %d", duration)
        return
    }
}

```

## 使用自定义的协程池

当我们要使用自定义的配置，或者同时操作两个协程池时，仅仅使用全局的ppool已经无法满足需求。
我们可以自行维护自己的Worker和Pool, Worker只需要实现WorkAble接口即可，除此之外使用方法和默认的方式一致。

```go

package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "time"

    "github.com/y7ut/ppool"
    "github.com/y7ut/ppool/option"
)

var _ ppool.WorkAble = (*DoSomeThing)(nil)

type DoSomeThing struct {
    name string
}

// Work Worker通过上下文来控制超时，所以请自行实现超时的错误处理逻辑
func (m *DoSomeThing) Work(ctx context.Context) {
    r := rand.New(rand.NewSource(time.Now().UnixNano()))
    duration := time.Duration(1 + r.Intn(4))
    log.Printf("[%s] start, need: %d\n", m.name, duration)
    time.Sleep(duration * time.Second)
    log.Printf("[%s] done, speed: %d", m.name, duration)
}

func WorkerAndPooExample() {
    p, _ := ppool.CreatePool[*DoSomeThing](
        option.WithLogger(log.Default()),
        option.WithMaxIdleWorkerDuration(5*time.Second),
        option.WithMaxWorkCount(20),
        option.WithTimeout(5*time.Second),
    )

    go func() {
        for i := 0; i < 10; i++ {
            ok, err := p.Serve(&DoSomeThing{fmt.Sprintf("worker-%d", i)})
            if err != nil {
                log.Fatal(err)
            }
            if !ok {
                fmt.Printf("worker-%d is refused\n", i)
                continue
            }
        }
    }()
    time.Sleep(15 * time.Second)
    p.Stop()
}

```
