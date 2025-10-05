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
	writable.Remove(c.entity)
	return nil
}

var (
	_ Command = createEntityCommand{}
	_ Command = destroyEntityCommand{}
	_ Command = addComponentCommand{}
	_ Command = removeComponentCommand{}
)
