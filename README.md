# refont

Replace the fonts inside game archives with fonts of your own choosing.

refont exists for two moments every immersion player knows: the game is
beautiful but the default font is ugly, or you play in Japanese and the
shipped font renders kana as tofu boxes. refont reads a game asset archive,
finds the font files inside, swaps in your font, and writes the archive back,
keeping a timestamped backup of the original. It never touches game logic,
only glyphs.

## Supported formats

| Format | Games | Status |
| ------ | ----- | ------ |
| Nova Engine .nvc | Jupiter Hell (Jupiter Hell Classic untested) | v0.1 |

The archive handling lives in `internal/nvc` and font detection in
`internal/font`, so more formats can be added as small drivers.

## Install

Requires Go 1.22 or newer:

    go install github.com/Breacher1337/refont@latest

or build from a checkout:

    git clone https://github.com/Breacher1337/refont
    cd refont
    go build

Tagged releases also produce prebuilt binaries through CI.

## Quickstart: Jupiter Hell

1. Open your install folder (Steam: right click the game, then Manage, then
   Browse local files). The archives are `core.nvc` and `assets.nvc`.
2. See what is inside:

       refont scan core.nvc

   Each line is one entry. The kind column shows ttf or otf for font files.
3. Swap every detected font with your font (a backup is created automatically):

       refont swap core.nvc -font NotoSansJP-Regular.ttf

4. Start the game. If anything looks wrong, restore the `.refont-bak-*` file
   next to the archive.

Use a font you have a license for. Noto Sans JP and BIZ UDGothic are popular
free choices for Japanese; Zen Maru Gothic pairs well with lighter UI styles.

### Replacing one specific font

Games often ship several fonts for different UI roles. Target one entry at a
time using the idx column from `refont scan`:

    refont swap core.nvc -font ZenMaruGothic-Medium.ttf -index 42 -dry-run

`-dry-run` previews, `-out patched.nvc` writes elsewhere, `-hash` targets
by hash, `-backup=false` skips the backup.

### Showing real entry names

.nvc entries are keyed by the FNV-1a hash of their original path, not the
path itself. To see names, generate a pathlist with jhmod
(`jhmod pathlist scan`) and pass it to any command with
`-pathlist paths.txt`.

## Commands

| Command | What it does |
| ------- | ------------ |
| `refont scan ARCHIVE.nvc` | list entries, detect fonts, show hashes |
| `refont swap ARCHIVE.nvc -font F.ttf` | replace font entries in the archive |
| `refont extract ARCHIVE.nvc` | dump archive contents for inspection |

## Honest limitations

- Bitmap font atlases (pre-rendered glyph sheets, no font file) are not
  detected yet. If `refont scan` finds no fonts, the game may use an atlas:
  run `refont extract`, look around, and open an issue with what you find so
  the format can be supported.
- Encrypted entries are copied untouched and are never replaced.
- Untouched entries are copied byte for byte, so a patched archive differs
  from the original only in the entries you replaced.
- refont does not add a font engine: the game must already load fonts as font
  files for a swap to change anything. It also does not translate any text.
- Test on your own copy of the game and keep the backups. Do not distribute
  patched archives; share the tool and your font choice, not game assets.

## Roadmap

- Bitmap font atlas detection and regeneration (BMFont style formats)
- Unity TextMeshPro font asset bundles, as a companion to plugins like TMPJaFix
- More engines: Godot PCK and common pak formats
- Per-engine notes on Japanese line breaking (kinsoku) pitfalls

## Credits

- The .nvc format was first documented and implemented in jhmod by winny and
  sector-f (BSD-3-Clause). refont reimplements the format from that work:
  https://github.com/sector-f/jhmod
- Jupiter Hell modding basics: https://github.com/chaosforgeorg/jh-mods/wiki
- Prior art for Japanese font fixes in Unity games: TMPJaFix by dewfalse:
  https://github.com/dewfalse/TMPJaFix

## License

MIT, see LICENSE. The nvc package is an independent reimplementation of a
format first published under BSD-3-Clause by the jhmod authors.
