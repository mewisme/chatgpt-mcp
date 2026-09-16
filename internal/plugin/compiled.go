package plugin

import "sync"

var (
	compiledMu       sync.RWMutex
	compiledBuiltins BuiltinRegistry
)

func SetCompiledBuiltins(registry BuiltinRegistry) {
	compiledMu.Lock()
	compiledBuiltins = append(BuiltinRegistry(nil), registry...)
	compiledMu.Unlock()
}

func compiledBuiltinClone() BuiltinRegistry {
	compiledMu.RLock()
	defer compiledMu.RUnlock()
	return append(BuiltinRegistry(nil), compiledBuiltins...)
}
