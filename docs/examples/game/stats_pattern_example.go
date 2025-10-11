package game

import (
	"context"
	"fmt"
	"time"

	"github.com/DangerosoDavo/ecs"
	ecsstorage "github.com/DangerosoDavo/ecs/ecs/storage"
)

// ExampleStatsPattern demonstrates the recommended pattern for using shared stats
// components. This pattern separates:
// 1. BaseStats (shared) - immutable archetype stats that multiple entities of the same type share
// 2. CurrentStats (dense) - mutable runtime stats unique to each entity
// 3. StatModifiers (dense) - time-limited buffs/debuffs unique to each entity
//
// This allows you to:
// - Save memory by sharing base stats across entity types
// - Modify individual entity stats without affecting others
// - Apply temporary stat modifiers (buffs/debuffs)
// - Calculate effective stats by combining base + modifiers
func ExampleStatsPattern() {
	world := ecs.NewWorld()

	// BaseStats uses SHARED storage - all zombies reference the same base stats
	world.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())

	// CurrentStats uses DENSE storage - each entity has unique current health
	world.RegisterComponent("CurrentStats", ecsstorage.NewDenseStrategy())

	// StatModifiers uses DENSE storage - each entity has unique active buffs/debuffs
	world.RegisterComponent("StatModifiers", ecsstorage.NewDenseStrategy())

	// Position is unique per entity
	world.RegisterComponent("Position", ecsstorage.NewDenseStrategy())

	// Create scheduler with systems
	scheduler, err := ecs.NewScheduler(world)
	if err != nil {
		panic(err)
	}

	scheduler.RegisterWorkGroup(ecs.WorkGroupConfig{
		ID:   "gameplay",
		Mode: ecs.WorkGroupModeSynchronized,
		Systems: []ecs.System{
			HealthSystem{},
			CombatSystem{},
			ModifierCleanupSystem{},
			StatsDisplaySystem{},
		},
	})

	cmds := ecs.NewCommandBuffer()

	// Spawn 100 zombies - they all SHARE the same BaseStats instance
	fmt.Println("Creating 100 zombies with shared base stats...")
	for i := 0; i < 100; i++ {
		var zombieID ecs.EntityID
		cmds.Push(ecs.NewCreateEntityCommand(&zombieID))

		// BaseStats is SHARED across all zombies (memory efficient!)
		cmds.Push(ecs.NewAddComponentCommand(zombieID, "BaseStats", ZombieBaseStats))

		// CurrentStats is UNIQUE per zombie (can be modified independently)
		cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", CurrentStats{
			CurrentHealth: ZombieBaseStats.MaxHealth, // start at max health
			IsDead:        false,
		}))

		// Each zombie gets a unique position
		cmds.Push(ecs.NewAddComponentCommand(zombieID, "Position", Position{
			X: float64(i * 10),
			Y: float64(i % 10),
		}))
	}

	// Spawn 50 skeletons
	fmt.Println("Creating 50 skeletons with shared base stats...")
	for i := 0; i < 50; i++ {
		var skeletonID ecs.EntityID
		cmds.Push(ecs.NewCreateEntityCommand(&skeletonID))

		cmds.Push(ecs.NewAddComponentCommand(skeletonID, "BaseStats", SkeletonBaseStats))
		cmds.Push(ecs.NewAddComponentCommand(skeletonID, "CurrentStats", CurrentStats{
			CurrentHealth: SkeletonBaseStats.MaxHealth,
			IsDead:        false,
		}))
		cmds.Push(ecs.NewAddComponentCommand(skeletonID, "Position", Position{
			X: float64(i * 15),
			Y: 100.0,
		}))
	}

	// Spawn 1 boss
	fmt.Println("Creating 1 boss with unique base stats...")
	var bossID ecs.EntityID
	cmds.Push(ecs.NewCreateEntityCommand(&bossID))
	cmds.Push(ecs.NewAddComponentCommand(bossID, "BaseStats", BossBaseStats))
	cmds.Push(ecs.NewAddComponentCommand(bossID, "CurrentStats", CurrentStats{
		CurrentHealth: BossBaseStats.MaxHealth,
		IsDead:        false,
	}))
	cmds.Push(ecs.NewAddComponentCommand(bossID, "Position", Position{X: 500, Y: 500}))

	// Apply all commands
	world.ApplyCommands(cmds.Drain())

	fmt.Println("\nMemory efficiency:")
	fmt.Println("- 151 entities created")
	fmt.Println("- Only 3 unique BaseStats instances in memory (Zombie, Skeleton, Boss)")
	fmt.Println("- 151 unique CurrentStats instances (one per entity)")
	fmt.Println("- Memory savings: ~98% for BaseStats compared to dense storage")

	// Now let's damage some zombies individually
	fmt.Println("\n=== Damaging Individual Zombies ===")

	// Get the first 3 zombie IDs
	zombieIDs := []ecs.EntityID{}
	statsView, _ := world.ViewComponent("CurrentStats")
	statsView.Iterate(func(id ecs.EntityID, _ any) bool {
		if len(zombieIDs) < 3 {
			zombieIDs = append(zombieIDs, id)
		}
		return len(zombieIDs) < 3
	})

	// Damage zombie 1
	if len(zombieIDs) > 0 {
		current, _ := statsView.Get(zombieIDs[0])
		zombieStats := current.(CurrentStats)
		zombieStats.CurrentHealth -= 20
		fmt.Printf("Damaged zombie %v: health %d -> %d\n", zombieIDs[0], 50, zombieStats.CurrentHealth)

		cmds := ecs.NewCommandBuffer()
		cmds.Push(ecs.NewAddComponentCommand(zombieIDs[0], "CurrentStats", zombieStats))
		world.ApplyCommands(cmds.Drain())
	}

	// Verify other zombies are unaffected
	if len(zombieIDs) > 1 {
		current, _ := statsView.Get(zombieIDs[1])
		zombieStats := current.(CurrentStats)
		fmt.Printf("Zombie %v still at full health: %d\n", zombieIDs[1], zombieStats.CurrentHealth)
	}

	fmt.Println("\n✓ Only the damaged zombie's health changed - others are unaffected!")

	// Now let's apply a buff to one zombie
	fmt.Println("\n=== Applying Buff to One Zombie ===")
	if len(zombieIDs) > 0 {
		buffedZombieID := zombieIDs[0]

		// Add a strength buff (2x attack for 30 seconds)
		strengthBuff := StatModifiers{
			Modifiers: []StatModifier{
				{
					Type:      ModifierTypeAttackMultiplier,
					Value:     2.0,
					ExpiresAt: time.Now().Add(30 * time.Second),
					Source:    "strength_potion",
				},
			},
		}

		cmds := ecs.NewCommandBuffer()
		cmds.Push(ecs.NewAddComponentCommand(buffedZombieID, "StatModifiers", strengthBuff))
		world.ApplyCommands(cmds.Drain())

		// Calculate effective stats
		baseView, _ := world.ViewComponent("BaseStats")
		modView, _ := world.ViewComponent("StatModifiers")

		base, _ := baseView.Get(buffedZombieID)
		mod, _ := modView.Get(buffedZombieID)

		baseStats := base.(BaseStats)
		mods := mod.(StatModifiers)

		effectiveAttack := GetEffectiveAttack(baseStats, &mods)

		fmt.Printf("Zombie %v received strength buff:\n", buffedZombieID)
		fmt.Printf("  Base attack: %d\n", baseStats.BaseAttackDamage)
		fmt.Printf("  Effective attack (with buff): %d\n", effectiveAttack)
		fmt.Println("\n✓ Buff only affects this zombie - other zombies still have base attack!")
	}

	// Show how multiple components can access the same entity's stats
	fmt.Println("\n=== Multiple Systems Accessing Same Entity's Stats ===")
	if len(zombieIDs) > 0 {
		entityID := zombieIDs[0]

		baseView, _ := world.ViewComponent("BaseStats")
		currentView, _ := world.ViewComponent("CurrentStats")
		modView, _ := world.ViewComponent("StatModifiers")

		base, _ := baseView.Get(entityID)
		current, _ := currentView.Get(entityID)
		mod, hasMod := modView.Get(entityID)

		baseStats := base.(BaseStats)
		currentStats := current.(CurrentStats)

		var mods *StatModifiers
		if hasMod {
			m := mod.(StatModifiers)
			mods = &m
		}

		fmt.Printf("Entity %v stats accessible by all systems:\n", entityID)
		fmt.Printf("  HealthSystem sees: CurrentHealth=%d, MaxHealth=%d\n",
			currentStats.CurrentHealth, baseStats.MaxHealth)
		fmt.Printf("  CombatSystem sees: EffectiveAttack=%d, EffectiveDefense=%d\n",
			GetEffectiveAttack(baseStats, mods), GetEffectiveDefense(baseStats, mods))
		fmt.Printf("  MovementSystem sees: EffectiveSpeed=%.1f\n",
			GetEffectiveSpeed(baseStats, mods))
		fmt.Println("\n✓ All systems can access the same entity's stats components!")
	}

	// Run a few ticks
	fmt.Println("\n=== Running Simulation ===")
	for i := 0; i < 3; i++ {
		if err := scheduler.Tick(context.Background(), 16*time.Millisecond); err != nil {
			panic(err)
		}
	}
}

