package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/y7ut/ppool"
	"github.com/y7ut/ppool/option"
)

func DoExample() {
	// 注意,这里是使用的是全局的 Pool实例, 所以在 Close 之前不可以再次 Apply，
	// 因为在 Pool 运行过程中不会修改配置
	// 同理在 Close 之前使用 Do 就会继续使用全局的默认实例
	ppool.Apply(
		option.WithMaxWorkCount(3),
		option.WithTimeout(3*time.Second),
	)
	go func() {
		for i := 0; i < 10; i++ {
			// 阻塞直到开始工作
			ppool.Do(func(ctx context.Context) {
				r := rand.New(rand.NewSource(time.Now().UnixNano()))
				duration := time.Duration(1 + r.Intn(4))
				ticker := time.NewTicker(duration * time.Second)
				defer ticker.Stop()
				log.Printf("start work[%d], need: %d\n", i+1, duration)
				select {
				case <-ctx.Done():
					log.Printf("cancel work[%d], reason: %s", i+1, ctx.Err())
					return
				case <-ticker.C:
					log.Printf("done work[%d], speed: %d", i+1, duration)
					return
				}
			})
			log.Println("start once")
		}
		log.Println("finish to add!")
	}()

	log.Println("Stop after 5s")
	time.Sleep(5 * time.Second)
	start := time.Now()
	ppool.Close()

	log.Printf("stop the pool cost: %.2f s ", time.Since(start).Seconds())
}
