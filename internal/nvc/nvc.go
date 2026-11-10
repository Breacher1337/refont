// Package nvc reads and writes Nova Engine .nvc archives as shipped with
// Jupiter Hell (ChaosForge).
//
// On-disk layout, all values little endian:
//
//	magic    8 bytes: "nvc1d" followed by three zero bytes
//	count    uint32  number of table of contents entries
//	entries  count * 24 byte records:
//	             uint64 hash    FNV-1a 64 hash of the original file path
//	             uint32 offset  absolute offset of the stored data
//	             uint32 rawLen  decompressed size in bytes
//	             uint32 len     stored size in bytes
//	             uint32 flags   0 stored, 1 zlib, 3 encrypted
//
// The format was first documented and implemented in jhmod
// (https://github.com/sector-f/jhmod, BSD-3-Clause) by winny and sector-f.
// This is an independent implementation written for refont.
package nvc

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
)

// ErrBadMagic is returned when a file does not start with the nvc magic bytes.
var ErrBadMagic = errors.New("not an nvc archive: bad magic bytes")

// magicBytes is the 8 byte file type header of every nvc archive.
var magicBytes = [8]byte{0x6e, 0x76, 0x63, 0x31, 0x64, 0, 0, 0} // "nvc1d"

// Storage flags of a table of contents entry.
const (
	FlagNone      uint32 = 0 // data is stored as-is
	FlagZlib      uint32 = 1 // data is zlib compressed
	FlagEncrypted uint32 = 3 // data is encrypted and opaque to this package
)

const tocEntryLen = 24 // uint64 hash plus four uint32 fields

// TocEntry is one table of contents record of an archive.
type TocEntry struct {
	Hash      uint64
	Offset    uint32
	RawLength uint32
	Length    uint32
	Flags     uint32
}

// Archive is a parsed .nvc archive. Entries are in on-disk order and their
// contents are read lazily through the underlying reader.
type Archive struct {
	Entries []TocEntry
	r       io.ReaderAt
}

// String2Hash returns the FNV-1a 64 bit hash of s, the key used for archive
// entries.
func String2Hash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// Parse reads the header and table of contents from r. Entry contents are not
// read until EntryData, EntryPeek or StoredBytes is called.
func Parse(r io.ReaderAt) (*Archive, error) {
	head := make([]byte, 8)
	if _, err := r.ReadAt(head, 0); err != nil {
		return nil, err
	}
	if !bytes.Equal(head, magicBytes[:]) {
		return nil, ErrBadMagic
	}
	cnt := make([]byte, 4)
	if _, err := r.ReadAt(cnt, 8); err != nil {
		return nil, err
	}
	count := binary.LittleEndian.Uint32(cnt)
	raw := make([]byte, tocEntryLen)
	entries := make([]TocEntry, count)
	for i := range entries {
		if _, err := r.ReadAt(raw, int64(12+i*tocEntryLen)); err != nil {
			return nil, err
		}
		entries[i] = TocEntry{
			Hash:      binary.LittleEndian.Uint64(raw[0:8]),
			Offset:    binary.LittleEndian.Uint32(raw[8:12]),
			RawLength: binary.LittleEndian.Uint32(raw[12:16]),
			Length:    binary.LittleEndian.Uint32(raw[16:20]),
			Flags:     binary.LittleEndian.Uint32(raw[20:24]),
		}
	}
	return &Archive{Entries: entries, r: r}, nil
}

