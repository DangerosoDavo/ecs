package ecs

import "sync"

// CommandBuffer accumulates deferred commands during a scheduler tick.
type CommandBuffer struct {
	commands []Command
}

// NewCommandBuffer creates an empty buffer.
func NewCommandBuffer() *CommandBuffer {
	return &CommandBuffer{}
}

// Len reports how many commands are queued.
func (b *CommandBuffer) Len() int {
	return len(b.commands)
}

// Push appends a command to the buffer.
func (b *CommandBuffer) Push(cmd Command) {
	if cmd == nil {
		return
	}
	b.commands = append(b.commands, cmd)
}

// Drain returns queued commands and resets the buffer.
func (b *CommandBuffer) Drain() []Command {
	drained := b.commands
	b.commands = nil
	return drained
}

// Snapshot returns the current command count so callers can restore later.
func (b *CommandBuffer) Snapshot() int {
	return len(b.commands)
}

// Restore truncates the command buffer back to the provided snapshot.
func (b *CommandBuffer) Restore(snapshot int) {
	if snapshot < 0 {
		snapshot = 0
	}
	if snapshot >= len(b.commands) {
		return
	}
	b.commands = b.commands[:snapshot]
}

// CommandBufferPool reuses buffers to reduce allocations.
type CommandBufferPool struct {
	pool sync.Pool
}

// NewCommandBufferPool constructs a pool that returns fresh buffers.
func NewCommandBufferPool() *CommandBufferPool {
	p := &CommandBufferPool{}
	p.pool.New = func() any { return NewCommandBuffer() }
	return p
}

// Get retrieves a buffer from the pool.
func (p *CommandBufferPool) Get() *CommandBuffer {
	return p.pool.Get().(*CommandBuffer)
}

// Put returns a buffer to the pool after clearing it.
func (p *CommandBufferPool) Put(buf *CommandBuffer) {
	if buf == nil {
		return
	}
	buf.Drain()
	p.pool.Put(buf)
}
