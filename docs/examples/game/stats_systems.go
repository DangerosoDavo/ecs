package game

import (
	"context"
	"time"

	"github.com/DangerosoDavo/ecs"
)

// HealthSystem manages entity health, death, and regeneration.
// It reads BaseStats (shared) and modifies CurrentStats (unique per entity).
type HealthSystem struct{}

func (HealthSystem) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{
		Name:         "health",
		Reads:        []ecs.ComponentType{"BaseStats", "StatModifiers"},
		Writes:       []ecs.ComponentType{"CurrentStats"},
		RunEvery:     ecs.TickInterval{Every: 1},
		AsyncAllowed: false,
	}
}

func (HealthSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
	baseStatsView, err := exec.World().ViewComponent("BaseStats")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}

	currentStatsView, err := exec.World().ViewComponent("CurrentStats")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}

	modifiersView, _ := exec.World().ViewComponent("StatModifiers")

	// Iterate through entities with current stats
	currentStatsView.Iterate(func(id ecs.EntityID, component any) bool {
		current := component.(CurrentStats)

		if current.IsDead {
			return true // skip dead entities
		}

		// Get base stats
		baseComponent, hasBase := baseStatsView.Get(id)
		if !hasBase {
			return true
		}
		base := baseComponent.(BaseStats)

		// Get modifiers if available
		var mods *StatModifiers
		if modComponent, hasMods := modifiersView.Get(id); hasMods {
			m := modComponent.(StatModifiers)
			mods = &m
		}

		// Apply health regeneration from modifiers
		if mods != nil {
			for _, mod := range mods.Modifiers {
				if mod.Type == ModifierTypeHealthRegen {
					current.CurrentHealth += int(mod.Value)
					if current.CurrentHealth > base.MaxHealth {
						current.CurrentHealth = base.MaxHealth
					}
				}
			}
		}

		// Check for death
		if current.CurrentHealth <= 0 {
			current.IsDead = true
			current.CurrentHealth = 0
			exec.Logger().Info("entity died", "entity", id)
		}

		// Update current stats
		exec.Defer(ecs.NewAddComponentCommand(id, "CurrentStats", current))
		return true
	})

	return ecs.SystemResult{}
}

// CombatSystem handles damage calculation using base stats and modifiers.
type CombatSystem struct{}

func (CombatSystem) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{
		Name:         "combat",
		Reads:        []ecs.ComponentType{"BaseStats", "StatModifiers", "CurrentStats", "Position"},
		Writes:       []ecs.ComponentType{"CurrentStats"},
		RunEvery:     ecs.TickInterval{Every: 60}, // combat happens every 60 ticks
		AsyncAllowed: false,
	}
}

func (CombatSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
	baseStatsView, _ := exec.World().ViewComponent("BaseStats")
	modifiersView, _ := exec.World().ViewComponent("StatModifiers")
	currentStatsView, _ := exec.World().ViewComponent("CurrentStats")
	positionView, _ := exec.World().ViewComponent("Position")

	// Find entities in combat range and apply damage
	var entities []ecs.EntityID
	currentStatsView.Iterate(func(id ecs.EntityID, _ any) bool {
		entities = append(entities, id)
		return true
	})

	for i := 0; i < len(entities); i++ {
		attackerID := entities[i]

		// Get attacker stats
		attackerCurrent, ok := currentStatsView.Get(attackerID)
		if !ok {
			continue
		}
		attackerCurrentStats := attackerCurrent.(CurrentStats)
		if attackerCurrentStats.IsDead {
			continue
		}

		attackerBase, ok := baseStatsView.Get(attackerID)
		if !ok {
			continue
		}
		attackerBaseStats := attackerBase.(BaseStats)

		// Get attacker modifiers
		var attackerMods *StatModifiers
		if modComponent, hasMods := modifiersView.Get(attackerID); hasMods {
			m := modComponent.(StatModifiers)
			attackerMods = &m
		}

		// Get attacker position
		attackerPos, hasPos := positionView.Get(attackerID)
		if !hasPos {
			continue
		}
		attackerPosition := attackerPos.(Position)

		// Find nearby targets
		for j := 0; j < len(entities); j++ {
			if i == j {
				continue
			}

			targetID := entities[j]

			// Get target position
			targetPos, hasTargetPos := positionView.Get(targetID)
			if !hasTargetPos {
				continue
			}
			targetPosition := targetPos.(Position)

			// Check if in range (simple distance check)
			dx := attackerPosition.X - targetPosition.X
			dy := attackerPosition.Y - targetPosition.Y
			distSq := dx*dx + dy*dy
			if distSq > 100.0 { // attack range = 10 units
				continue
			}

			// Get target stats
			targetCurrent, ok := currentStatsView.Get(targetID)
			if !ok {
				continue
			}
			targetCurrentStats := targetCurrent.(CurrentStats)
			if targetCurrentStats.IsDead {
				continue
			}

			targetBase, ok := baseStatsView.Get(targetID)
			if !ok {
				continue
			}
			targetBaseStats := targetBase.(BaseStats)

			// Get target modifiers
			var targetMods *StatModifiers
			if modComponent, hasMods := modifiersView.Get(targetID); hasMods {
				m := modComponent.(StatModifiers)
				targetMods = &m
			}

			// Calculate effective attack and defense
			effectiveAttack := GetEffectiveAttack(attackerBaseStats, attackerMods)
			effectiveDefense := GetEffectiveDefense(targetBaseStats, targetMods)

			// Apply damage
			damage := effectiveAttack - effectiveDefense
			if damage < 1 {
				damage = 1 // minimum damage
			}

			targetCurrentStats.CurrentHealth -= damage
			exec.Logger().Info("combat",
				"attacker", attackerID,
				"target", targetID,
				"damage", damage,
				"remaining_health", targetCurrentStats.CurrentHealth,
			)

			// Update target health
			exec.Defer(ecs.NewAddComponentCommand(targetID, "CurrentStats", targetCurrentStats))
			break // only attack one target per tick
		}
	}

	return ecs.SystemResult{}
}

