package service

import (
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"

	"gorm.io/gorm"
)

// Short-lived tombstone of just-deleted client emails so that a node snapshot
// arriving between delete and node-side processing doesn't resurrect them.
var (
	recentlyDeletedMu sync.Mutex
	recentlyDeleted   = map[string]time.Time{}
)

const deleteTombstoneTTL = 90 * time.Second

var (
	inboundMutationLocksMu sync.Mutex
	inboundMutationLocks   = map[int]*sync.Mutex{}

	// Tunnel address allocation spans inbounds. Per-inbound locks alone allow
	// two concurrent creates to observe the same free AllowedIPs value on
	// different inbounds and both claim it before either transaction commits.
	// Keep this short-lived process lock around the add path; the database
	// remains the durable source of truth, while this closes the in-process
	// race tested by the CRUD API.
	tunnelAddressMutationMu sync.Mutex

	// resellerScopeMutationMu serializes reseller ownership changes with
	// reseller-scoped client mutations. The database checks in a controller
	// are only a snapshot; keeping the service-level check and mutation under
	// this lock prevents an admin assignment/unassignment from changing the
	// tenant boundary between validation and the runtime operation.
	resellerScopeMutationMu sync.RWMutex
)

func lockInbound(inboundId int) *sync.Mutex {
	inboundMutationLocksMu.Lock()
	defer inboundMutationLocksMu.Unlock()
	m, ok := inboundMutationLocks[inboundId]
	if !ok {
		m = &sync.Mutex{}
		inboundMutationLocks[inboundId] = m
	}
	m.Lock()
	return m
}

func compactOrphans(db *gorm.DB, clients []any) []any {
	if len(clients) == 0 {
		return clients
	}
	emailKeys := make([]string, 0, len(clients))
	seenKeys := make(map[string]struct{}, len(clients))
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if e, _ := cm["email"].(string); e != "" {
			if key := transferEmailKey(e); key != "" {
				if _, seen := seenKeys[key]; !seen {
					seenKeys[key] = struct{}{}
					emailKeys = append(emailKeys, key)
				}
			}
		}
	}
	if len(emailKeys) == 0 {
		return clients
	}
	existing := make(map[string]struct{}, len(emailKeys))
	const orphanChunk = 400
	for start := 0; start < len(emailKeys); start += orphanChunk {
		end := min(start+orphanChunk, len(emailKeys))
		var found []string
		if err := db.Model(&model.ClientRecord{}).
			Where("LOWER(TRIM(email)) IN ?", emailKeys[start:end]).
			Pluck("email", &found).Error; err != nil {
			logger.Warning("compactOrphans pluck:", err)
			return clients
		}
		for _, e := range found {
			existing[transferEmailKey(e)] = struct{}{}
		}
	}
	if len(existing) == len(emailKeys) {
		return clients
	}
	out := make([]any, 0, len(existing))
	for _, c := range clients {
		cm, ok := c.(map[string]any)
		if !ok {
			out = append(out, c)
			continue
		}
		e, _ := cm["email"].(string)
		if e == "" {
			out = append(out, c)
			continue
		}
		if _, ok := existing[transferEmailKey(e)]; ok {
			out = append(out, c)
		}
	}
	return out
}

func tombstoneClientEmail(email string) {
	email = transferEmailKey(email)
	if email == "" {
		return
	}
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	recentlyDeleted[email] = time.Now()
	cutoff := time.Now().Add(-deleteTombstoneTTL)
	for e, ts := range recentlyDeleted {
		if ts.Before(cutoff) {
			delete(recentlyDeleted, e)
		}
	}
}

func tombstoneClientEmails(emails []string) {
	if len(emails) == 0 {
		return
	}
	now := time.Now()
	cutoff := now.Add(-deleteTombstoneTTL)
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	for _, email := range emails {
		if key := transferEmailKey(email); key != "" {
			recentlyDeleted[key] = now
		}
	}
	for e, ts := range recentlyDeleted {
		if ts.Before(cutoff) {
			delete(recentlyDeleted, e)
		}
	}
}

func isClientEmailTombstoned(email string) bool {
	email = transferEmailKey(email)
	if email == "" {
		return false
	}
	recentlyDeletedMu.Lock()
	defer recentlyDeletedMu.Unlock()
	ts, ok := recentlyDeleted[email]
	if !ok {
		return false
	}
	if time.Since(ts) > deleteTombstoneTTL {
		delete(recentlyDeleted, email)
		return false
	}
	return true
}
