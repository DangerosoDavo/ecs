package ecs

import (
	"context"
	"sync"
)

type workerPool struct {
	size   int
	jobs   chan jobRequest
	closed chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

type jobRequest struct {
	ctx    context.Context
	fn     func(context.Context) jobResult
	result chan jobResult
}

type jobResult struct {
	err      error
	commands []Command
	summary  *workGroupRunSummary
}

func (r jobResult) Err() error { return r.err }

func (r jobResult) Commands() []Command { return r.commands }

func (r jobResult) Summary() *workGroupRunSummary { return r.summary }

func newWorkerPool(size int) *workerPool {
	if size <= 0 {
		return nil
	}
	p := &workerPool{
		size:   size,
		jobs:   make(chan jobRequest),
		closed: make(chan struct{}),
	}
	p.start()
	return p
}

func (p *workerPool) start() {
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

func (p *workerPool) worker() {
	defer p.wg.Done()
	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			p.execute(job)
		case <-p.closed:
			return
		}
	}
}

func (p *workerPool) execute(job jobRequest) {
	if job.result == nil {
		return
	}
	defer close(job.result)
	if job.fn == nil {
		job.result <- jobResult{}
		return
	}
	select {
	case <-job.ctx.Done():
		job.result <- jobResult{err: job.ctx.Err()}
	default:
		job.result <- job.fn(job.ctx)
	}
}

func (p *workerPool) Submit(ctx context.Context, fn func(context.Context) jobResult) *jobHandle {
	if fn == nil {
		ch := make(chan jobResult, 1)
		ch <- jobResult{}
		close(ch)
		return &jobHandle{result: ch}
	}
	if p == nil {
		ch := make(chan jobResult, 1)
		ch <- fn(ctx)
		close(ch)
		return &jobHandle{result: ch}
	}
	result := make(chan jobResult, 1)
	job := jobRequest{ctx: ctx, fn: fn, result: result}
	select {
	case <-p.closed:
		result <- jobResult{err: ErrWorkerPoolClosed}
		close(result)
		return &jobHandle{result: result}
	case <-ctx.Done():
		result <- jobResult{err: ctx.Err()}
		close(result)
		return &jobHandle{result: result}
	default:
	}
	if safeSendJob(p.jobs, job) {
		return &jobHandle{result: result}
	}
	result <- jobResult{err: ErrWorkerPoolClosed}
	close(result)
	return &jobHandle{result: result}
}

func (p *workerPool) Close() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		close(p.closed)
		close(p.jobs)
	})
	p.wg.Wait()
}

type jobHandle struct {
	result chan jobResult
}

func (h *jobHandle) Wait() jobResult {
	if h == nil || h.result == nil {
		return jobResult{}
	}
	res, ok := <-h.result
	if !ok {
		return jobResult{}
	}
	return res
}

func safeSendJob(ch chan jobRequest, job jobRequest) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	ch <- job
	return true
}