// ModifierCleanupSystem removes expired stat modifiers.
type ModifierCleanupSystem struct{}

func (ModifierCleanupSystem) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{
		Name:         "modifier_cleanup",
		Reads:        []ecs.ComponentType{},
		Writes:       []ecs.ComponentType{"StatModifiers"},
		RunEvery:     ecs.TickInterval{Every: 10}, // check every 10 ticks
		AsyncAllowed: true,
	}
}

func (ModifierCleanupSystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
	modifiersView, err := exec.World().ViewComponent("StatModifiers")
	if err != nil {
		return ecs.SystemResult{Err: err}
	}

	now := time.Now()

	modifiersView.Iterate(func(id ecs.EntityID, component any) bool {
		mods := component.(StatModifiers)

		if mods.RemoveExpired(now) {
			exec.Logger().Info("expired modifiers removed", "entity", id)
			exec.Defer(ecs.NewAddComponentCommand(id, "StatModifiers", mods))
		}

		return true
	})

	return ecs.SystemResult{}
}

// StatsDisplaySystem logs entity stats for debugging.
type StatsDisplaySystem struct{}

func (StatsDisplaySystem) Descriptor() ecs.SystemDescriptor {
	return ecs.SystemDescriptor{
		Name:         "stats_display",
		Reads:        []ecs.ComponentType{"BaseStats", "CurrentStats", "StatModifiers"},
		Writes:       []ecs.ComponentType{},
		RunEvery:     ecs.TickInterval{Every: 100}, // display every 100 ticks
		AsyncAllowed: true,
	}
}

func (StatsDisplaySystem) Run(ctx context.Context, exec ecs.ExecutionContext) ecs.SystemResult {
	baseStatsView, _ := exec.World().ViewComponent("BaseStats")
	currentStatsView, _ := exec.World().ViewComponent("CurrentStats")
	modifiersView, _ := exec.World().ViewComponent("StatModifiers")

	currentStatsView.Iterate(func(id ecs.EntityID, component any) bool {
		current := component.(CurrentStats)

		baseComponent, hasBase := baseStatsView.Get(id)
		if !hasBase {
			return true
		}
		base := baseComponent.(BaseStats)

		var mods *StatModifiers
		if modComponent, hasMods := modifiersView.Get(id); hasMods {
			m := modComponent.(StatModifiers)
			mods = &m
		}

		// Calculate effective stats
		effectiveAttack := GetEffectiveAttack(base, mods)
		effectiveDefense := GetEffectiveDefense(base, mods)
		effectiveSpeed := GetEffectiveSpeed(base, mods)

		modCount := 0
		if mods != nil {
			modCount = len(mods.Modifiers)
		}

		exec.Logger().Info("entity stats",
			"entity", id,
			"health", current.CurrentHealth,
			"max_health", base.MaxHealth,
			"attack", effectiveAttack,
			"base_attack", base.BaseAttackDamage,
			"defense", effectiveDefense,
			"base_defense", base.BaseDefense,
			"speed", effectiveSpeed,
			"base_speed", base.BaseMoveSpeed,
			"active_modifiers", modCount,
			"is_dead", current.IsDead,
		)

		return true
	})

	return ecs.SystemResult{}
}
