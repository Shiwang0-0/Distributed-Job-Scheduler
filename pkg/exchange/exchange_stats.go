package exchange

func (e *ExchangeStats) IncrementRouted() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.JobsRouted++
}

func (e *ExchangeStats) IncrementErrors() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.RoutingErrors++
}

func (e *ExchangeStats) GetStats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return map[string]interface{}{
		"jobs_routed":    e.JobsRouted,
		"routing_errors": e.RoutingErrors,
	}
}