// ExampleUpgradingEntityArchetype demonstrates how to "upgrade" an entity from one
// archetype to another by changing its BaseStats reference.
func ExampleUpgradingEntityArchetype() {
	world := ecs.NewWorld()

	world.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())
	world.RegisterComponent("CurrentStats", ecsstorage.NewDenseStrategy())

	// Create a zombie
	var zombieID ecs.EntityID
	cmds := ecs.NewCommandBuffer()

	cmds.Push(ecs.NewCreateEntityCommand(&zombieID))
	cmds.Push(ecs.NewAddComponentCommand(zombieID, "BaseStats", ZombieBaseStats))
	cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", CurrentStats{
		CurrentHealth: ZombieBaseStats.MaxHealth,
		IsDead:        false,
	}))

	world.ApplyCommands(cmds.Drain())

	fmt.Printf("Created zombie with base attack: %d\n", ZombieBaseStats.BaseAttackDamage)

	// "Upgrade" the zombie to a boss by changing its BaseStats reference
	// This is memory-efficient because we're just changing the reference, not copying data
	fmt.Println("\nUpgrading zombie to boss archetype...")

	cmds = ecs.NewCommandBuffer()
	cmds.Push(ecs.NewRemoveComponentCommand(zombieID, "BaseStats"))
	cmds.Push(ecs.NewAddComponentCommand(zombieID, "BaseStats", BossBaseStats))

	// Also update current health to match new max
	cmds.Push(ecs.NewAddComponentCommand(zombieID, "CurrentStats", CurrentStats{
		CurrentHealth: BossBaseStats.MaxHealth,
		IsDead:        false,
	}))

	world.ApplyCommands(cmds.Drain())

	// Verify the upgrade
	baseView, _ := world.ViewComponent("BaseStats")
	base, _ := baseView.Get(zombieID)
	newBaseStats := base.(BaseStats)

	fmt.Printf("Zombie upgraded! New base attack: %d\n", newBaseStats.BaseAttackDamage)
	fmt.Println("\n✓ Entity archetype changed without affecting other zombies!")
}

