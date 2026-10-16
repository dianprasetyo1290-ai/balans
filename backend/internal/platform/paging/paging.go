// Package paging menormalkan parameter pagination dan sort standar (Technical Guideline 8.2).
package paging

import (
	"slices"
	"strings"
)

const (
	DefaultSize = 50
	MaxSize     = 200
)

// Page adalah halaman yang diminta (mulai dari 1).
type Page struct {
	Number int
	Size   int
}

// New menormalkan nilai dari query string; nil berarti default.
func New(number, size *int) Page {
	p := Page{Number: 1, Size: DefaultSize}
	if number != nil && *number > 0 {
		p.Number = *number
	}
	if size != nil && *size > 0 {
		p.Size = min(*size, MaxSize)
	}
	return p
}

// Offset untuk query SQL.
func (p Page) Offset() int32 { return int32((p.Number - 1) * p.Size) } //nolint:gosec // dibatasi MaxSize

// Limit untuk query SQL.
func (p Page) Limit() int32 { return int32(p.Size) } //nolint:gosec // dibatasi MaxSize

// TotalPages menghitung jumlah halaman dari total baris.
func (p Page) TotalPages(total int64) int {
	if total == 0 {
		return 0
	}
	return int((total + int64(p.Size) - 1) / int64(p.Size))
}

// Result adalah satu halaman hasil list.
type Result[T any] struct {
	Items []T
	Page  Page
	Total int64
}

// Sort adalah satu field sort.
type Sort struct {
	Field string
	Desc  bool
}

// ParseSort membaca `-createdAt,sku`. Field yang tidak ada di allowed diabaikan; hasil kosong memakai fallback.
func ParseSort(s *string, allowed []string, fallback Sort) Sort {
	if s != nil {
		for _, part := range strings.Split(*s, ",") {
			part = strings.TrimSpace(part)
			desc := strings.HasPrefix(part, "-")
			field := strings.TrimPrefix(part, "-")
			if slices.Contains(allowed, field) {
				return Sort{Field: field, Desc: desc}
			}
		}
	}
	return fallback
}

// Key mengembalikan kunci sort untuk query sqlc, mis. `sku` atau `-createdAt`.
func (s Sort) Key() string {
	if s.Desc {
		return "-" + s.Field
	}
	return s.Field
}
