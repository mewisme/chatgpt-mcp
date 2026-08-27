package mcp

import "time"

const SessionDeleteGrace = 45 * time.Second

type DeletePolicy struct{ Grace time.Duration }

func DefaultDeletePolicy() DeletePolicy { return DeletePolicy{Grace: SessionDeleteGrace} }
