# Shared Components Guide

## Overview

Shared components allow multiple entities to reference the same component instance in memory. This is a powerful optimization technique for scenarios where many entities have identical component values.

## When to Use Shared Components

### Good Use Cases

1. **Entity Archetypes/Templates**
   - All zombies share the same base stats
   - All walls share the same physics properties
   - All trees share the same resource yield values

2. **Configuration Data**
   - Weapon stats shared across all instances of a weapon type
   - NPC behavior profiles shared across NPC types
   - Difficulty settings shared across game modes

3. **Large-Scale Simulations**
   - 10,000 soldiers with identical stats
   - 1,000 buildings of the same type
   - Massive particle systems with shared properties

### Bad Use Cases

1. **Frequently Modified Data**
   - Current health (changes per entity)
   - Position, velocity (unique per entity)
   - Inventory contents (entity-specific)

2. **Entity-Specific State**
   - AI decision state
   - Animation state
   - Cooldown timers

## Memory Efficiency

Shared components dramatically reduce memory usage for identical data:

```go
// Scenario: 1,000 entities with identical stats

// Dense Storage (traditional)
// Memory: 1,000 × sizeof(GameStats) = ~40KB (assuming 40 bytes per struct)
world.RegisterComponent("Stats", ecsstorage.NewDenseStrategy())
for i := 0; i < 1000; i++ {
    world.ApplyCommands([]ecs.Command{
        ecs.NewAddComponentCommand(entityID, "Stats", stats),
    })
}

// Shared Storage (optimized)
// Memory: 1 × sizeof(GameStats) + 1,000 × sizeof(EntityID->ValueID map entry) = ~40 bytes + 8KB = ~8KB
// Savings: ~80% memory reduction
world.RegisterComponent("Stats", ecsstorage.NewSharedStrategy())
for i := 0; i < 1000; i++ {
    world.ApplyCommands([]ecs.Command{
        ecs.NewAddComponentCommand(entityID, "Stats", stats),
    })
}
```

## Design Patterns

### Pattern 1: BaseStats + CurrentStats (RECOMMENDED for Games)

This is the recommended pattern for game statistics where you need both shared base values and mutable per-entity state.

**Components:**
- `BaseStats` (SharedStorage): Immutable base values shared across entity archetypes
- `CurrentStats` (DenseStorage): Mutable runtime values unique to each entity
- `StatModifiers` (DenseStorage): Time-limited buffs/debuffs unique to each entity

**Benefits:**
- ✓ Memory efficient: Base stats shared across entity types
- ✓ Mutable state: Each entity can have unique current health, status, etc.
- ✓ Flexible modifiers: Temporary buffs/debuffs per entity
- ✓ Multiple systems can access same entity's different stat components

**Example:**
```go
// Components
type BaseStats struct {
    MaxHealth        int
    BaseAttackDamage int
    BaseDefense      int
}

type CurrentStats struct {
    CurrentHealth int
    IsDead        bool
}

type StatModifiers struct {
    Modifiers []StatModifier
}

// Registration
world.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())
world.RegisterComponent("CurrentStats", ecsstorage.NewDenseStrategy())
world.RegisterComponent("StatModifiers", ecsstorage.NewDenseStrategy())

// Create 100 zombies - all share BaseStats, each has unique CurrentStats
zombieBaseStats := BaseStats{MaxHealth: 50, BaseAttackDamage: 10, BaseDefense: 5}
for i := 0; i < 100; i++ {
    cmds.Push(ecs.NewAddComponentCommand(zombieID, "BaseStats", zombieBaseStats))
    cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", CurrentStats{
        CurrentHealth: zombieBaseStats.MaxHealth,
        IsDead: false,
    }))
}

// Damage a zombie - only affects that zombie's CurrentStats
current := getCurrentStats(zombieID)
current.CurrentHealth -= 20
cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", current))
// BaseStats remain shared, other zombies unaffected
```

See `docs/examples/game/stats_pattern_example.go` for a complete implementation.

### Pattern 2: Pure Shared Components (Use with Caution)

This pattern uses SharedStorage for all stats. It works but has limitations when you need to modify values.

## Usage Patterns

### Basic Usage

