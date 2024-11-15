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

func (m *DoSomeThing) Work(ctx context.Context) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	duration := time.Duration(1 + r.Intn(4))
	log.Printf("[%s] start, need: %d\n", m.name, duration)
	time.Sleep(duration * time.Second)
	log.Printf("[%s] done, speed: %d", m.name, duration)
}

func WorkAndPooExample() {
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
