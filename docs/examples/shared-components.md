# Shared component definitions

Register immutable definitions with `storage.NewSharedStrategy()` and changing runtime values with `storage.NewDenseStrategy()`. See the [compiled game examples](game/) and [README](../../README.md).

```go
world.RegisterComponent("WeaponDefinition", storage.NewSharedStrategy())
world.RegisterComponent("WeaponState", storage.NewDenseStrategy())
world.ApplyCommands([]ecs.Command{
    ecs.NewSpawnEntityCommand(nil, map[ecs.ComponentType]any{
        "WeaponDefinition": WeaponDefinition{Damage: 20},
        "WeaponState": WeaponState{Ammo: 10},
    }),
})
```

Check errors in application code. Define `WeaponDefinition` and `WeaponState` as your project's data structs.

Shared values are deduplicated using deep equality; costs scale with the number/size of unique values. Values containing pointers, slices, or maps remain mutable Go objects: the store does not enforce deep immutability. Replace a definition through a command to change one entity; never mutate the shared graph in place.

Get returns the stored value. Normal Iterate follows Go map order; use `ecs.IterateOrdered` or an ordered `ecs.Query` if outcomes depend on iteration order. Async snapshots detach declared reads, including nested plain data, from the live store.

Do not infer whole-world memory savings from unique payload counts. Entity-to-definition maps, IDs, locks, and allocator overhead still consume memory; measure representative definitions and populations with allocation profiles.
