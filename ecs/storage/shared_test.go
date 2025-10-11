package storage

import (
	"testing"

	ecs "github.com/DangerosoDavo/ecs"
)

type GameStats struct {
	Health       int
	AttackDamage int
	Defense      int
}

func TestSharedStorage_BasicOperations(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats")

	if store.ComponentType() != "Stats" {
		t.Errorf("expected component type 'Stats', got %s", store.ComponentType())
	}

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)

	stats := GameStats{Health: 100, AttackDamage: 25, Defense: 10}

	// Add same stats to both entities
	if err := store.Set(entity1, stats); err != nil {
		t.Fatalf("failed to set component: %v", err)
	}

	if err := store.Set(entity2, stats); err != nil {
		t.Fatalf("failed to set component: %v", err)
	}

	// Both entities should have the component
	if !store.Has(entity1) {
		t.Error("entity1 should have component")
	}
	if !store.Has(entity2) {
		t.Error("entity2 should have component")
	}

	// Verify values
	val1, ok := store.Get(entity1)
	if !ok {
		t.Fatal("entity1 component not found")
	}
	if val1.(GameStats).Health != 100 {
		t.Errorf("expected health 100, got %d", val1.(GameStats).Health)
	}

	val2, ok := store.Get(entity2)
	if !ok {
		t.Fatal("entity2 component not found")
	}
	if val2.(GameStats).AttackDamage != 25 {
		t.Errorf("expected attack 25, got %d", val2.(GameStats).AttackDamage)
	}
}

func TestSharedStorage_ValueSharing(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats").(*sharedStore)

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)
	entity3 := ecs.EntityIDFromParts(3, 1)

	zombieStats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}
	playerStats := GameStats{Health: 100, AttackDamage: 25, Defense: 15}

	// Two zombies with same stats
	store.Set(entity1, zombieStats)
	store.Set(entity2, zombieStats)

	// One player with different stats
	store.Set(entity3, playerStats)

	// Check that zombies share the same value instance
	stats := store.Stats()
	if stats.EntityCount != 3 {
		t.Errorf("expected 3 entities, got %d", stats.EntityCount)
	}
	if stats.UniqueValueCount != 2 {
		t.Errorf("expected 2 unique values, got %d", stats.UniqueValueCount)
	}

	// Sharing ratio should be 1.5 (3 entities / 2 unique values)
	expectedRatio := 1.5
	if stats.SharingRatio != expectedRatio {
		t.Errorf("expected sharing ratio %.2f, got %.2f", expectedRatio, stats.SharingRatio)
	}
}

func TestSharedStorage_RemoveDecrementsRefCount(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats").(*sharedStore)

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)

	stats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	// Both entities share the same stats
	store.Set(entity1, stats)
	store.Set(entity2, stats)

	// Should have 1 unique value with refcount 2
	if len(store.valueToData) != 1 {
		t.Errorf("expected 1 unique value, got %d", len(store.valueToData))
	}

	// Get the value ID
	var valueID uint32
	for id := range store.valueToData {
		valueID = id
		break
	}

	// Check ref count
	if store.valueToData[valueID].refCount != 2 {
		t.Errorf("expected refcount 2, got %d", store.valueToData[valueID].refCount)
	}

	// Remove from entity1
	if !store.Remove(entity1) {
		t.Error("failed to remove component from entity1")
	}

	// Ref count should decrease to 1
	if store.valueToData[valueID].refCount != 1 {
		t.Errorf("expected refcount 1 after removal, got %d", store.valueToData[valueID].refCount)
	}

	// Remove from entity2
	store.Remove(entity2)

	// Value should be completely removed now
	if len(store.valueToData) != 0 {
		t.Errorf("expected 0 unique values after all removals, got %d", len(store.valueToData))
	}
}

func TestSharedStorage_UpdateValue(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats").(*sharedStore)

	entity1 := ecs.EntityIDFromParts(1, 1)

	stats1 := GameStats{Health: 50, AttackDamage: 10, Defense: 5}
	stats2 := GameStats{Health: 100, AttackDamage: 20, Defense: 10}

	// Set initial value
	store.Set(entity1, stats1)

	// Should have 1 unique value
	if len(store.valueToData) != 1 {
		t.Errorf("expected 1 unique value, got %d", len(store.valueToData))
	}

	// Update to new value
	store.Set(entity1, stats2)

	// Should still have 1 unique value (old one was garbage collected)
	if len(store.valueToData) != 1 {
		t.Errorf("expected 1 unique value after update, got %d", len(store.valueToData))
	}

	// Verify new value
	val, ok := store.Get(entity1)
	if !ok {
		t.Fatal("component not found after update")
	}
	if val.(GameStats).Health != 100 {
		t.Errorf("expected updated health 100, got %d", val.(GameStats).Health)
	}
}

