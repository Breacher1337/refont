// Command refont replaces fonts inside game archives with fonts of your own
// choosing. Version 0.1 supports Nova Engine .nvc archives (Jupiter Hell).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Breacher1337/refont/internal/font"
	"github.com/Breacher1337/refont/internal/nvc"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var err error
	switch cmd {
	case "scan":
		err = cmdScan(args)
	case "swap":
		err = cmdSwap(args)
	case "extract":
		err = cmdExtract(args)
	case "version":
		fmt.Println("refont " + version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "refont: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "refont: "+err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`refont ` + version + ` - replace ugly game fonts with your own

Usage:

  refont scan ARCHIVE.nvc [flags]
      List archive entries and detect font files.

  refont swap ARCHIVE.nvc -font MYFONT.ttf [flags]
      Replace font entries inside the archive with your own font.

  refont extract ARCHIVE.nvc [flags]
      Dump archive contents for inspection.

  refont version
      Print the version.

scan flags:
  -pathlist FILE  optional file with original paths, one per line, used to
                  show entry names (see the jhmod tool)
  -peek N         bytes to inspect per entry for font detection (default 16)

swap flags:
  -font FILE      replacement font (.ttf, .otf or .ttc), required
  -index LIST     comma separated entry indices to replace (from refont scan)
  -hash LIST      comma separated entry hashes (hex) to replace
  -out FILE       write the patched archive to this path instead of patching
                  in place
  -backup         keep a timestamped backup next to the original (default true)
  -force          allow a replacement file that is not a detected font format
  -dry-run        show the plan without writing anything

extract flags:
  -out DIR        output directory (default refont-extract)
  -pathlist FILE  as in scan, used to name files
  -fonts-only     extract only detected font files

Examples:

  refont scan core.nvc
  refont swap core.nvc -font NotoSansJP-Regular.ttf
  refont swap core.nvc -font ZenMaruGothic-Medium.ttf -index 42 -dry-run

When neither -index nor -hash is given, refont swap replaces every entry that
looks like a font file. Untouched entries are copied byte for byte, so the
patched archive differs from the original only in the entries you replaced.
`)
}

// loadPathlist reads a file with one original path per line and maps the
// FNV-1a hash of each path to the path itself.
func loadPathlist(path string) (map[uint64]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	names := make(map[uint64]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names[nvc.String2Hash(line)] = line
	}
	return names, nil
}

func cmdScan(args []string) error {
	fs := flag.NewFlagSet("refont scan", flag.ContinueOnError)
	pathlistPath := fs.String("pathlist", "", "file with original paths, one per line")
	peek := fs.Int("peek", 16, "bytes to inspect per entry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: refont scan ARCHIVE.nvc [flags]")
	}
	names, err := loadPathlist(*pathlistPath)
	if err != nil {
		return err
	}
	a, err := nvc.ParseFile(fs.Arg(0))
	if err != nil {
		return err
	}
	defer a.Close()
	fmt.Printf("%s: %d entries\n\n", fs.Arg(0), len(a.Entries))
	fmt.Printf("%5s  %-16s %5s %12s %12s  %-5s %s\n", "idx", "hash", "flags", "raw", "stored", "kind", "name")
	for i := range a.Entries {
		e := &a.Entries[i]
		kind := ""
		if e.Flags == nvc.FlagEncrypted {
			kind = "?"
		} else if head, perr := a.EntryPeek(i, *peek); perr == nil {
			kind = string(font.Detect(head))
		}
		fmt.Printf("%5d  %016x %5d %12d %12d  %-5s %s\n", i, e.Hash, e.Flags, e.RawLength, e.Length, kind, names[e.Hash])
	}
	return nil
}

func cmdSwap(args []string) error {
	fs := flag.NewFlagSet("refont swap", flag.ContinueOnError)
	fontPath := fs.String("font", "", "replacement font file (.ttf, .otf, .ttc)")
	indexList := fs.String("index", "", "comma separated entry indices to replace")
	hashList := fs.String("hash", "", "comma separated entry hashes (hex) to replace")
	outPath := fs.String("out", "", "write the patched archive to this path")
	backup := fs.Bool("backup", true, "keep a timestamped backup of the original")
	force := fs.Bool("force", false, "allow a replacement file that is not a detected font format")
	dryRun := fs.Bool("dry-run", false, "print the plan without writing anything")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: refont swap ARCHIVE.nvc -font MYFONT.ttf [flags]")
	}
	if *fontPath == "" {
		return fmt.Errorf("-font is required")
	}
	newFont, err := os.ReadFile(*fontPath)
	if err != nil {
		return err
	}
	if font.Detect(newFont) == "" && !*force {
		return fmt.Errorf("%s does not look like a .ttf, .otf or .ttc file; pass -force to override", *fontPath)
	}
	archivePath := fs.Arg(0)
	a, err := nvc.ParseFile(archivePath)
	if err != nil {
		return err
	}
	targets, err := resolveTargets(a, *indexList, *hashList)
	if err != nil {
		a.Close()
		return err
	}
	if len(targets) == 0 {
		a.Close()
		return fmt.Errorf("no font entries detected; run refont scan and pass -index or -hash manually (bitmap font atlases are not detected yet, see the README)")
	}
	fmt.Printf("archive: %s (%d entries)\n", archivePath, len(a.Entries))
	fmt.Printf("font:    %s (%d bytes)\n", *fontPath, len(newFont))
	for _, i := range targets {
		e := &a.Entries[i]
		fmt.Printf("replace: entry %d, hash %016x, %d bytes\n", i, e.Hash, e.RawLength)
	}
	if *dryRun {
		a.Close()
		fmt.Println("dry run: nothing was written")
		return nil
	}
	dest := *outPath
	inPlace := dest == ""
	if inPlace {
		dest = archivePath
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".refont-tmp-")
	if err != nil {
		a.Close()
		return err
	}
	tmpName := tmp.Name()
	fail := func(e error) error {
		tmp.Close()
		os.Remove(tmpName)
		a.Close()
		return e
	}
	if err := rewrite(a, tmp, targets, newFont); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		a.Close()
		return err
	}
	if err := a.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if inPlace && *backup {
		bak := fmt.Sprintf("%s.refont-bak-%s", dest, time.Now().Format("20060102-150405"))
		if err := os.Rename(archivePath, bak); err != nil {
			os.Remove(tmpName)
			return err
		}
		fmt.Println("backup:  " + bak)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	fmt.Println("patched: " + dest)
	return nil
}

// rewrite copies every entry of a into w, replacing the entries listed in
// targets with newFont. Untouched entries are copied byte for byte with their
// original storage flags, so they never need to be decompressed or
// recompressed.
func rewrite(a *nvc.Archive, w io.WriteSeeker, targets []int, newFont []byte) error {
	writer, err := nvc.NewWriter(w, len(a.Entries))
	if err != nil {
		return err
	}
	picked := make(map[int]bool, len(targets))
	for _, i := range targets {
		picked[i] = true
	}
	for i := range a.Entries {
		e := &a.Entries[i]
		switch {
		case picked[i]:
			switch e.Flags {
			case nvc.FlagNone:
				err = writer.WriteRaw(newFont, uint32(len(newFont)), e.Hash, nvc.FlagNone)
			case nvc.FlagZlib:
				err = writer.WriteCompressed(bytes.NewReader(newFont), e.Hash)
			default:
				err = fmt.Errorf("entry %d is encrypted; refusing to replace it", i)
			}
		default:
			var stored []byte
			stored, err = a.StoredBytes(i)
			if err == nil {
				err = writer.WriteRaw(stored, e.RawLength, e.Hash, e.Flags)
			}
		}
		if err != nil {
			return err
		}
	}
	return writer.Finalize()
}

// resolveTargets returns the sorted entry indices to replace. If both lists
// are empty, every entry whose contents look like a font file is selected.
func resolveTargets(a *nvc.Archive, indexList, hashList string) ([]int, error) {
	picked := make(map[int]bool)
	if indexList != "" {
		for _, part := range strings.Split(indexList, ",") {
			part = strings.TrimSpace(part)
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= len(a.Entries) {
				return nil, fmt.Errorf("invalid entry index %q", part)
			}
			picked[n] = true
		}
	}
	if hashList != "" {
		for _, part := range strings.Split(hashList, ",") {
			part = strings.TrimSpace(part)
			v, err := strconv.ParseUint(part, 16, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid entry hash %q", part)
			}
			found := false
			for i := range a.Entries {
				if a.Entries[i].Hash == v {
					picked[i] = true
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("no entry with hash %016x", v)
			}
		}
	}
	out := make([]int, 0, len(picked))
	for i := range picked {
		out = append(out, i)
	}
	sort.Ints(out)
	if len(out) > 0 {
		return out, nil
	}
	for i := range a.Entries {
		if a.Entries[i].Flags == nvc.FlagEncrypted {
			continue
		}
		head, err := a.EntryPeek(i, 16)
		if err != nil {
			continue
		}
		if font.Detect(head) != "" {
			out = append(out, i)
		}
	}
	return out, nil
}

func cmdExtract(args []string) error {
	fs := flag.NewFlagSet("refont extract", flag.ContinueOnError)
	outDir := fs.String("out", "refont-extract", "output directory")
	pathlistPath := fs.String("pathlist", "", "file with original paths, one per line")
	fontsOnly := fs.Bool("fonts-only", false, "extract only detected font files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: refont extract ARCHIVE.nvc [flags]")
	}
	names, err := loadPathlist(*pathlistPath)
	if err != nil {
		return err
	}
	a, err := nvc.ParseFile(fs.Arg(0))
	if err != nil {
		return err
	}
	defer a.Close()
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	written := 0
	skipped := 0
	for i := range a.Entries {
		e := &a.Entries[i]
		name, known := names[e.Hash]
		if !known {
			name = fmt.Sprintf("%03d-%016x", i, e.Hash)
		}
		data, derr := a.EntryData(i)
		if derr != nil {
			fmt.Fprintf(os.Stderr, "skipping entry %d: %v\n", i, derr)
			skipped++
			continue
		}
		kind := font.Detect(data)
		if *fontsOnly && kind == "" {
			continue
		}
		full := filepath.Join(*outDir, name)
		if known {
			if !withinDir(*outDir, full) {
				full = filepath.Join(*outDir, fmt.Sprintf("%03d-%016x", i, e.Hash))
			}
		} else if kind != "" {
			full = full + "." + kind.Ext()
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return err
		}
		written++
	}
	fmt.Printf("extracted %d entries to %s (%d skipped)\n", written, *outDir, skipped)
	return nil
}

// withinDir reports whether path stays inside dir after cleaning.
func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
