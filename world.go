package ecs

type WorldOption func(*World)

// NewWorld constructs a world with default registries and providers.
func NewWorld(opts ...WorldOption) *World {
	w := &World{
		registry:  NewEntityRegistry(),
		storage:   newStorageProvider(),
		resources: newResourceContainer(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// WithEntityRegistry overrides the default registry.
func WithEntityRegistry(registry *EntityRegistry) WorldOption {
	return func(w *World) {
		if registry != nil {
			w.registry = registry
		}
	}
}

// WithStorageProvider overrides the default storage provider.
func WithStorageProvider(provider StorageProvider) WorldOption {
	return func(w *World) {
		if provider != nil {
			w.storage = provider
		}
	}
}

// WithResourceContainer overrides the default resource container.
func WithResourceContainer(container ResourceContainer) WorldOption {
	return func(w *World) {
		if container != nil {
			w.resources = container
		}
	}
}

// Registry exposes the backing entity registry.
func (w *World) Registry() *EntityRegistry {
	return w.registry
}

// Storage returns the storage provider used by the world.
func (w *World) Storage() StorageProvider {
	return w.storage
}

// Resources exposes the resource container.
func (w *World) Resources() ResourceContainer {
	return w.resources
}

// RegisterComponent allows callers to register component storage strategies.
func (w *World) RegisterComponent(t ComponentType, strategy StorageStrategy) error {
    return w.storage.RegisterComponent(t, strategy)
}

// ViewComponent retrieves a component view by type.
func (w *World) ViewComponent(t ComponentType) (ComponentView, error) {
    return w.storage.View(t)
}

// ApplyCommands executes deferred commands against the world.
func (w *World) ApplyCommands(commands []Command) error {
    return w.storage.Apply(w, commands)
}
