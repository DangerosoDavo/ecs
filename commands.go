package ecs

import "fmt"

// NewCreateEntityCommand enqueues a new entity creation. If target is non-nil it receives the allocated ID.
func NewCreateEntityCommand(target *EntityID) Command {
	return createEntityCommand{target: target}
}

// NewDestroyEntityCommand enqueues an entity deletion.
func NewDestroyEntityCommand(id EntityID) Command {
	return destroyEntityCommand{entity: id}
}

// NewAddComponentCommand enqueues a component addition.
func NewAddComponentCommand(id EntityID, component ComponentType, value any) Command {
	return addComponentCommand{entity: id, component: component, value: value}
}

// NewRemoveComponentCommand enqueues a component removal.
func NewRemoveComponentCommand(id EntityID, component ComponentType) Command {
	return removeComponentCommand{entity: id, component: component}
}

// NewRestoreEntityCommand enqueues restoration of a specific entity identifier.
func NewRestoreEntityCommand(id EntityID) Command {
	return restoreEntityCommand{id: id}
}

// NewDisableComponentCommand enqueues disabling a single component on an entity.
// Systems iterating the affected component type will skip the entity until it
// is re-enabled. Fails if the component type is non-disableable.
func NewDisableComponentCommand(id EntityID, component ComponentType) Command {
	return disableComponentCommand{entity: id, component: component}
}

// NewEnableComponentCommand enqueues re-enabling a single component on an entity.
func NewEnableComponentCommand(id EntityID, component ComponentType) Command {
	return enableComponentCommand{entity: id, component: component}
}

// NewDisableEntityCommand enqueues disabling every disableable component on
// the entity in a single step. Non-disableable types are left untouched.
func NewDisableEntityCommand(id EntityID) Command {
	return disableEntityCommand{entity: id}
}

// NewEnableEntityCommand enqueues re-enabling every disabled component on the
// entity.
func NewEnableEntityCommand(id EntityID) Command {
	return enableEntityCommand{entity: id}
}

type createEntityCommand struct {
	target *EntityID
}

type destroyEntityCommand struct {
	entity EntityID
}

type addComponentCommand struct {
	entity    EntityID
	component ComponentType
	value     any
}

type removeComponentCommand struct {
	entity    EntityID
	component ComponentType
}

type restoreEntityCommand struct {
	id EntityID
}

type disableComponentCommand struct {
	entity    EntityID
	component ComponentType
}

type enableComponentCommand struct {
	entity    EntityID
	component ComponentType
}

type disableEntityCommand struct {
	entity EntityID
}

type enableEntityCommand struct {
	entity EntityID
}

func (c createEntityCommand) Apply(world *World) error {
	id := world.registry.Create()
	if c.target != nil {
		*c.target = id
	}
	return nil
}

func (c destroyEntityCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: destroy zero entity")
	}
	// Remove all component data before freeing the registry slot.
	// Must happen first while the entity is still alive so generation-matched
	// Remove calls succeed in component stores.
	world.storage.RemoveEntity(c.entity)
	if !world.registry.Destroy(c.entity) {
		return fmt.Errorf("ecs: destroy stale entity %v", c.entity)
	}
	return nil
}

func (c addComponentCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: add component to zero entity")
	}
	store, err := world.storage.View(c.component)
	if err != nil {
		return err
	}
	writable, ok := store.(ComponentStore)
	if !ok {
		return fmt.Errorf("ecs: component %s is not writable", c.component)
	}
	world.MarkChanged(c.entity)
	return writable.Set(c.entity, c.value)
}

func (c removeComponentCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: remove component from zero entity")
	}
	store, err := world.storage.View(c.component)
	if err != nil {
		return err
	}
	writable, ok := store.(ComponentStore)
	if !ok {
		return fmt.Errorf("ecs: component %s is not writable", c.component)
	}
	world.MarkChanged(c.entity)
	writable.Remove(c.entity)
	return nil
}

func (c restoreEntityCommand) Apply(world *World) error {
	if c.id.IsZero() {
		return fmt.Errorf("ecs: restore zero entity")
	}
	return world.registry.CreateWithID(c.id)
}

func (c disableComponentCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: disable component on zero entity")
	}
	return world.DisableComponent(c.entity, c.component)
}

func (c enableComponentCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: enable component on zero entity")
	}
	world.EnableComponent(c.entity, c.component)
	return nil
}

func (c disableEntityCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: disable zero entity")
	}
	world.DisableEntity(c.entity)
	return nil
}

func (c enableEntityCommand) Apply(world *World) error {
	if c.entity.IsZero() {
		return fmt.Errorf("ecs: enable zero entity")
	}
	world.EnableEntity(c.entity)
	return nil
}

var (
	_ Command = createEntityCommand{}
	_ Command = destroyEntityCommand{}
	_ Command = addComponentCommand{}
	_ Command = removeComponentCommand{}
	_ Command = restoreEntityCommand{}
	_ Command = disableComponentCommand{}
	_ Command = enableComponentCommand{}
	_ Command = disableEntityCommand{}
	_ Command = enableEntityCommand{}
)