// ParseFile opens path and parses its header and table of contents.
func ParseFile(path string) (*Archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	a, err := Parse(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return a, nil
}

// Close closes the underlying file, if the archive was opened from one.
func (a *Archive) Close() error {
	if c, ok := a.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// StoredBytes returns the on-disk bytes of entry i. This works for every
// storage mode, including encrypted entries, and never decompresses.
func (a *Archive) StoredBytes(i int) ([]byte, error) {
	e := a.Entries[i]
	buf := make([]byte, e.Length)
	if _, err := a.r.ReadAt(buf, int64(e.Offset)); err != nil {
		return nil, fmt.Errorf("entry %d: reading %d bytes at offset %d: %w", i, e.Length, e.Offset, err)
	}
	return buf, nil
}

// EntryData returns the decompressed contents of entry i.
func (a *Archive) EntryData(i int) ([]byte, error) {
	e := a.Entries[i]
	switch e.Flags {
	case FlagNone:
		return a.StoredBytes(i)
	case FlagZlib:
		stored, err := a.StoredBytes(i)
		if err != nil {
			return nil, err
		}
		zr, err := zlib.NewReader(bytes.NewReader(stored))
		if err != nil {
			return nil, fmt.Errorf("entry %d: zlib stream: %w", i, err)
		}
		defer zr.Close()
		return io.ReadAll(io.LimitReader(zr, int64(e.RawLength)))
	default:
		return nil, fmt.Errorf("entry %d: unsupported flags %d (encrypted entries cannot be extracted)", i, e.Flags)
	}
}

// EntryPeek returns up to n decompressed bytes from the start of entry i
// without materializing the entire entry.
func (a *Archive) EntryPeek(i int, n int) ([]byte, error) {
	e := a.Entries[i]
	if e.Flags == FlagZlib {
		zr, err := zlib.NewReader(io.NewSectionReader(a.r, int64(e.Offset), int64(e.Length)))
		if err != nil {
			return nil, fmt.Errorf("entry %d: zlib stream: %w", i, err)
		}
		defer zr.Close()
		buf := make([]byte, n)
		m, err := io.ReadFull(zr, buf)
		if err != nil && m == 0 {
			return nil, err
		}
		return buf[:m], nil
	}
	want := int(e.Length)
	if n < want {
		want = n
	}
	buf := make([]byte, want)
	if _, err := a.r.ReadAt(buf, int64(e.Offset)); err != nil {
		return nil, err
	}
	return buf, nil
}

// Writer builds a .nvc archive. The number of entries must be known up front
// because the table of contents is written at the start of the file.
type Writer struct {
	w      io.WriteSeeker
	toc    []TocEntry
	offset uint32
	index  int
}

// NewWriter writes placeholder space for the header and positions the stream
// at the start of the data section. w must be positioned at the start of an
// empty or truncated file, for example one from os.Create.
func NewWriter(w io.WriteSeeker, count int) (*Writer, error) {
	if count < 0 {
		return nil, errors.New("negative entry count")
	}
	headerLen := 8 + 4 + int64(count)*tocEntryLen
	if _, err := w.Write(make([]byte, headerLen)); err != nil {
		return nil, err
	}
	return &Writer{w: w, toc: make([]TocEntry, count), offset: uint32(headerLen)}, nil
}

// WriteRaw appends data as pre-stored bytes with the given storage flags.
// rawLength must be the decompressed size, equal to len(data) for FlagNone.
func (w *Writer) WriteRaw(data []byte, rawLength uint32, hash uint64, flags uint32) error {
	if w.index >= len(w.toc) {
		return errors.New("more entries written than declared in NewWriter")
	}
	if flags == FlagNone && rawLength != uint32(len(data)) {
		return fmt.Errorf("uncompressed entry: raw length %d does not match data length %d", rawLength, len(data))
	}
	e := &w.toc[w.index]
	w.index++
	e.Hash = hash
	e.Offset = w.offset
	e.RawLength = rawLength
	e.Length = uint32(len(data))
	e.Flags = flags
	if _, err := w.w.Write(data); err != nil {
		return err
	}
	w.offset += uint32(len(data))
	return nil
}

// WriteCompressed appends the contents of r, zlib compressed.
func (w *Writer) WriteCompressed(r io.Reader, hash uint64) error {
	if w.index >= len(w.toc) {
		return errors.New("more entries written than declared in NewWriter")
	}
	e := &w.toc[w.index]
	w.index++
	cw := &countingWriter{w: w.w}
	zw := zlib.NewWriter(cw)
	n, err := io.Copy(zw, r)
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	e.Hash = hash
	e.Offset = w.offset
	e.RawLength = uint32(n)
	e.Length = uint32(cw.n)
	e.Flags = FlagZlib
	w.offset += uint32(cw.n)
	return nil
}

// Finalize seeks back to the start of the file and writes the header and the
// table of contents. It must be called once, after all entries are written.
func (w *Writer) Finalize() error {
	if w.index != len(w.toc) {
		return fmt.Errorf("wrote %d of %d declared entries", w.index, len(w.toc))
	}
	if _, err := w.w.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err := w.w.Write(magicBytes[:]); err != nil {
		return err
	}
	if err := binary.Write(w.w, binary.LittleEndian, uint32(len(w.toc))); err != nil {
		return err
	}
	var raw [tocEntryLen]byte
	for _, e := range w.toc {
		binary.LittleEndian.PutUint64(raw[0:8], e.Hash)
		binary.LittleEndian.PutUint32(raw[8:12], e.Offset)
		binary.LittleEndian.PutUint32(raw[12:16], e.RawLength)
		binary.LittleEndian.PutUint32(raw[16:20], e.Length)
		binary.LittleEndian.PutUint32(raw[20:24], e.Flags)
		if _, err := w.w.Write(raw[:]); err != nil {
			return err
		}
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n uint64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += uint64(n)
	return n, err
}
