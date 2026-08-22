package application

import (
	"sync"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// PolicySource hands out the current immutable registry snapshot. Saving a
// policy through the catalog replaces the snapshot, so the change takes
// effect on the next request without a restart or rebuild.
type PolicySource struct {
	mu      sync.RWMutex
	current *domain.Registry
}

// NewPolicySource pins the initial registry loaded at startup.
func NewPolicySource(registry *domain.Registry) *PolicySource {
	return &PolicySource{current: registry}
}

// Current returns the registry snapshot valid at this instant.
func (source *PolicySource) Current() *domain.Registry {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.current
}

// Replace installs the next registry snapshot.
func (source *PolicySource) Replace(registry *domain.Registry) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.current = registry
}
