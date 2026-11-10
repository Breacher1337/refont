// Package font detects font file formats from their leading magic bytes.
package font

// Kind names a font file format that can be detected by magic bytes.
type Kind string

const (
	KindTrueType   Kind = "ttf"
	KindOpenType   Kind = "otf"
	KindCollection Kind = "ttc"
	KindWOFF       Kind = "woff"
	KindWOFF2      Kind = "woff2"
)

var kindByMagic = map[[4]byte]Kind{
	{0x00, 0x01, 0x00, 0x00}: KindTrueType,   // TrueType
	{0x4f, 0x54, 0x54, 0x4f}: KindOpenType,   // OTTO
	{0x74, 0x74, 0x63, 0x66}: KindCollection, // ttcf
	{0x77, 0x4f, 0x46, 0x46}: KindWOFF,       // wOFF
	{0x77, 0x4f, 0x46, 0x32}: KindWOFF2,      // wOF2
	{0x74, 0x72, 0x75, 0x65}: KindTrueType,   // true, Apple variant
}

// Detect returns the font kind implied by the first bytes of a file, or the
// empty kind if the bytes do not look like a supported font format.
func Detect(head []byte) Kind {
	if len(head) < 4 {
		return ""
	}
	var k [4]byte
	copy(k[:], head[:4])
	return kindByMagic[k]
}

// Ext returns a conventional file extension for the kind, without a dot.
func (k Kind) Ext() string {
	if k == "" {
		return ""
	}
	return string(k)
}
