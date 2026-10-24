package ffmpeg

import (
	"bytes"
	"encoding/binary"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func box(boxType string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(out[:4], uint32(8+len(body)))
	copy(out[4:8], boxType)
	return append(out, body...)
}

var _ = Describe("isFragmentedMP4", func() {
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00"))

	DescribeTable("detects fragmentation",
		func(data []byte, expected bool) {
			frag, err := isFragmentedMP4(bytes.NewReader(data))
			Expect(err).ToNot(HaveOccurred())
			Expect(frag).To(Equal(expected))
		},
		Entry("DASH-style init + fragment", bytes.Join([][]byte{
			ftyp, box("moov", box("mvhd", make([]byte, 20)), box("mvex", box("trex", make([]byte, 24)))),
			box("moof", box("mfhd", make([]byte, 8))), box("mdat", make([]byte, 32)),
		}, nil), true),
		Entry("bare moof after ftyp", bytes.Join([][]byte{ftyp, box("moof"), box("mdat")}, nil), true),
		Entry("regular faststart MP4", bytes.Join([][]byte{
			ftyp, box("moov", box("mvhd", make([]byte, 20)), box("trak", make([]byte, 16))),
			box("mdat", make([]byte, 64)),
		}, nil), false),
		Entry("FLAC file", []byte("fLaC\x00\x00\x00\x22"+string(make([]byte, 64))), false),
		Entry("MP3 with ID3 tag", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"+string(make([]byte, 64))), false),
		Entry("empty input", []byte{}, false),
	)
})
