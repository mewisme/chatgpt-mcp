package cluster

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const ProtocolVersion = 1

type Advertisement struct {
	InstanceID  string   `json:"instance_id"`
	Name        string   `json:"name"`
	CatalogHash string   `json:"catalog_hash,omitempty"`
	Workspaces  []string `json:"workspaces,omitempty"`
}

type Member struct {
	InstanceID  string    `json:"instance_id"`
	Name        string    `json:"name"`
	CatalogHash string    `json:"catalog_hash,omitempty"`
	Workspaces  []string  `json:"workspaces,omitempty"`
	Online      bool      `json:"online"`
	ConnectedAt time.Time `json:"connected_at,omitempty"`
	LastSeen    time.Time `json:"last_seen,omitempty"`
}

type WorkspaceOwner struct {
	WorkspaceID string `json:"workspace_id"`
	InstanceID  string `json:"instance_id"`
	Online      bool   `json:"online"`
}

type LeaderLease struct {
	TunnelID   string    `json:"tunnel_id"`
	InstanceID string    `json:"instance_id"`
	Epoch      uint64    `json:"epoch"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type Snapshot struct {
	Members           []Member         `json:"members"`
	Workspaces        []WorkspaceOwner `json:"workspaces"`
	Leaders           []LeaderLease    `json:"leaders,omitempty"`
	CatalogHash       string           `json:"catalog_hash,omitempty"`
	CatalogCompatible bool             `json:"catalog_compatible"`
	CatalogError      string           `json:"catalog_error,omitempty"`
}

type FrameKind string

const (
	FrameRPCRequest  FrameKind = "rpc_request"
	FrameRPCResponse FrameKind = "rpc_response"
)

type Frame struct {
	Version        int             `json:"version"`
	Kind           FrameKind       `json:"kind"`
	FromInstanceID string          `json:"from_instance_id"`
	ToInstanceID   string          `json:"to_instance_id"`
	RequestID      string          `json:"request_id"`
	Payload        json.RawMessage `json:"payload,omitempty"`
}

type RPCRequest struct {
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type RPCResponse struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

var (
	ErrClosed       = errors.New("cluster session is closed")
	ErrOwnerOffline = errors.New("workspace owner is offline")
	ErrNoOwner      = errors.New("workspace owner not found")
	ErrLeaseLost    = errors.New("cluster leader lease lost")
)

func catalogStatus(members []Member) (string, bool, string) {
	online := 0
	hashes := map[string]bool{}
	missing := false
	for _, member := range members {
		if !member.Online {
			continue
		}
		online++
		hash := strings.TrimSpace(member.CatalogHash)
		if hash == "" {
			missing = true
			continue
		}
		hashes[hash] = true
	}
	if online <= 1 {
		for hash := range hashes {
			return hash, true, ""
		}
		return "", true, ""
	}
	if missing {
		return "", false, "one or more online runtimes do not advertise a catalog hash"
	}
	if len(hashes) != 1 {
		return "", false, "online runtimes advertise different tool catalogs"
	}
	for hash := range hashes {
		return hash, true, ""
	}
	return "", false, "tool catalog is unavailable"
}

func validateAdvertisement(value Advertisement) error {
	value.InstanceID = strings.TrimSpace(value.InstanceID)
	value.Name = strings.TrimSpace(value.Name)
	if value.InstanceID == "" {
		return errors.New("cluster instance_id is required")
	}
	if value.Name == "" {
		return errors.New("cluster instance name is required")
	}
	seen := map[string]bool{}
	for _, workspaceID := range value.Workspaces {
		workspaceID = strings.TrimSpace(workspaceID)
		if workspaceID == "" {
			return errors.New("cluster workspace_id is required")
		}
		if seen[workspaceID] {
			return errors.New("cluster workspace advertisement contains duplicate workspace_id")
		}
		seen[workspaceID] = true
	}
	return nil
}

func normalizeWorkspaces(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type RuntimeStatus struct {
	Enabled           bool             `json:"enabled"`
	Connected         bool             `json:"connected"`
	RelayURL          string           `json:"relay_url,omitempty"`
	InstanceID        string           `json:"instance_id,omitempty"`
	Name              string           `json:"name,omitempty"`
	MemberCount       int              `json:"member_count"`
	OnlineMemberCount int              `json:"online_member_count"`
	WorkspaceCount    int              `json:"workspace_count"`
	CatalogHash       string           `json:"catalog_hash,omitempty"`
	CatalogCompatible bool             `json:"catalog_compatible"`
	CatalogError      string           `json:"catalog_error,omitempty"`
	TunnelRole        string           `json:"tunnel_role,omitempty"`
	LeaderInstanceID  string           `json:"leader_instance_id,omitempty"`
	LeaderEpoch       uint64           `json:"leader_epoch,omitempty"`
	LeaseExpiresAt    time.Time        `json:"lease_expires_at,omitempty"`
	LastError         string           `json:"last_error,omitempty"`
	Members           []Member         `json:"members,omitempty"`
	Workspaces        []WorkspaceOwner `json:"workspaces,omitempty"`
}
