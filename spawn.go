package ecs

import "sort"

// NewSpawnEntityCommand creates an entity and attaches an entire component
// bundle using its allocated ID. It copies the map; component payloads must
// remain immutable until application. target is assigned only on success.
func NewSpawnEntityCommand(target *EntityID, components map[ComponentType]any) Command {
	values := make(map[ComponentType]any, len(components))
	for t, value := range components {
		values[t] = value
	}
	return spawnEntityCommand{target: target, components: values}
}

type spawnEntityCommand struct {
	target     *EntityID
	components map[ComponentType]any
}

func (c spawnEntityCommand) Apply(w *World) error {
	types := make([]ComponentType, 0, len(c.components))
	stores := make(map[ComponentType]ComponentStore, len(c.components))
	for t := range c.components {
		view, err := w.storage.View(t)
		if err != nil {
			return err
		}
		store, ok := view.(ComponentStore)
		if !ok {
			return ErrNilComponentStore
		}
		types = append(types, t)
		stores[t] = store
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	id := w.registry.Create()
	for _, t := range types {
		if err := stores[t].Set(id, c.components[t]); err != nil {
			w.storage.RemoveEntity(id)
			w.registry.Destroy(id)
			return err
		}
	}
	w.MarkChanged(id)
	if c.target != nil {
		*c.target = id
	}
	return nil
}
