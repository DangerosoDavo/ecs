package ecs

import (
	"fmt"
)

type WorldOption func(*World)

// NewWorld constructs a world with default registries and providers.
func NewWorld(opts ...WorldOption) *World {
	w := &World{
		registry:       NewEntityRegistry(),
		storage:        newStorageProvider(),
		resources:      newResourceContainer(),
		changed:        make(map[EntityID]bool),
		disabled:       make(map[EntityID]map[ComponentType]bool),
		componentCfgs:  make(map[ComponentType]ComponentConfig),
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
// Optional ComponentOption arguments configure behavior such as
// WithNonDisableable() to lock a component type to always-on visibility.
func (w *World) RegisterComponent(t ComponentType, strategy StorageStrategy, opts ...ComponentOption) error {
	if err := w.storage.RegisterComponent(t, strategy); err != nil {
		return err
	}
	cfg := ComponentConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	w.mu.Lock()
	w.componentCfgs[t] = cfg
	w.mu.Unlock()
	return nil
}

// ComponentConfig returns the registered configuration for a component type.
// The second return value is false if the component has not been registered.
func (w *World) ComponentConfig(t ComponentType) (ComponentConfig, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	cfg, ok := w.componentCfgs[t]
	return cfg, ok
}

// ViewComponent retrieves a component view by type. The returned view filters
// out entities whose component has been disabled via DisableComponent/Entity.
// Iterate skips disabled entries; Get/Has return underlying data unchanged so
// callers that need to inspect disabled state (e.g. a decay system inspecting
// a paused Cargo) can do so without re-enabling the component.
func (w *World) ViewComponent(t ComponentType) (ComponentView, error) {
	inner, err := w.storage.View(t)
	if err != nil {
		return nil, err
	}
	return &filteredView{world: w, inner: inner, compType: t}, nil
}

// DestroyEntity removes all component data for the entity and then frees its
// registry slot. Prefer this over calling Registry().Destroy() directly to
// ensure component stores are cleaned up.
func (w *World) DestroyEntity(id EntityID) bool {
	w.storage.RemoveEntity(id)
	w.mu.Lock()
	delete(w.disabled, id)
	w.mu.Unlock()
	return w.registry.Destroy(id)
}

// ApplyCommands executes deferred commands against the world.
func (w *World) ApplyCommands(commands []Command) error {
	return w.storage.Apply(w, commands)
}

// MarkChanged flags an entity as modified this tick. Systems should call this
// after mutating a component through a pointer obtained via Get(). Commands
// (AddComponent, RemoveComponent) call this automatically.
//
// Takes the world mutex because ApplyCommands runs on whichever goroutine
// called it — the game tick loop AND async command workers both route
// writes through this path, and concurrent access to a plain map is an
// immediate fatal ("concurrent map writes" panic from the runtime).
func (w *World) MarkChanged(id EntityID) {
	w.mu.Lock()
	w.changed[id] = true
	w.mu.Unlock()
}

// Changed returns a snapshot copy of the current dirty set without clearing
// it. The copy lets callers iterate safely while other goroutines continue
// to mark entities changed.
func (w *World) Changed() map[EntityID]bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	snap := make(map[EntityID]bool, len(w.changed))
	for k, v := range w.changed {
		snap[k] = v
	}
	return snap
}

// DrainChanged returns the current dirty set and resets it. Designed for
// consumers like a sync system that process changes once per tick.
func (w *World) DrainChanged() map[EntityID]bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.changed
	w.changed = make(map[EntityID]bool)
	return c
}

// ClearChanged resets the dirty set without returning it.
func (w *World) ClearChanged() {
	w.mu.Lock()
	w.changed = make(map[EntityID]bool)
	w.mu.Unlock()
}

// --- Component disable/enable -----------------------------------------------

