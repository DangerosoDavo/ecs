package ecs

import "sort"

// IterateOrdered visits by entity index then generation, independently of a
// storage strategy's iteration order. Callbacks must not structurally mutate
// the store during iteration; defer commands instead.
func IterateOrdered(view ComponentView, fn func(EntityID, any) bool) {
	ids := make([]EntityID, 0, view.Len())
	view.Iterate(func(id EntityID, _ any) bool { ids = append(ids, id); return true })
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].Index() == ids[j].Index() {
			return ids[i].Generation() < ids[j].Generation()
		}
		return ids[i].Index() < ids[j].Index()
	})
	for _, id := range ids {
		if value, ok := view.Get(id); ok && !fn(id, value) {
			return
		}
	}
}

// Query iterates the smallest component bucket and joins the others by ID.
// Disabled components are excluded from every side of the join. values is
// reused for each callback: copy it if it must outlive the callback. Empty
// queries visit nothing. ordered enables stable entity ordering for simulation
// operations whose result depends on order.
func Query(w *World, types []ComponentType, ordered bool, fn func(EntityID, []any) bool) error {
	if len(types) == 0 {
		return nil
	}
	views := make([]ComponentView, len(types))
	values := make([]any, len(types))
	smallest := 0
	for i, t := range types {
		view, err := w.ViewComponent(t)
		if err != nil {
			return err
		}
		views[i] = view
		if view.Len() < views[smallest].Len() {
			smallest = i
		}
	}
	visit := func(id EntityID, _ any) bool {
		if !w.Registry().IsAlive(id) {
			return true
		}
		for i, view := range views {
			if w.IsComponentDisabled(id, types[i]) {
				return true
			}
			value, ok := view.Get(id)
			if !ok {
				return true
			}
			values[i] = value
		}
		return fn(id, values)
	}
	if ordered {
		IterateOrdered(views[smallest], visit)
	} else {
		views[smallest].Iterate(visit)
	}
	return nil
}
