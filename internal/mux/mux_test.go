package mux

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeBox(f *os.File, size uint64, typ string, payload []byte) error {
	if size > 0xffffffff {
		return nil
	}
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(size))
	copy(hdr[4:8], typ)
	if _, err := f.Write(hdr[:]); err != nil {
		return err
	}
	_, err := f.Write(payload)
	return err
}

func writeExtendedBox(f *os.File, size uint64, typ string, payload []byte) error {
	var hdr [16]byte
	binary.BigEndian.PutUint32(hdr[:4], 1)
	copy(hdr[4:8], typ)
	binary.BigEndian.PutUint64(hdr[8:], size)
	if _, err := f.Write(hdr[:]); err != nil {
		return err
	}
	_, err := f.Write(payload)
	return err
}

func TestBoxPositionsExtendedSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extended.mp4")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := writeBox(f, 24, "ftyp", make([]byte, 16)); err != nil {
		t.Fatal(err)
	}
	if err := writeExtendedBox(f, 24, "mdat", make([]byte, 8)); err != nil {
		t.Fatal(err)
	}
	if err := writeBox(f, 8, "moov", nil); err != nil {
		t.Fatal(err)
	}
	moov, mdat, err := boxPositions(path)
	if err != nil {
		t.Fatal(err)
	}
	if moov <= mdat || mdat < 0 {
		t.Fatalf("got moov=%d mdat=%d, want moov after mdat", moov, mdat)
	}
}

func TestBoxPositionsZeroSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero.mp4")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := writeBox(f, 24, "ftyp", make([]byte, 16)); err != nil {
		t.Fatal(err)
	}
	if err := writeBox(f, 8, "moov", nil); err != nil {
		t.Fatal(err)
	}
	var hdr [8]byte
	copy(hdr[4:8], "mdat")
	if _, err := f.Write(hdr[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(make([]byte, 4)); err != nil {
		t.Fatal(err)
	}
	moov, mdat, err := boxPositions(path)
	if err != nil {
		t.Fatal(err)
	}
	if moov < 0 || mdat < 0 || moov >= mdat {
		t.Fatalf("got moov=%d mdat=%d, want moov before EOF-sized mdat", moov, mdat)
	}
}
