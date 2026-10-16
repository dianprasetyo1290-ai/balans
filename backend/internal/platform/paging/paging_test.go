package paging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"balans/internal/platform/paging"
)

func ptr[T any](v T) *T { return &v }

func TestNew(t *testing.T) {
	assert.Equal(t, paging.Page{Number: 1, Size: 50}, paging.New(nil, nil))
	assert.Equal(t, paging.Page{Number: 3, Size: 200}, paging.New(ptr(3), ptr(1000)))
	assert.Equal(t, paging.Page{Number: 1, Size: 50}, paging.New(ptr(0), ptr(-1)))

	p := paging.New(ptr(3), ptr(20))
	assert.Equal(t, int32(40), p.Offset())
	assert.Equal(t, int32(20), p.Limit())
	assert.Equal(t, 0, p.TotalPages(0))
	assert.Equal(t, 1, p.TotalPages(20))
	assert.Equal(t, 2, p.TotalPages(21))
}

func TestParseSort(t *testing.T) {
	allowed := []string{"sku", "createdAt"}
	fallback := paging.Sort{Field: "createdAt", Desc: true}

	assert.Equal(t, fallback, paging.ParseSort(nil, allowed, fallback))
	assert.Equal(t, paging.Sort{Field: "sku"}, paging.ParseSort(ptr("sku"), allowed, fallback))
	assert.Equal(t, paging.Sort{Field: "sku", Desc: true}, paging.ParseSort(ptr("hack,-sku"), allowed, fallback))
	assert.Equal(t, fallback, paging.ParseSort(ptr("password"), allowed, fallback))
	assert.Equal(t, "-sku", paging.Sort{Field: "sku", Desc: true}.Key())
	assert.Equal(t, "sku", paging.Sort{Field: "sku"}.Key())
}
