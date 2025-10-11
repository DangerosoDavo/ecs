package game

import (
	"context"
	"fmt"
	"time"

	"github.com/DangerosoDavo/ecs"
	ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

// GameStats represents shared statistics that multiple entities of the same type use.
// IMPORTANT: When using SharedStorage, GameStats are IMMUTABLE from an entity's perspective.
// To "modify" an entity's stats, you must remove the old component and add a new one.
// This automatically "unshares" that entity from others.
//
// RECOMMENDED PATTERN: Use BaseStats (shared) + CurrentStats (dense) pattern instead.
// See stats_pattern_example.go for the recommended approach where:
// - BaseStats (shared): Immutable base values for entity archetypes
// - CurrentStats (dense): Mutable runtime values unique to each entity
// - StatModifiers (dense): Time-limited buffs/debuffs
//
// This example demonstrates the basic shared storage mechanism.
type GameStats struct {
	MaxHealth     int
	AttackDamage  int
	Defense       int
	MoveSpeed     float64
	MiningEfficiency int
}

// Position is a unique component - each entity has its own position
type Position struct {
	X, Y float64
}

// SimpleCombatSystem demonstrates how to use shared stats in a system
type SimpleCombatSystem struct{}

func (SimpleCombatSystem) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{
		Name:         "combat",
		Reads:        []ecs.ComponentType{"GameStats", "Position"},
		Writes:       []ecs.ComponentType{},
		RunEvery:     ecs.TickInterval{Every: 1},
		AsyncAllowed: true,
	}
}

func (SimpleCombatSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
	statsView, err := exec.World().ViewComponent("GameStats")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}

	posView, err := exec.World().ViewComponent("Position")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}

	// Iterate through all entities with GameStats
	statsView.Iterate(func(id ecs.EntityID, component any) bool {
		stats := component.(GameStats)

		// Get position if available
		if pos, ok := posView.Get(id); ok {
			position := pos.(Position)
			exec.Logger().Info("entity combat stats",
				"entity", id,
				"health", stats.MaxHealth,
				"attack", stats.AttackDamage,
				"defense", stats.Defense,
				"pos_x", position.X,
				"pos_y", position.Y,
			)
		}
		return true
	})

	return ecs.SystemResult{}
}

// MiningSystem demonstrates another system using the same shared stats
type MiningSystem struct{}

func (MiningSystem) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{
		Name:         "mining",
		Reads:        []ecs.ComponentType{"GameStats"},
		Writes:       []ecs.ComponentType{},
		RunEvery:     ecs.TickInterval{Every: 10}, // runs less frequently
		AsyncAllowed: true,
	}
}

func (MiningSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
	statsView, err := exec.World().ViewComponent("GameStats")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}

	statsView.Iterate(func(id ecs.EntityID, component any) bool {
		stats := component.(GameStats)
		if stats.MiningEfficiency > 0 {
			exec.Logger().Info("entity mining",
				"entity", id,
				"efficiency", stats.MiningEfficiency,
			)
		}
		return true
	})

	return ecs.SystemResult{}
}

