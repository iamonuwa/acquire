package normalize

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

const zipMembersFingerprint = "zip_members-1"

const (
	maxArchiveBytes = 200 << 20 // 200 MiB compressed
	maxMembers      = 100
	maxUncompressed = 1 << 30 // 1 GiB decompressed, guards a 1 GB droplet against zip bombs
)

// zipMembers validates the archive, then returns its members concatenated in
// sorted name order, each under a header line. Oversize, too many members,
// decompression bombs, and path traversal all fail before any content is read.
func zipMembers(raw []byte) ([]byte, error) {
	if len(raw) > maxArchiveBytes {
		return nil, fmt.Errorf("normalize: archive is %d bytes, over the %d limit", len(raw), maxArchiveBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("normalize: open archive: %w", err)
	}
	if len(zr.File) > maxMembers {
		return nil, fmt.Errorf("normalize: archive has %d members, over the %d limit", len(zr.File), maxMembers)
	}

	byName := make(map[string]*zip.File, len(zr.File))
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		if !safeMemberName(f.Name) {
			return nil, fmt.Errorf("normalize: unsafe member path %q", f.Name)
		}
		byName[f.Name] = f
		names = append(names, f.Name)
	}
	sort.Strings(names)

	var out bytes.Buffer
	budget := int64(maxUncompressed)
	for _, name := range names {
		rc, err := byName[name].Open()
		if err != nil {
			return nil, fmt.Errorf("normalize: open member %q: %w", name, err)
		}
		fmt.Fprintf(&out, "===== %s =====\n", name)
		n, err := io.Copy(&out, io.LimitReader(rc, budget+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("normalize: read member %q: %w", name, err)
		}
		if budget -= n; budget < 0 {
			return nil, fmt.Errorf("normalize: archive expands past the %d limit", maxUncompressed)
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// safeMemberName rejects absolute paths and parent-directory traversal.
func safeMemberName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "/") && !strings.Contains(name, "..")
}
