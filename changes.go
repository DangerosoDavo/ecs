package ecs

// ChangeKind distinguishes an update from a durable deletion tombstone.
type ChangeKind uint8

const (
	ChangeUpdated ChangeKind = iota
	ChangeDestroyed
)

// Change is a notification, not a snapshot of component values. Consume changes
// and copy required state at a simulation barrier, before handing it to I/O.
type Change struct {
	Sequence uint64
	Entity   EntityID
	Kind     ChangeKind
}

type ChangeBatch struct {
	Cursor  uint64
	Changes []Change
}

// WithChangeHistoryCapacity bounds memory regardless of consumer progress.
// A slow consumer receives ErrChangeHistoryExpired and must perform a full
// resync. Zero disables history; legacy Changed/DrainChanged still work.
func WithChangeHistoryCapacity(capacity int) WorldOption {
	return func(w *World) {
		if capacity < 0 {
			capacity = 0
		}
		w.changeCapacity = capacity
	}
}

func (w *World) recordChangeLocked(id EntityID, kind ChangeKind) {
	w.changeSequence++
	if w.changeCapacity == 0 {
		return
	}
	change := Change{Sequence: w.changeSequence, Entity: id, Kind: kind}
	if len(w.changeHistory) < w.changeCapacity {
		w.changeHistory = append(w.changeHistory, change)
		return
	}
	w.changeHistory[(change.Sequence-1)%uint64(w.changeCapacity)] = change
}

// ChangeCursor starts an independent consumer at the current world revision.
func (w *World) ChangeCursor() uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.changeSequence
}

// ChangesSince never drains another consumer's history. A returned cursor is
// advanced only after the caller successfully consumes the complete batch.
func (w *World) ChangesSince(cursor uint64) (ChangeBatch, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	batch := ChangeBatch{Cursor: w.changeSequence}
	if cursor > w.changeSequence {
		return batch, ErrChangeHistoryExpired
	}
	if w.changeSequence-cursor > uint64(len(w.changeHistory)) {
		return batch, ErrChangeHistoryExpired
	}
	if cursor == w.changeSequence {
		return batch, nil
	}
	batch.Changes = make([]Change, 0, w.changeSequence-cursor)
	for seq := cursor + 1; seq <= w.changeSequence; seq++ {
		batch.Changes = append(batch.Changes, w.changeHistory[(seq-1)%uint64(w.changeCapacity)])
	}
	return batch, nil
}
