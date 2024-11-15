package option

import (
	"log"
	"time"
)

// PiplineWorkingOption 工作池配置
type PiplineWorkingOption struct {
	timeout, maxIdleWorkerDuration time.Duration
	maxWorkCount                   int
	logger                         *log.Logger
}

// GetTimeOutDuration 获取超时时间
func (o *PiplineWorkingOption) GetTimeOutDuration() time.Duration {
	return o.timeout
}

// GetMaxIdleWorkerDuration 获取最大的空闲时间
func (o *PiplineWorkingOption) GetMaxIdleWorkerDuration() time.Duration {
	return o.maxIdleWorkerDuration
}

// GetMaxWorkCount 获取最大的工作数量
func (o *PiplineWorkingOption) GetMaxWorkCount() int {
	return o.maxWorkCount
}

// GetLogger 获取日志
func (o *PiplineWorkingOption) GetLogger() *log.Logger {
	return o.logger
}

// Option 选项
type Option struct {
	Apply func(option *PiplineWorkingOption)
}

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) *Option {
	return &Option{
		Apply: func(option *PiplineWorkingOption) {
			option.timeout = timeout
		},
	}
}

// WithMaxIdleWorkerDuration 设置最大的空闲时间
func WithMaxIdleWorkerDuration(maxIdleWorkerDuration time.Duration) *Option {
	return &Option{
		Apply: func(option *PiplineWorkingOption) {
			option.maxIdleWorkerDuration = maxIdleWorkerDuration
		},
	}
}

// WithMaxWorkCount 设置最大的工作数量
func WithMaxWorkCount(maxWorkCount int) *Option {
	return &Option{
		Apply: func(option *PiplineWorkingOption) {
			option.maxWorkCount = maxWorkCount
		},
	}
}

// WithLogger 设置日志
func WithLogger(logger *log.Logger) *Option {
	return &Option{
		Apply: func(option *PiplineWorkingOption) {
			option.logger = logger
		},
	}
}

// DefaultOption 获取默认配置
func DefaultOption() *PiplineWorkingOption {
	return &PiplineWorkingOption{
		timeout:               5 * time.Second,
		maxIdleWorkerDuration: 5 * time.Second,
		maxWorkCount:          10,
		logger:                log.Default(),
	}
}
