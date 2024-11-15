package ppool

import "context"

// WorkAble
type WorkAble interface {
	Work(ctx context.Context)
}
