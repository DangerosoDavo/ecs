package game

import (
	"time"
)

// BaseStats represents the immutable base statistics for an entity archetype.
// This is shared across all entities of the same type (e.g., all zombies share zombie base stats).
// Use SharedStorageStrategy for this component.
type BaseStats struct {
	MaxHealth        int
	BaseAttackDamage int
	BaseDefense      int
	BaseMoveSpeed    float64
	MiningEfficiency int
}

// CurrentStats represents the runtime mutable statistics for an individual entity.
// This is unique per entity and can be modified by systems.
// Use DenseStorageStrategy for this component.
type CurrentStats struct {
	CurrentHealth int
	IsDead        bool
}

// StatModifier represents a time-limited modification to stats (buff or debuff).
type StatModifier struct {
	Type       ModifierType
	Value      float64 // multiplier or flat value
	ExpiresAt  time.Time
	Source     string // what caused this modifier (e.g., "strength_potion", "poison")
}

type ModifierType int

const (
	ModifierTypeAttackMultiplier ModifierType = iota
	ModifierTypeDefenseMultiplier
	ModifierTypeSpeedMultiplier
	ModifierTypeFlatAttack
	ModifierTypeFlatDefense
	ModifierTypeHealthRegen
)

// StatModifiers holds all active modifiers for an entity.
// Multiple components/systems can add modifiers that affect the entity's effective stats.
// Use DenseStorageStrategy for this component.
type StatModifiers struct {
	Modifiers []StatModifier
}

// AddModifier adds a new stat modifier to the entity.
func (sm *StatModifiers) AddModifier(mod StatModifier) {
	sm.Modifiers = append(sm.Modifiers, mod)
}

// RemoveExpired removes all expired modifiers and returns true if any were removed.
func (sm *StatModifiers) RemoveExpired(now time.Time) bool {
	initialLen := len(sm.Modifiers)

	// Filter out expired modifiers
	active := sm.Modifiers[:0]
	for _, mod := range sm.Modifiers {
		if now.Before(mod.ExpiresAt) {
			active = append(active, mod)
		}
	}
	sm.Modifiers = active

	return len(sm.Modifiers) < initialLen
}

// GetEffectiveAttack calculates the current attack damage considering base stats and modifiers.
func GetEffectiveAttack(base BaseStats, mods *StatModifiers) int {
	if mods == nil {
		return base.BaseAttackDamage
	}

	attack := float64(base.BaseAttackDamage)

	// Apply flat modifiers first
	for _, mod := range mods.Modifiers {
		if mod.Type == ModifierTypeFlatAttack {
			attack += mod.Value
		}
	}

	// Apply multipliers
	for _, mod := range mods.Modifiers {
		if mod.Type == ModifierTypeAttackMultiplier {
			attack *= mod.Value
		}
	}

	return int(attack)
}

// GetEffectiveDefense calculates the current defense considering base stats and modifiers.
func GetEffectiveDefense(base BaseStats, mods *StatModifiers) int {
	if mods == nil {
		return base.BaseDefense
	}

	defense := float64(base.BaseDefense)

	// Apply flat modifiers first
	for _, mod := range mods.Modifiers {
		if mod.Type == ModifierTypeFlatDefense {
			defense += mod.Value
		}
	}

	// Apply multipliers
	for _, mod := range mods.Modifiers {
		if mod.Type == ModifierTypeDefenseMultiplier {
			defense *= mod.Value
		}
	}

	return int(defense)
}

// GetEffectiveSpeed calculates the current movement speed considering base stats and modifiers.
func GetEffectiveSpeed(base BaseStats, mods *StatModifiers) float64 {
	if mods == nil {
		return base.BaseMoveSpeed
	}

	speed := base.BaseMoveSpeed

	// Apply speed multipliers
	for _, mod := range mods.Modifiers {
		if mod.Type == ModifierTypeSpeedMultiplier {
			speed *= mod.Value
		}
	}

	return speed
}

// EntityArchetypes defines common base stats for different entity types.
var (
	ZombieBaseStats = BaseStats{
		MaxHealth:        50,
		BaseAttackDamage: 10,
		BaseDefense:      5,
		BaseMoveSpeed:    2.0,
		MiningEfficiency: 0,
	}

	SkeletonBaseStats = BaseStats{
		MaxHealth:        40,
		BaseAttackDamage: 15,
		BaseDefense:      3,
		BaseMoveSpeed:    3.0,
		MiningEfficiency: 0,
	}

	MinerBaseStats = BaseStats{
		MaxHealth:        75,
		BaseAttackDamage: 5,
		BaseDefense:      8,
		BaseMoveSpeed:    3.0,
		MiningEfficiency: 15,
	}

	BossBaseStats = BaseStats{
		MaxHealth:        500,
		BaseAttackDamage: 50,
		BaseDefense:      30,
		BaseMoveSpeed:    1.5,
		MiningEfficiency: 0,
	}

	PlayerBaseStats = BaseStats{
		MaxHealth:        100,
		BaseAttackDamage: 20,
		BaseDefense:      10,
		BaseMoveSpeed:    5.0,
		MiningEfficiency: 5,
	}
)
