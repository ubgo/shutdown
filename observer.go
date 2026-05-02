package shutdown

import (
	"os"
	"time"
)

// fireOnSignal invokes each observer's OnSignal hook.
func (m *Manager) fireOnSignal(sig os.Signal) {
	m.mu.RLock()
	obs := append([]Observer(nil), m.observers...)
	m.mu.RUnlock()
	for _, o := range obs {
		if o.OnSignal != nil {
			o.OnSignal(sig)
		}
	}
}

// fireOnPhaseStart invokes each observer's OnPhaseStart hook.
func (m *Manager) fireOnPhaseStart(p Phase, count int) {
	m.mu.RLock()
	obs := append([]Observer(nil), m.observers...)
	m.mu.RUnlock()
	for _, o := range obs {
		if o.OnPhaseStart != nil {
			o.OnPhaseStart(p, count)
		}
	}
}

// fireOnPhaseEnd invokes each observer's OnPhaseEnd hook.
func (m *Manager) fireOnPhaseEnd(p Phase, dur time.Duration, errs []error) {
	m.mu.RLock()
	obs := append([]Observer(nil), m.observers...)
	m.mu.RUnlock()
	for _, o := range obs {
		if o.OnPhaseEnd != nil {
			o.OnPhaseEnd(p, dur, errs)
		}
	}
}

// fireOnHandlerStart invokes each observer's OnHandlerStart hook.
func (m *Manager) fireOnHandlerStart(name string, p Phase) {
	m.mu.RLock()
	obs := append([]Observer(nil), m.observers...)
	m.mu.RUnlock()
	for _, o := range obs {
		if o.OnHandlerStart != nil {
			o.OnHandlerStart(name, p)
		}
	}
}

// fireOnHandlerEnd invokes each observer's OnHandlerEnd hook.
func (m *Manager) fireOnHandlerEnd(name string, p Phase, dur time.Duration, err error) {
	m.mu.RLock()
	obs := append([]Observer(nil), m.observers...)
	m.mu.RUnlock()
	for _, o := range obs {
		if o.OnHandlerEnd != nil {
			o.OnHandlerEnd(name, p, dur, err)
		}
	}
}

// fireOnComplete invokes each observer's OnComplete hook.
func (m *Manager) fireOnComplete(totalDur time.Duration, err error) {
	m.mu.RLock()
	obs := append([]Observer(nil), m.observers...)
	m.mu.RUnlock()
	for _, o := range obs {
		if o.OnComplete != nil {
			o.OnComplete(totalDur, err)
		}
	}
}
