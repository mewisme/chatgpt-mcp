package mcp

type RecoveryConfig struct {
	Enabled            bool
	DeleteGraceSeconds int
}

func DefaultRecovery() RecoveryConfig { return RecoveryConfig{Enabled: true, DeleteGraceSeconds: 45} }