```go
package main

import (
    "github.com/DangerosoDavo/ecs"
    ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

type GameStats struct {
    Health       int
    AttackDamage int
    Defense      int
}

func main() {
    world := ecs.NewWorld()

    // Register component with shared storage
    world.RegisterComponent("GameStats", ecsstorage.NewSharedStrategy())

    // Create entities with shared stats
    zombieStats := GameStats{Health: 50, AttackDamage: 10, Defense: 5}

    cmds := ecs.NewCommandBuffer()
    for i := 0; i < 100; i++ {
        var zombieID ecs.EntityID
        cmds.Push(ecs.NewCreateEntityCommand(&zombieID))
        cmds.Push(ecs.NewAddComponentCommand(zombieID, "GameStats", zombieStats))
    }
    world.ApplyCommands(cmds.Drain())

    // All 100 zombies now share the same GameStats instance in memory
}
```

### Reading Shared Components

Systems access shared components exactly like regular components:

```go
type CombatSystem struct{}

func (CombatSystem) Descriptor() ecs.SystemDescriptor {
    return ecs.SystemDescriptor{
        Name:   "combat",
        Reads:  []ecs.ComponentType{"GameStats"},
        Writes: []ecs.ComponentType{},
    }
}

func (CombatSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
    statsView, err := exec.World().ViewComponent("GameStats")
    if err != nil {
        return ecs.SystemResult{Err: err}
    }

    statsView.Iterate(func(id ecs.EntityID, component any) bool {
        stats := component.(GameStats)
        // Use stats normally - sharing is transparent
        exec.Logger().Info("combat", "entity", id, "health", stats.Health)
        return true
    })

    return ecs.SystemResult{}
}
```

### Modifying Shared Components

Shared components are **immutable per entity**. To modify a shared component:

1. Remove the old component
2. Add a new component with updated values

**Important:** This is expensive for frequently-changing values like health. Use the BaseStats + CurrentStats pattern instead.

```go
// Entity needs upgraded stats (ONLY for infrequent changes like archetype upgrades)
upgradedStats := GameStats{
    Health:       100,  // upgraded from 50
    AttackDamage: 20,   // upgraded from 10
    Defense:      10,   // upgraded from 5
}

cmds := ecs.NewCommandBuffer()
cmds.Push(ecs.NewRemoveComponentCommand(entityID, "GameStats"))
cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", upgradedStats))
exec.Defer(cmds.Drain()...)

// The entity now has different stats, and the old stats instance
// is automatically garbage collected if no other entities reference it
```

**For frequently-changing values (health, status):** Use the BaseStats + CurrentStats pattern where:
- BaseStats (shared) stores immutable max values
- CurrentStats (dense) stores mutable current values
- Systems read both components on the same entity

### Mixing Shared and Dense Components

Most games will use both storage strategies:

```go
world := ecs.NewWorld()

// Shared: base stats that many entities share
world.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())
world.RegisterComponent("WeaponStats", ecsstorage.NewSharedStrategy())

// Dense: entity-specific data
world.RegisterComponent("CurrentHealth", ecsstorage.NewDenseStrategy())
world.RegisterComponent("Position", ecsstorage.NewDenseStrategy())
world.RegisterComponent("Velocity", ecsstorage.NewDenseStrategy())
world.RegisterComponent("Inventory", ecsstorage.NewDenseStrategy())

// Example entity
var playerID ecs.EntityID
cmds := ecs.NewCommandBuffer()
cmds.Push(ecs.NewCreateEntityCommand(&playerID))

// Shared components
cmds.Push(ecs.NewAddComponentCommand(playerID, "BaseStats", warriorStats))
cmds.Push(ecs.NewAddComponentCommand(playerID, "WeaponStats", swordStats))

// Unique components
cmds.Push(ecs.NewAddComponentCommand(playerID, "CurrentHealth", 100))
cmds.Push(ecs.NewAddComponentCommand(playerID, "Position", Position{X: 10, Y: 20}))
cmds.Push(ecs.NewAddComponentCommand(playerID, "Velocity", Velocity{VX: 0, VY: 0}))

world.ApplyCommands(cmds.Drain())
```

## Archetype Pattern

The archetype pattern uses shared components to define entity templates:

