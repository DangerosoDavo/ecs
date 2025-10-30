package ecs

import (
	"fmt"
	"sync"
)

// EntityID identifies an entity and encodes a generation for stale-handle detection.
type EntityID struct {
	index      uint32
	generation uint32
}

// Index returns the backing index of the entity.
func (id EntityID) Index() uint32 {
	return id.index
}

// Generation returns the generation counter associated with the entity.
func (id EntityID) Generation() uint32 {
	return id.generation
}

// IsZero reports whether the identifier is the zero value.
func (id EntityID) IsZero() bool {
	return id.index == 0 && id.generation == 0
}

// String renders the entity identifier for debugging purposes.
func (id EntityID) String() string {
	if id.IsZero() {
		return "EntityID(0:0)"
	}
	return fmt.Sprintf("EntityID(%d:%d)", id.index, id.generation)
}

// EntityIDFromParts constructs an identifier from raw components.
func EntityIDFromParts(index, generation uint32) EntityID {
	return EntityID{index: index, generation: generation}
}

// NewEntityRegistry constructs an empty registry.
func NewEntityRegistry() *EntityRegistry {
	return &EntityRegistry{}
}

// EntityRegistry coordinates entity allocation and recycling.
type EntityRegistry struct {
	mu          sync.Mutex
	generations []uint32
	free        []uint32
	alive       uint32
}

// Create issues a new entity identifier, recycling slots when possible.
func (r *EntityRegistry) Create() EntityID {
	r.mu.Lock()
	defer r.mu.Unlock()

	var index uint32
	if n := len(r.free); n > 0 {
		index = r.free[n-1]
		r.free = r.free[:n-1]
	} else {
		index = uint32(len(r.generations))
		r.generations = append(r.generations, 0)
	}

	r.generations[index]++
	generation := r.generations[index]
	r.alive++
	return EntityID{index: index, generation: generation}
}

// CreateWithID allocates a specific entity identifier, enabling state restoration.
// The supplied identifier must be non-zero and refer to an unused or free slot.
func (r *EntityRegistry) CreateWithID(id EntityID) error {
	if id.IsZero() {
		return fmt.Errorf("ecs: cannot restore zero entity")
	}
	if id.Generation() == 0 {
		return fmt.Errorf("ecs: cannot restore entity with zero generation")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	idx := id.Index()
	gen := id.Generation()

	for idx >= uint32(len(r.generations)) {
		r.generations = append(r.generations, 0)
	}

	currentGen := r.generations[idx]

	freeIdx := -1
	for i, candidate := range r.free {
		if candidate == idx {
			freeIdx = i
			break
		}
	}
	slotFree := freeIdx >= 0

	if currentGen != 0 && !slotFree {
		if currentGen == gen {
			return fmt.Errorf("ecs: entity %d:%d already allocated", idx, gen)
		}
		return fmt.Errorf("ecs: entity slot %d in use (generation %d, requested %d)", idx, currentGen, gen)
	}

	if currentGen != 0 && slotFree && gen < currentGen {
		return fmt.Errorf("ecs: cannot restore entity slot %d with stale generation %d (current %d)", idx, gen, currentGen)
	}

	r.generations[idx] = gen
	if slotFree {
		r.free = append(r.free[:freeIdx], r.free[freeIdx+1:]...)
	}
	r.alive++
	return nil
}

// Destroy releases the entity identifier, returning true when successful.
func (r *EntityRegistry) Destroy(id EntityID) bool {
	if id.IsZero() {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isAliveLocked(id) {
		return false
	}

	r.alive--
	r.generations[id.index]++
	r.free = append(r.free, id.index)
	return true
}

// IsAlive reports whether the identifier refers to a currently allocated entity.
func (r *EntityRegistry) IsAlive(id EntityID) bool {
	if id.IsZero() {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isAliveLocked(id)
}

// Count returns the number of live entities.
func (r *EntityRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int(r.alive)
}

func (r *EntityRegistry) isAliveLocked(id EntityID) bool {
	idx := id.index
	if idx >= uint32(len(r.generations)) {
		return false
	}
	return r.generations[idx] == id.generation
}
