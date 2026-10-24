package main

// Reading single packages straight out of a mod's container (.utoc + .ucas), without retoc. Only
// containers that are neither encrypted nor compressed with Oodle can be read this way; retoc writes
// mods uncompressed, so most mods are. Used to see what other mods put in the skin list.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf16"
)

type ioBlock struct {
	offset         int64
	compSize, size int
	method         int
}

type ZenContainer struct {
	ucas      string
	ids       [][12]byte
	offs      []int64
	lens      []int64
	blockSize int64
	blocks    []ioBlock
	methods   []string
	names     map[int]string
}

func be40(b []byte) int64 {
	return int64(b[0])<<32 | int64(b[1])<<24 | int64(b[2])<<16 | int64(b[3])<<8 | int64(b[4])
}

func OpenZenContainer(utoc string) (*ZenContainer, error) {
	st, err := os.Stat(utoc)
	if err != nil {
		return nil, err
	}
	if st.Size() > maxModTOC {
		return nil, errors.New("the table of contents is too large for a mod")
	}
	b, err := os.ReadFile(utoc)
	if err != nil {
		return nil, err
	}
	info, err := parseTOC(b)
	if err != nil {
		return nil, err
	}
	if info.Encrypted {
		return nil, errors.New("the container is encrypted")
	}
	u32 := func(o int) int { return int(binary.LittleEndian.Uint32(b[o:])) }
	headerSize, entries, nblocks := u32(20), u32(24), u32(28)
	methodCount, methodLen := u32(36), u32(40)
	c := &ZenContainer{ucas: strings.TrimSuffix(utoc, ".utoc") + ".ucas", ids: info.ChunkIDs, names: info.Names, blockSize: int64(u32(44))}
	seeds, noHash := u32(84), u32(96)
	o := headerSize + entries*12
	for i := 0; i < entries; i++ {
		e := b[o+i*10:]
		c.offs = append(c.offs, be40(e))
		c.lens = append(c.lens, be40(e[5:]))
	}
	o += entries*10 + seeds*4 + noHash*4
	for i := 0; i < nblocks; i++ {
		e := b[o+i*12:]
		off := int64(binary.LittleEndian.Uint64(append(append([]byte(nil), e[:5]...), 0, 0, 0)))
		cs := int(e[5]) | int(e[6])<<8 | int(e[7])<<16
		us := int(e[8]) | int(e[9])<<8 | int(e[10])<<16
		c.blocks = append(c.blocks, ioBlock{offset: off, compSize: cs, size: us, method: int(e[11])})
	}
	o += nblocks * 12
	for i := 0; i < methodCount; i++ {
		c.methods = append(c.methods, strings.TrimRight(string(b[o+i*methodLen:o+(i+1)*methodLen]), "\x00"))
	}
	if c.blockSize <= 0 {
		return nil, errors.New("unexpected container layout")
	}
	return c, nil
}

// FindPackage gives the chunk number of the package whose path ends with suffix (for example
// "Datatables/Information/DT_Skin"), or -1.
func (c *ZenContainer) FindPackage(suffix string) int {
	for i, id := range c.ids {
		if id[11]&0x3f != 1 { // export bundle data: the package itself
			continue
		}
		n := c.names[i]
		n = strings.TrimSuffix(strings.TrimSuffix(n, ".uasset"), ".umap")
		if strings.HasSuffix(strings.ToLower(n), strings.ToLower(suffix)) {
			return i
		}
	}
	return -1
}

// ReadChunk gives a chunk's bytes.
func (c *ZenContainer) ReadChunk(i int) ([]byte, error) {
	off, n := c.offs[i], c.lens[i]
	if n > 256<<20 {
		return nil, errors.New("this part of the container is too large to read here")
	}
	f, err := os.Open(c.ucas)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make([]byte, 0, n)
	first := off / c.blockSize
	skip := off % c.blockSize
	for bi := first; int64(len(out)) < n; bi++ {
		if bi >= int64(len(c.blocks)) {
			return nil, errors.New("the container is cut short")
		}
		blk := c.blocks[bi]
		raw := make([]byte, (blk.compSize+15)&^15)
		if _, err := f.ReadAt(raw, blk.offset); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		raw = raw[:blk.compSize]
		var data []byte
		method := ""
		if blk.method > 0 && blk.method <= len(c.methods) {
			method = strings.ToLower(c.methods[blk.method-1])
		}
		switch {
		case blk.method == 0:
			data = raw
		case method == "zlib":
			zr, err := zlib.NewReader(bytes.NewReader(raw))
			if err != nil {
				return nil, err
			}
			data, err = io.ReadAll(zr)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("the container is compressed with %s, which this program cannot read", c.methods[blk.method-1])
		}
		if len(data) < blk.size {
			return nil, errors.New("a block of the container is cut short")
		}
		data = data[:blk.size]
		if skip > 0 {
			data = data[skip:]
			skip = 0
		}
		need := n - int64(len(out))
		if int64(len(data)) > need {
			data = data[:need]
		}
		out = append(out, data...)
	}
	return out, nil
}

