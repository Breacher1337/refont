package font

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		head []byte
		want Kind
	}{
		{[]byte{0x00, 0x01, 0x00, 0x00, 0x47}, KindTrueType},
		{[]byte{0x4f, 0x54, 0x54, 0x4f}, KindOpenType},
		{[]byte{0x74, 0x74, 0x63, 0x66}, KindCollection},
		{[]byte{0x77, 0x4f, 0x46, 0x46}, KindWOFF},
		{[]byte{0x77, 0x4f, 0x46, 0x32}, KindWOFF2},
		{[]byte{0x74, 0x72, 0x75, 0x65}, KindTrueType},
		{[]byte{0x4d, 0x5a, 0x90, 0x00}, ""},
		{[]byte{0x00, 0x01, 0x00}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := Detect(c.head); got != c.want {
			t.Errorf("Detect(% x) = %q, want %q", c.head, got, c.want)
		}
	}
}