// ErrComponentNonDisableable is returned when a caller tries to disable a
// component type registered with WithNonDisableable().
var ErrComponentNonDisableable = fmt.Errorf("ecs: component type is non-disableable")

// DisableComponent hides a single component on an entity from Iterate-based
// system traversal. Get/Has still return the underlying data, so systems that
// need to inspect a disabled component (e.g. reading reserved cargo on a
// paused construction) can do so explicitly. Returns an error if the
// component type was registered as non-disableable.
func (w *World) DisableComponent(id EntityID, t ComponentType) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if cfg, ok := w.componentCfgs[t]; ok && cfg.NonDisableable {
		return ErrComponentNonDisableable
	}
	set, ok := w.disabled[id]
	if !ok {
		set = make(map[ComponentType]bool)
		w.disabled[id] = set
	}
	set[t] = true
	w.changed[id] = true
	return nil
}

// EnableComponent restores iteration visibility for a single component on an
// entity. No-op if the component was not disabled.
func (w *World) EnableComponent(id EntityID, t ComponentType) {
	w.mu.Lock()
	defer w.mu.Unlock()
	set, ok := w.disabled[id]
	if !ok {
		return
	}
	if _, had := set[t]; !had {
		return
	}
	delete(set, t)
	if len(set) == 0 {
		delete(w.disabled, id)
	}
	w.changed[id] = true
}

// IsComponentDisabled reports whether a component on an entity is currently
// hidden from Iterate. Returns false for unregistered types.
func (w *World) IsComponentDisabled(id EntityID, t ComponentType) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	set, ok := w.disabled[id]
	if !ok {
		return false
	}
	return set[t]
}

// DisableEntity disables every disableable component attached to the entity.
// Non-disableable types are skipped silently so callers can use this as a
// one-shot pause without enumerating components. Returns the list of types
// that were newly disabled so callers can log or restore state.
func (w *World) DisableEntity(id EntityID) []ComponentType {
	types := w.storage.ComponentsFor(id)
	if len(types) == 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var disabled []ComponentType
	set := w.disabled[id]
	for _, t := range types {
		if cfg, ok := w.componentCfgs[t]; ok && cfg.NonDisableable {
			continue
		}
		if set == nil {
			set = make(map[ComponentType]bool)
			w.disabled[id] = set
		}
		if !set[t] {
			set[t] = true
			disabled = append(disabled, t)
		}
	}
	if len(disabled) > 0 {
		w.changed[id] = true
	}
	return disabled
}

// EnableEntity re-enables every disabled component on the entity.
func (w *World) EnableEntity(id EntityID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.disabled[id]; !ok {
		return
	}
	delete(w.disabled, id)
	w.changed[id] = true
}

// DisabledComponents returns a snapshot of currently disabled component types
// for an entity. Returns nil if none are disabled.
func (w *World) DisabledComponents(id EntityID) []ComponentType {
	w.mu.RLock()
	defer w.mu.RUnlock()
	set, ok := w.disabled[id]
	if !ok || len(set) == 0 {
		return nil
	}
	out := make([]ComponentType, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	return out
}

// filteredView wraps an inner ComponentView so Iterate skips entities whose
// component has been disabled on the World. Get/Has/Len pass through to the
// underlying store, preserving direct inspection of disabled data.
type filteredView struct {
	world    *World
	inner    ComponentView
	compType ComponentType
}

func (v *filteredView) ComponentType() ComponentType { return v.inner.ComponentType() }
func (v *filteredView) Len() int                     { return v.inner.Len() }
func (v *filteredView) Has(id EntityID) bool         { return v.inner.Has(id) }
func (v *filteredView) Get(id EntityID) (any, bool)  { return v.inner.Get(id) }

func (v *filteredView) Iterate(fn func(EntityID, any) bool) {
	v.inner.Iterate(func(id EntityID, val any) bool {
		if v.world.IsComponentDisabled(id, v.compType) {
			return true
		}
		return fn(id, val)
	})
}