// ZenPackage is the part of a zen package this program reads: its names and its export data.
type ZenPackage struct {
	Name       string
	Names      []string
	ExportData []byte // every export's data, one after the other
}

// ParseZenPackage reads a UE 5.4 zen package (container header version NoExportInfo).
func ParseZenPackage(b []byte) (zp *ZenPackage, err error) {
	defer func() {
		if r := recover(); r != nil {
			zp, err = nil, fmt.Errorf("unexpected package layout (%v)", r)
		}
	}()
	if len(b) < 64 {
		return nil, errors.New("package too small")
	}
	hasVersion := binary.LittleEndian.Uint32(b[0:])
	headerSize := int(binary.LittleEndian.Uint32(b[4:]))
	nameIdx := int(binary.LittleEndian.Uint32(b[8:]) & 0x3FFFFFFF)
	nameNum := int32(binary.LittleEndian.Uint32(b[12:]))
	if hasVersion != 0 {
		return nil, errors.New("packages with version numbers are not handled")
	}
	if headerSize <= 52 || headerSize > len(b) {
		return nil, errors.New("bad package header size")
	}
	o := 52
	num := int(binary.LittleEndian.Uint32(b[o:]))
	o += 4
	if num < 0 || num > 1_000_000 {
		return nil, errors.New("bad name count")
	}
	zp = &ZenPackage{}
	if num > 0 {
		o += 4 + 8   // string bytes, hash version
		o += num * 8 // hashes
		lens := make([]int, num)
		wide := make([]bool, num)
		for i := 0; i < num; i++ {
			l := int16(binary.BigEndian.Uint16(b[o+2*i:]))
			if l < 0 {
				wide[i] = true
				lens[i] = int(int32(l) - int32(-32768))
			} else {
				lens[i] = int(l)
			}
		}
		o += num * 2
		for i := 0; i < num; i++ {
			if wide[i] {
				u := make([]uint16, lens[i])
				for k := range u {
					u[k] = binary.LittleEndian.Uint16(b[o+2*k:])
				}
				o += 2 * lens[i]
				zp.Names = append(zp.Names, string(utf16.Decode(u)))
			} else {
				zp.Names = append(zp.Names, string(b[o:o+lens[i]]))
				o += lens[i]
			}
		}
	}
	if nameIdx < len(zp.Names) {
		zp.Name = joinFName(zp.Names[nameIdx], nameNum)
	}
	// the export map says how long each export's data is (the data may be followed by padding)
	exportMap := int(int32(binary.LittleEndian.Uint32(b[32:])))
	bundleEntries := int(int32(binary.LittleEndian.Uint32(b[36:])))
	n := (bundleEntries - exportMap) / 72
	if exportMap <= 0 || n <= 0 || exportMap+n*72 > headerSize {
		return nil, errors.New("bad export map")
	}
	total := 0
	for i := 0; i < n; i++ {
		total += int(binary.LittleEndian.Uint64(b[exportMap+72*i+8:]))
	}
	if headerSize+total > len(b) {
		return nil, errors.New("the exports are cut short")
	}
	zp.ExportData = b[headerSize : headerSize+total]
	return zp, nil
}

// readZenSkinTables reads the skin list and unlock list a container holds (either may be nil).
func readZenSkinTables(utoc string, sc *Schema) (*SkinTable, *UnlockTable, error) {
	c, err := OpenZenContainer(utoc)
	if err != nil {
		return nil, nil, err
	}
	var st *SkinTable
	var ut *UnlockTable
	if i := c.FindPackage("Datatables/Information/DT_Skin"); i >= 0 {
		data, err := c.ReadChunk(i)
		if err != nil {
			return nil, nil, err
		}
		zp, err := ParseZenPackage(data)
		if err != nil {
			return nil, nil, err
		}
		st, err = readSkinTableData(zp.Names, zp.ExportData, sc)
		if err != nil {
			return nil, nil, err
		}
	}
	if i := c.FindPackage("Unlock/DA_Unlock_Skin"); i >= 0 {
		data, err := c.ReadChunk(i)
		if err != nil {
			return nil, nil, err
		}
		zp, err := ParseZenPackage(data)
		if err != nil {
			return nil, nil, err
		}
		ut, err = readUnlockTableData(zp.Names, zp.ExportData, sc)
		if err != nil {
			return nil, nil, err
		}
	}
	return st, ut, nil
}
