// Package localdisk menyimpan isi file di disk lokal (FILESTORE_DIR). Dipakai sampai adapter object storage
// S3-compatible dibangun; antarmukanya sama sehingga service filestore tidak berubah.
package localdisk

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Disk menyimpan objek di <dir>/<2 karakter awal key>/<key>.
type Disk struct {
	dir string
}

// New membuat Disk dan memastikan folder ada.
func New(dir string) (*Disk, error) {
	if dir == "" {
		return nil, errors.New("localdisk: folder wajib diisi")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("localdisk: buat folder %s: %w", dir, err)
	}
	return &Disk{dir: dir}, nil
}

func (d *Disk) path(key uuid.UUID) string {
	k := key.String()
	return filepath.Join(d.dir, k[:2], k)
}

// Put menulis isi objek secara atomik (file sementara lalu rename) dan mengembalikan jumlah byte serta SHA-256.
// Objek yang sudah ada ditimpa.
func (d *Disk) Put(ctx context.Context, key uuid.UUID, r io.Reader) (int64, []byte, error) {
	target := d.path(key)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return 0, nil, fmt.Errorf("localdisk: buat folder: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return 0, nil, fmt.Errorf("localdisk: file sementara: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // sudah di-rename jika berhasil

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), ctxReader{ctx: ctx, r: r})
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, nil, fmt.Errorf("localdisk: tulis: %w", err)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return n, nil, fmt.Errorf("localdisk: rename: %w", err)
	}
	return n, h.Sum(nil), nil
}

// Open membuka objek untuk dibaca.
func (d *Disk) Open(_ context.Context, key uuid.UUID) (io.ReadSeekCloser, error) {
	f, err := os.Open(d.path(key))
	if err != nil {
		return nil, fmt.Errorf("localdisk: buka: %w", err)
	}
	return f, nil
}

// Delete menghapus objek; objek yang tidak ada tidak dianggap error.
func (d *Disk) Delete(_ context.Context, key uuid.UUID) error {
	if err := os.Remove(d.path(key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("localdisk: hapus: %w", err)
	}
	return nil
}

// ctxReader menghentikan penyalinan saat request dibatalkan.
type ctxReader struct {
	ctx context.Context // hanya hidup selama satu panggilan Put
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
