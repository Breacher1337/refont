package nvc

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func fakeTtf(payload string) []byte {
	out := make([]byte, 4, 4+len(payload))
	copy(out, []byte{0x00, 0x01, 0x00, 0x00})
	return append(out, payload...)
}

func luaBytes() []byte {
	return []byte(`register_blueprint "x" {}`)
}

func writeSample(t *testing.T, ttf []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.nvc")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriter(f, 3)
	if err != nil {
		t.Fatal(err)
	}
	lua := luaBytes()
	if err := w.WriteRaw(lua, uint32(len(lua)), String2Hash("data/lua/text.lua"), FlagNone); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteCompressed(bytes.NewReader(ttf), String2Hash("data/font/main.ttf")); err != nil {
		t.Fatal(err)
	}
	csv := []byte("key,value\n")
	if err := w.WriteRaw(csv, uint32(len(csv)), String2Hash("data/lang/en.csv"), FlagNone); err != nil {
		t.Fatal(err)
	}
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRoundTrip(t *testing.T) {
	ttf := fakeTtf("GLYF")
	path := writeSample(t, ttf)
	a, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(a.Entries))
	}
	if a.Entries[0].Hash != String2Hash("data/lua/text.lua") {
		t.Error("entry 0 hash mismatch")
	}
	if a.Entries[1].Hash != String2Hash("data/font/main.ttf") {
		t.Error("entry 1 hash mismatch")
	}
	if a.Entries[1].Flags != FlagZlib {
		t.Errorf("entry 1 flags = %d, want %d", a.Entries[1].Flags, FlagZlib)
	}
	got0, err := a.EntryData(0)
	if err != nil || !bytes.Equal(got0, luaBytes()) {
		t.Errorf("entry 0 data mismatch: %q, %v", got0, err)
	}
	got1, err := a.EntryData(1)
	if err != nil || !bytes.Equal(got1, ttf) {
		t.Errorf("entry 1 data mismatch: %v", err)
	}
}

func TestByteIdenticalCopy(t *testing.T) {
	path := writeSample(t, fakeTtf("COPY"))
	a, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "copy.nvc")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriter(out, len(a.Entries))
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.Entries {
		e := &a.Entries[i]
		stored, err := a.StoredBytes(i)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteRaw(stored, e.RawLength, e.Hash, e.Flags); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	copyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orig, copyBytes) {
		t.Error("repacked archive is not byte identical to the original")
	}
}

func TestSwapFontEntry(t *testing.T) {
	path := writeSample(t, fakeTtf("ORIGINAL"))
	a, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	newFont := fakeTtf("NOTOSANSJP")
	outPath := filepath.Join(t.TempDir(), "out.nvc")
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	w, err := NewWriter(out, len(a.Entries))
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.Entries {
		e := &a.Entries[i]
		if i == 1 {
			if err := w.WriteCompressed(bytes.NewReader(newFont), e.Hash); err != nil {
				t.Fatal(err)
			}
			continue
		}
		stored, err := a.StoredBytes(i)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteRaw(stored, e.RawLength, e.Hash, e.Flags); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Finalize(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := ParseFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.Entries[0].Hash != a.Entries[0].Hash || b.Entries[2].Hash != a.Entries[2].Hash {
		t.Error("entry order or hashes not preserved")
	}
	got, err := b.EntryData(1)
	if err != nil || !bytes.Equal(got, newFont) {
		t.Errorf("swapped font not present: %v", err)
	}
	got0, err := b.EntryData(0)
	if err != nil || !bytes.Equal(got0, luaBytes()) {
		t.Error("untouched entry corrupted")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBadMagic(t *testing.T) {
	if _, err := Parse(bytes.NewReader([]byte("garbage"))); err == nil {
		t.Error("expected an error for a file with bad magic bytes")
	}
}