// ExampleSharedStats demonstrates how to set up and use shared component storage
func ExampleSharedStats() {
	world := ecs.NewWorld()

	// Register GameStats with SHARED storage - multiple entities will reference the same data
	world.RegisterComponent("GameStats", ecsstorage.NewSharedStrategy())

	// Register Position with DENSE storage - each entity has unique position
	world.RegisterComponent("Position", ecsstorage.NewDenseStrategy())

	// Create the scheduler
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		panic(err)
	}

	scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
		ID:      "gameplay",
		Mode:    ecs.WorkGroupModeSynchronized,
		Systems: []ecs.System{SimpleCombatSystem{}, MiningSystem{}},
	})

	// Define entity archetypes (templates) with shared stats
	zombieStats := GameStats{
		MaxHealth:     50,
		AttackDamage:  10,
		Defense:       5,
		MoveSpeed:     2.0,
		MiningEfficiency: 0,
	}

	minerStats := GameStats{
		MaxHealth:     75,
		AttackDamage:  5,
		Defense:       8,
		MoveSpeed:     3.0,
		MiningEfficiency: 15,
	}

	bossStats := GameStats{
		MaxHealth:     500,
		AttackDamage:  50,
		Defense:       30,
		MoveSpeed:     1.5,
		MiningEfficiency: 0,
	}

	// Create entities - all zombies will share the same stats instance in memory
	cmds := ecs.NewCommandBuffer()

	// Spawn 100 zombies - they all share the SAME GameStats instance
	for i := 0; i < 100; i++ {
		var zombieID ecs.EntityID
		cmds.Push(ecs.NewCreateEntityCommand(&zombieID))
		cmds.Push(ecs.NewAddComponentCommand(zombieID, "GameStats", zombieStats))
		cmds.Push(ecs.NewAddComponentCommand(zombieID, "Position", Position{
			X: float64(i * 10),
			Y: float64(i % 10),
		}))
	}

	// Spawn 50 miners - they all share the SAME GameStats instance
	for i := 0; i < 50; i++ {
		var minerID ecs.EntityID
		cmds.Push(ecs.NewCreateEntityCommand(&minerID))
		cmds.Push(ecs.NewAddComponentCommand(minerID, "GameStats", minerStats))
		cmds.Push(ecs.NewAddComponentCommand(minerID, "Position", Position{
			X: float64(i * 15),
			Y: 100.0,
		}))
	}

	// Spawn 1 boss with unique stats
	var bossID ecs.EntityID
	cmds.Push(ecs.NewCreateEntityCommand(&bossID))
	cmds.Push(ecs.NewAddComponentCommand(bossID, "GameStats", bossStats))
	cmds.Push(ecs.NewAddComponentCommand(bossID, "Position", Position{X: 500, Y: 500}))

	// Apply all entity creation commands
	world.ApplyCommands(cmds.Drain())

	fmt.Println("Created 151 entities:")
	fmt.Println("- 100 zombies (sharing 1 GameStats instance)")
	fmt.Println("- 50 miners (sharing 1 GameStats instance)")
	fmt.Println("- 1 boss (unique GameStats instance)")
	fmt.Println("Total unique GameStats instances in memory: 3")

	// Run a few simulation ticks
	for i := 0; i < 3; i++ {
		if err := scheduler.Tick(context.Background(), 16*time.Millisecond); err != nil {
			panic(err)
		}
	}
}

// ExampleModifyingSharedStats demonstrates how to "modify" shared stats
// Since shared components are immutable, you need to remove and re-add with new values
func ExampleModifyingSharedStats() {
	world := ecs.NewWorld()
	world.RegisterComponent("GameStats", ecsstorage.NewSharedStrategy())

	// Create entity with stats
	entityID := world.Registry().Create()
	cmds := ecs.NewCommandBuffer()

	originalStats := GameStats{
		MaxHealth:    50,
		AttackDamage: 10,
		Defense:      5,
		MoveSpeed:    2.0,
	}

	cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", originalStats))
	world.ApplyCommands(cmds.Drain())

	// To "modify" shared stats, remove the old value and add a new one
	// This automatically unshares the entity if others were using the same stats
	upgradedStats := GameStats{
		MaxHealth:    75,    // upgraded
		AttackDamage: 15,    // upgraded
		Defense:      8,     // upgraded
		MoveSpeed:    2.0,   // same
	}

	cmds = ecs.NewCommandBuffer()
	cmds.Push(ecs.NewRemoveComponentCommand(entityID, "GameStats"))
	cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", upgradedStats))
	world.ApplyCommands(cmds.Drain())

	fmt.Println("Entity stats upgraded successfully")
}

