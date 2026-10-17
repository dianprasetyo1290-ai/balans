// Package domain berisi entri audit trail dan perhitungan diff (NFR-03, Technical Guideline 6.6).
package domain

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Action adalah jenis aksi yang dicatat.
type Action string

const (
	ActionCreate       Action = "create"
	ActionUpdate       Action = "update"
	ActionDelete       Action = "delete"
	ActionRestore      Action = "restore"
	ActionStatusChange Action = "status_change"
	ActionApprove      Action = "approve"
	ActionReject       Action = "reject"
	ActionSubmit       Action = "submit"
	ActionPost         Action = "post"
	ActionCancel       Action = "cancel"
	ActionLogin        Action = "login"
	ActionLoginFailed  Action = "login_failed"
	ActionLogout       Action = "logout"
	ActionImport       Action = "import"
	ActionExport       Action = "export"
)

// MaskedValue menggantikan nilai field sensitif di diff.
const MaskedValue = "••••"

// sensitiveFields dimask di semua entitas. Nama field mengikuti JSON entity (camelCase).
var sensitiveFields = []string{
	"password", "passwordHash", "mfaSecret", "nik", "npwp", "bankAccountNumber", "accountNumber", "salary",
}

// Entry adalah perubahan yang dicatat modul pemilik data.
type Entry struct {
	Action      Action
	EntityType  string
	EntityID    uuid.UUID
	EntityLabel string
	// Before dan After adalah snapshot entitas (struct atau map) yang di-marshal ke JSON untuk dihitung diff-nya.
	Before any
	After  any
	// MaskFields menambahkan field sensitif khusus entitas ini.
	MaskFields []string
	Note       string
}

// Change adalah perubahan satu field.
type Change struct {
	From any `json:"from,omitempty"`
	To   any `json:"to,omitempty"`
}

// Log adalah baris audit trail tersimpan.
type Log struct {
	ID          uuid.UUID
	ActorID     uuid.UUID
	ActorName   string
	Action      Action
	EntityType  string
	EntityID    uuid.UUID
	EntityLabel string
	Changes     map[string]Change
	Note        string
	RequestID   string
	IP          string
	CreatedAt   time.Time
}

// Diff menghitung perubahan per field antara Before dan After. Field sensitif dimask; field yang sama diabaikan.
func (e Entry) Diff() (map[string]Change, error) {
	before, err := toMap(e.Before)
	if err != nil {
		return nil, fmt.Errorf("audit before: %w", err)
	}
	after, err := toMap(e.After)
	if err != nil {
		return nil, fmt.Errorf("audit after: %w", err)
	}
	masked := append(slices.Clone(sensitiveFields), e.MaskFields...)
	keys := slices.Sorted(maps.Keys(before))
	for k := range after {
		if _, ok := before[k]; !ok {
			keys = append(keys, k)
		}
	}
	changes := map[string]Change{}
	for _, k := range keys {
		if ignoredField(k) {
			continue
		}
		from, hasFrom := before[k]
		to, hasTo := after[k]
		if hasFrom && hasTo && jsonEqual(from, to) {
			continue
		}
		// Field kosong yang tidak ada di sisi lain (mis. field null saat create) bukan perubahan.
		if from == nil && to == nil {
			continue
		}
		if isMasked(k, masked) {
			c := Change{}
			if hasFrom && from != nil {
				c.From = MaskedValue
			}
			if hasTo && to != nil {
				c.To = MaskedValue
			}
			changes[k] = c
			continue
		}
		changes[k] = Change{From: from, To: to}
	}
	return changes, nil
}

// ignoredField: metadata yang selalu berubah dan tidak bermakna sebagai diff.
func ignoredField(k string) bool {
	switch k {
	case "updatedAt", "updatedBy", "version", "createdAt", "createdBy":
		return true
	}
	return false
}

func isMasked(field string, masked []string) bool {
	for _, m := range masked {
		if strings.EqualFold(field, m) || strings.HasSuffix(strings.ToLower(field), "enc") {
			return true
		}
	}
	return false
}

func toMap(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if string(b) == "null" {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func jsonEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ab) == string(bb)
}

// Filter adalah kriteria list audit trail.
type Filter struct {
	EntityType string
	EntityID   uuid.UUID
	ActorID    uuid.UUID
	Action     Action
	From, To   *time.Time
}
