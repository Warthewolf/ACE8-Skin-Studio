package main

// Aircraft whose skins are ordinary textures.
//
// The player aircraft keep each skin texture as one streamed UDIM texture (vt.go). Some aircraft,
// such as the Ladon, keep theirs as ordinary textures instead: one square picture each (4096 x 4096),
// with its smaller copies, the large ones in the .ubulk and the small ones inside the .uexp (tex2d.go).
// For Extract and Repack such a texture is a skin texture with a single square: VTex carries it
// (plain), so splitting, matching pictures and composing work the same; only reading and writing the
// pixels go through Tex2D.

import (
	"errors"
	"os"
)

// plainVTex describes an ordinary texture as a skin texture of one square.
func plainVTex(tx *Tex2D, uexp []byte) *VTex {
	block, _, _ := texBlock(tx.Format)
	return &VTex{Format: tx.Format, BlockBytes: block, BlocksW: 1, BlocksH: 1, Mips: len(tx.Mips), Width: tx.W(), Height: tx.H(),
		SRGB: tx.SRGB, blockMips: []int{0}, chunksResolved: true, plain: tx, plainExp: uexp}
}

// parseAnyTex reads a skin texture's description: streamed (UDIM) or, when ua is given, ordinary.
func parseAnyTex(ua, uexp []byte) (*VTex, error) {
	t, err := ParseVTex(uexp)
	if err == nil || !errors.Is(err, errNotStreamed) || ua == nil {
		return t, err
	}
	p, perr := ParsePackage(ua, uexp)
	if perr != nil {
		return nil, perr
	}
	tx, terr := ParseTex2D(p)
	if terr != nil {
		return nil, terr
	}
	return plainVTex(tx, uexp), nil
}

// readTexDesc reads only what is needed to describe a raw texture (not its pixels).
func readTexDesc(f TexFiles) (*VTex, error) {
	uexp, err := os.ReadFile(f.UExp)
	if err != nil {
		return nil, err
	}
	ua, _ := os.ReadFile(f.UAsset)
	return parseAnyTex(ua, uexp)
}

// IsPlain tells whether this is an ordinary texture (one square, no tiles).
func (t *VTex) IsPlain() bool { return t.plain != nil }