```go
// Define archetypes
var (
    ZombieArchetype = GameStats{
        Health:       50,
        AttackDamage: 10,
        Defense:      5,
        MoveSpeed:    2.0,
    }

    SkeletonArchetype = GameStats{
        Health:       40,
        AttackDamage: 15,
        Defense:      3,
        MoveSpeed:    3.0,
    }

    BossArchetype = GameStats{
        Health:       500,
        AttackDamage: 50,
        Defense:      30,
        MoveSpeed:    1.5,
    }
)

// Spawn entities from archetypes
func SpawnZombie(world *ecs.World, pos Position) ecs.EntityID {
    var id ecs.EntityID
    cmds := ecs.NewCommandBuffer()

    cmds.Push(ecs.NewCreateEntityCommand(&id))
    cmds.Push(ecs.NewAddComponentCommand(id, "GameStats", ZombieArchetype))
    cmds.Push(ecs.NewAddComponentCommand(id, "Position", pos))

    world.ApplyCommands(cmds.Drain())
    return id
}

// Spawn 100 zombies - they all share the same GameStats instance
for i := 0; i < 100; i++ {
    SpawnZombie(world, Position{X: float64(i * 10), Y: 0})
}
```

## Performance Characteristics

### Time Complexity

| Operation | Dense Strategy | Shared Strategy |
|-----------|----------------|-----------------|
| Set       | O(1)           | O(n)* for deduplication |
| Get       | O(1)           | O(1) |
| Remove    | O(1)           | O(1) |
| Iterate   | O(n)           | O(n) |

\* Shared storage uses `reflect.DeepEqual` to find existing values, which is O(n) where n is the number of unique values. This is typically very small (e.g., 10-100 unique archetypes).

### Memory Characteristics

- **Dense**: O(entities) memory
- **Shared**: O(unique values) memory

For scenarios with high component reuse (many entities sharing few unique values), shared storage provides significant memory savings.

### Best Performance Scenario

Shared components perform best when:
- High entity count (1000+)
- Low unique value count (10-100 archetypes)
- Infrequent modifications
- Large component structs (100+ bytes)

Example: 10,000 zombies with 5 unique upgrade levels = 10,000 entities, 5 unique values.

## Debugging and Monitoring

Use the `Stats()` method to monitor shared storage efficiency:

```go
store := world.Storage().View("GameStats").(*storage.SharedStore)
stats := store.Stats()

fmt.Printf("Entities: %d\n", stats.EntityCount)
fmt.Printf("Unique values: %d\n", stats.UniqueValueCount)
fmt.Printf("Sharing ratio: %.2f\n", stats.SharingRatio)

// High sharing ratio (>10) indicates effective use of shared storage
// Low sharing ratio (<2) suggests dense storage might be better
```

## Common Pitfalls

### 1. Modifying Shared Data Directly

**Don't do this:**
```go
// BAD: This modifies the shared instance, affecting ALL entities!
stats, _ := statsView.Get(entityID)
s := stats.(GameStats)
s.Health = 100  // WRONG: This doesn't actually do anything due to value copy
```

**Do this instead:**
```go
// GOOD: Remove and re-add with new value
cmds.Push(ecs.NewRemoveComponentCommand(entityID, "GameStats"))
cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", newStats))
```

### 2. Using Shared Storage for High-Churn Data

**Don't do this:**
```go
// BAD: CurrentHealth changes frequently per entity
world.RegisterComponent("CurrentHealth", ecsstorage.NewSharedStrategy())
```

**Do this instead:**
```go
// GOOD: Use dense storage for entity-specific, frequently changing data
world.RegisterComponent("CurrentHealth", ecsstorage.NewDenseStrategy())

// Use shared storage for base/max values
world.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())
```

### 3. Assuming Reference Semantics

Shared components use value semantics from the component's perspective. Each `Get()` returns a copy of the value:

```go
stats1, _ := statsView.Get(entity1)
stats2, _ := statsView.Get(entity2)

// stats1 and stats2 are separate copies, even if they're backed by the same shared instance
// Modifying stats1 does NOT affect stats2
stats1.Health = 999  // This only modifies the local copy
```

## Complete Example

See `docs/examples/game/shared_stats_example.go` for a complete, runnable example demonstrating:
- Setting up shared vs dense components
- Creating entities with shared stats
- Systems that read shared components
- Modifying shared components
- Performance comparisons

## Summary

| Aspect | Dense Strategy | Shared Strategy |
|--------|----------------|-----------------|
| **Memory** | O(entities) | O(unique values) |
| **Performance** | Fast set/get | Slower set, fast get |
| **Use case** | Entity-specific data | Archetype/template data |
| **Mutability** | Mutable | Immutable (remove + add) |
| **Best for** | Unique values per entity | Many entities, few unique values |

Choose the right storage strategy based on your data access patterns and memory constraints.
