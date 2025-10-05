package ecs

import "errors"

var (
	// ErrComponentAlreadyRegistered indicates an attempt to register the same component twice.
	ErrComponentAlreadyRegistered = errors.New("ecs: component already registered")
	// ErrComponentNotRegistered signals lookup on an unknown component type.
	ErrComponentNotRegistered = errors.New("ecs: component not registered")
	// ErrNilStorageStrategy is returned when storage registration receives a nil strategy.
	ErrNilStorageStrategy = errors.New("ecs: nil storage strategy")
	// ErrNilComponentStore is returned when a strategy produces a nil store.
	ErrNilComponentStore = errors.New("ecs: strategy returned nil store")
	// ErrWorkerPoolClosed indicates jobs cannot be submitted because the pool closed.
	ErrWorkerPoolClosed = errors.New("ecs: worker pool closed")
	// ErrAsyncWritesNotSupported indicates an async work group attempted to mutate components.
	ErrAsyncWritesNotSupported = errors.New("ecs: async work group cannot perform component writes")
	// ErrAsyncSystemNotAllowed indicates a system opted out of async execution.
	ErrAsyncSystemNotAllowed = errors.New("ecs: system does not allow async execution")
	// ErrDuplicateWriteAccess indicates conflicting write access within a work group.
	ErrDuplicateWriteAccess = errors.New("ecs: duplicate write access to component in work group")
	// ErrDuplicateResourceWriteAccess indicates conflicting resource write claims.
	ErrDuplicateResourceWriteAccess = errors.New("ecs: duplicate write access to resource in work group")
	// ErrAsyncResourceWritesNotSupported indicates async groups attempted to mutate resources.
	ErrAsyncResourceWritesNotSupported = errors.New("ecs: async work group cannot perform resource writes")
)