func TestSharedStorage_Iterate(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats")

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)
	entity3 := ecs.EntityIDFromParts(3, 1)

	stats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	store.Set(entity1, stats)
	store.Set(entity2, stats)
	store.Set(entity3, stats)

	// Iterate and count
	count := 0
	store.Iterate(func(id ecs.EntityID, component any) bool {
		count++
		s := component.(GameStats)
		if s.Health != 50 {
			t.Errorf("expected health 50, got %d", s.Health)
		}
		return true
	})

	if count != 3 {
		t.Errorf("expected to iterate over 3 entities, got %d", count)
	}
}

func TestSharedStorage_IterateEarlyExit(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats")

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)
	entity3 := ecs.EntityIDFromParts(3, 1)

	stats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	store.Set(entity1, stats)
	store.Set(entity2, stats)
	store.Set(entity3, stats)

	// Iterate but stop after 2
	count := 0
	store.Iterate(func(id ecs.EntityID, component any) bool {
		count++
		return count < 2
	})

	if count != 2 {
		t.Errorf("expected iteration to stop at 2, got %d", count)
	}
}

func TestSharedStorage_Clear(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats")

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)

	stats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	store.Set(entity1, stats)
	store.Set(entity2, stats)

	if store.Len() != 2 {
		t.Errorf("expected length 2, got %d", store.Len())
	}

	store.Clear()

	if store.Len() != 0 {
		t.Errorf("expected length 0 after clear, got %d", store.Len())
	}

	if store.Has(entity1) {
		t.Error("entity1 should not have component after clear")
	}
}

func TestSharedStorage_ZeroEntity(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats")

	zeroEntity := ecs.EntityID{}
	stats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	err := store.Set(zeroEntity, stats)
	if err == nil {
		t.Error("expected error when setting zero entity")
	}
}

func TestSharedStorage_MemoryEfficiency(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats").(*sharedStore)

	// Create 1000 entities with the same stats
	commonStats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	for i := 0; i < 1000; i++ {
		entity := ecs.EntityIDFromParts(uint32(i+1), 1)
		store.Set(entity, commonStats)
	}

	// Should only have 1 unique value despite 1000 entities
	stats := store.Stats()
	if stats.EntityCount != 1000 {
		t.Errorf("expected 1000 entities, got %d", stats.EntityCount)
	}
	if stats.UniqueValueCount != 1 {
		t.Errorf("expected 1 unique value, got %d", stats.UniqueValueCount)
	}
	if stats.SharingRatio != 1000.0 {
		t.Errorf("expected sharing ratio 1000, got %.2f", stats.SharingRatio)
	}

	// Now add some entities with different stats
	rareStats1 := GameStats{Health: 100, AttackDamage: 25, Defense: 15}
	rareStats2 := GameStats{Health: 75, AttackDamage: 15, Defense: 10}

	store.Set(ecs.EntityIDFromParts(1001, 1), rareStats1)
	store.Set(ecs.EntityIDFromParts(1002, 1), rareStats2)

	stats = store.Stats()
	if stats.UniqueValueCount != 3 {
		t.Errorf("expected 3 unique values, got %d", stats.UniqueValueCount)
	}
}

func TestSharedStorage_DifferentStructs(t *testing.T) {
	strategy := NewSharedStrategy()
	store := strategy.NewStore("Stats").(*sharedStore)

	entity1 := ecs.EntityIDFromParts(1, 1)
	entity2 := ecs.EntityIDFromParts(2, 1)

	// Two stats with same values should be deduplicated
	stats1 := GameStats{Health: 50, AttackDamage: 10, Defense: 5}
	stats2 := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

	store.Set(entity1, stats1)
	store.Set(entity2, stats2)

	// Should share the same underlying value
	if len(store.valueToData) != 1 {
		t.Errorf("expected 1 unique value for identical structs, got %d", len(store.valueToData))
	}
}
