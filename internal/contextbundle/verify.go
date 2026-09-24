package contextbundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

// Verify re-authorizes and re-hashes before a host hands the bundle to a Runtime.
// Read-only modes protect against accidents, not a malicious same-UID process.
// A sandbox read-only mount/separate identity must enforce that stronger boundary.
func (m *Materializer) Verify(ctx context.Context, input core.RunInputManifest) (Bundle, error) {
	input, digest, err := m.prepare(ctx, input)
	if err != nil {
		return Bundle{}, err
	}
	name := strings.TrimPrefix(digest, "sha256:")
	info, err := m.root.Lstat(name)
	if err != nil {
		return Bundle{}, err
	}
	if !info.IsDir() || info.Mode().Perm()&0o222 != 0 {
		return Bundle{}, ErrUnsafePath
	}
	root, err := m.root.OpenRoot(name)
	if err != nil {
		return Bundle{}, err
	}
	defer root.Close()
	manifestFile, err := openRegular(root, "manifest.json")
	if err != nil {
		return Bundle{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(contextReader{ctx, manifestFile}, (128<<10)+1))
	closeErr := manifestFile.Close()
	if readErr != nil {
		return Bundle{}, readErr
	}
	if closeErr != nil {
		return Bundle{}, closeErr
	}
	if len(data) > 128<<10 {
		return Bundle{}, ErrLimit
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Bundle{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Bundle{}, fmt.Errorf("invalid manifest framing")
	}
	if manifest.SchemaVersion != 1 || manifest.RunInputDigest != digest || len(manifest.Entries) != len(input.ContextRefs) {
		return Bundle{}, ErrDigest
	}
	// Canonical manifest bytes only: whitespace/reordering is not another identity.
	canonicalData, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(data, canonicalData) {
		return Bundle{}, ErrDigest
	}
	directory, err := root.Open(".")
	if err != nil {
		return Bundle{}, err
	}
	files, readErr := directory.ReadDir(len(input.ContextRefs) + 2)
	_ = directory.Close()
	if readErr != nil && readErr != io.EOF {
		return Bundle{}, readErr
	}
	if len(files) != len(input.ContextRefs)+1 {
		return Bundle{}, ErrDigest
	}
	var total int64
	for i, ref := range input.ContextRefs {
		entry := manifest.Entries[i]
		if entry.Ref != ref || entry.File != entryName(i, ref) || entry.Size < 0 || entry.Size > m.limits.EntryBytes {
			return Bundle{}, ErrDigest
		}
		total += entry.Size
		if total > m.limits.TotalBytes {
			return Bundle{}, ErrLimit
		}
		file, err := openRegular(root, entry.File)
		if err != nil {
			return Bundle{}, err
		}
		hash := sha256.New()
		size, readErr := io.Copy(hash, io.LimitReader(contextReader{ctx, file}, entry.Size+1))
		closeErr := file.Close()
		if readErr != nil {
			return Bundle{}, readErr
		}
		if closeErr != nil {
			return Bundle{}, closeErr
		}
		if size != entry.Size || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != ref.Digest {
			return Bundle{}, ErrDigest
		}
	}
	return Bundle{Path: filepath.Join(m.path, name), Digest: canonical.BytesDigest(data), Manifest: manifest}, nil
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return nil, ErrUnsafePath
	}
	return root.Open(name)
}