// ComparisonDenseVsShared demonstrates the memory efficiency of shared storage
func ComparisonDenseVsShared() {
	// Scenario: 1000 entities, all with identical stats

	// With Dense Storage (traditional approach)
	worldDense := ecs.NewWorld()
	worldDense.RegisterComponent("GameStats", ecsstorage.NewDenseStrategy())

	stats := GameStats{MaxHealth: 100, AttackDamage: 25, Defense: 10, MoveSpeed: 3.0}
	cmds := ecs.NewCommandBuffer()

	for i := 0; i < 1000; i++ {
		entityID := worldDense.Registry().Create()
		cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", stats))
	}
	worldDense.ApplyCommands(cmds.Drain())

	fmt.Println("Dense Storage: 1000 separate GameStats instances in memory")

	// With Shared Storage (new approach)
	worldShared := ecs.NewWorld()
	worldShared.RegisterComponent("GameStats", ecsstorage.NewSharedStrategy())

	cmds = ecs.NewCommandBuffer()
	for i := 0; i < 1000; i++ {
		entityID := worldShared.Registry().Create()
		cmds.Push(ecs.NewAddComponentCommand(entityID, "GameStats", stats))
	}
	worldShared.ApplyCommands(cmds.Drain())

	fmt.Println("Shared Storage: 1 shared GameStats instance in memory (referenced 1000 times)")
	fmt.Println("Memory savings: ~99.9%")
}

// IMPORTANT NOTES:
//
// 1. SharedStorage vs Dense+Shared Pattern
//
// The examples above use SharedStorage for GameStats where ALL stats (including health)
// are shared. This means if you want to damage one zombie's health, you must:
//   - Remove the GameStats component
//   - Add a new GameStats with updated health
//   - This "unshares" the zombie from others
//
// This works, but it's NOT the recommended pattern for game stats because:
//   - Health changes frequently (on every hit)
//   - Each health change creates a new unique GameStats instance
//   - You lose the memory benefits of sharing
//
// 2. RECOMMENDED PATTERN: BaseStats (shared) + CurrentStats (dense)
//
// A better approach is to split stats into:
//   - BaseStats (shared): Immutable base values for entity archetypes (max health, base attack, etc.)
//   - CurrentStats (dense): Mutable runtime values unique to each entity (current health, is dead, etc.)
//   - StatModifiers (dense): Time-limited buffs/debuffs (strength potion, poison, etc.)
//
// This gives you:
//   ✓ Memory efficiency: All zombies share BaseStats
//   ✓ Mutable per-entity state: Each zombie has unique CurrentStats
//   ✓ Flexible modifiers: Temporary buffs/debuffs per entity
//   ✓ Multiple systems can access the same entity's stats components
//
// See stats_pattern_example.go for a complete implementation of this pattern.
//
// 3. When to Use Shared Storage
//
// Use SharedStorage when:
//   - Component values are truly immutable (configuration, templates, archetypes)
//   - Many entities share identical values (1000+ zombies, all with same base stats)
//   - Changes are rare (entity type upgrades, archetype switches)
//
// DON'T use SharedStorage when:
//   - Values change frequently (health, position, velocity)
//   - Most entities have unique values
//   - You need mutable per-entity state
//
// 4. Example: Damaging an Entity
//
// With SharedStorage only (NOT RECOMMENDED for frequently changing values):
//
//   // Get entity's GameStats
//   stats, _ := statsView.Get(zombieID)
//   gameStats := stats.(GameStats)
//
//   // "Modify" health by removing and re-adding (expensive!)
//   gameStats.MaxHealth -= 20
//   cmds.Push(ecs.NewRemoveComponentCommand(zombieID, "GameStats"))
//   cmds.Push(ecs.NewAddComponentCommand(zombieID, "GameStats", gameStats))
//   // This zombie now has unique GameStats, loses sharing benefit
//
// With BaseStats + CurrentStats pattern (RECOMMENDED):
//
//   // Get entity's CurrentStats (mutable, unique per entity)
//   current, _ := currentView.Get(zombieID)
//   currentStats := current.(CurrentStats)
//
//   // Modify current health directly (cheap!)
//   currentStats.CurrentHealth -= 20
//   cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", currentStats))
//   // BaseStats remain shared, memory efficiency preserved