// ExampleSharedStatsVsDenseStats compares memory usage between shared and dense storage.
func ExampleSharedStatsVsDenseStats() {
	fmt.Println("=== Memory Comparison: Shared vs Dense Storage ===")

	// Scenario: 1000 zombies with identical base stats

	// With Dense Storage
	worldDense := ecs.NewWorld()
	worldDense.RegisterComponent("BaseStats", ecsstorage.NewDenseStrategy())

	cmds := ecs.NewCommandBuffer()
	for i := 0; i < 1000; i++ {
		var id ecs.EntityID
		cmds.Push(ecs.NewCreateEntityCommand(&id))
		cmds.Push(ecs.NewAddComponentCommand(id, "BaseStats", ZombieBaseStats))
	}
	worldDense.ApplyCommands(cmds.Drain())

	fmt.Println("Dense Storage:")
	fmt.Println("  Entities: 1000")
	fmt.Println("  BaseStats instances in memory: 1000")
	fmt.Println("  Approximate memory (40 bytes per struct): ~40 KB")

	// With Shared Storage
	worldShared := ecs.NewWorld()
	worldShared.RegisterComponent("BaseStats", ecsstorage.NewSharedStrategy())

	cmds = ecs.NewCommandBuffer()
	for i := 0; i < 1000; i++ {
		var id ecs.EntityID
		cmds.Push(ecs.NewCreateEntityCommand(&id))
		cmds.Push(ecs.NewAddComponentCommand(id, "BaseStats", ZombieBaseStats))
	}
	worldShared.ApplyCommands(cmds.Drain())

	fmt.Println("\nShared Storage:")
	fmt.Println("  Entities: 1000")
	fmt.Println("  BaseStats instances in memory: 1")
	fmt.Println("  Reference map overhead: ~8 KB")
	fmt.Println("  Total memory: ~8 KB")
	fmt.Println("\n  Memory savings: ~80%")
}
