package ecs

import "sync"

type storageProvider struct {
	mu     sync.RWMutex
	stores map[ComponentType]ComponentStore
}

func newStorageProvider() *storageProvider {
	return &storageProvider{stores: make(map[ComponentType]ComponentStore)}
}

func (p *storageProvider) RegisterComponent(t ComponentType, strategy StorageStrategy) error {
	if strategy == nil {
		return ErrNilStorageStrategy
	}

	store := strategy.NewStore(t)
	if store == nil {
		return ErrNilComponentStore
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.stores[t]; exists {
		return ErrComponentAlreadyRegistered
	}

	p.stores[t] = store
	return nil
}

func (p *storageProvider) View(t ComponentType) (ComponentView, error) {
	p.mu.RLock()
	store, ok := p.stores[t]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrComponentNotRegistered
	}

	return store, nil
}

func (p *storageProvider) Apply(world *World, commands []Command) error {
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		if err := cmd.Apply(world); err != nil {
			return err
		}
	}
	return nil
}

var _ StorageProvider = (*storageProvider)(nil)
