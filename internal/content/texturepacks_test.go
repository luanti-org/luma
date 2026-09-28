package content

import (
	"path/filepath"
	"testing"
)

func TestScanTextures(t *testing.T) {
	dir := t.TempDir()

	// well-formed texturepack
	writeFile(t, filepath.Join(dir, "goodtexturepack", "texture_pack.conf"),
		"name = goodtexturepack\ntitle = Good Texture Pack\nauthor = someone\ndescription = A texture pack\nrelease = 14\n")

	// no texture_pack.conf at all
	writeFile(t, filepath.Join(dir, "bare_texturepack", "override.txt"), "# nothing")

	texturepacks, err := ScanTextures(dir)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]Texturepack)
	for _, tp := range texturepacks {
		byName[tp.Dir] = tp
	}

	if len(texturepacks) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(texturepacks), texturepacks)
	}

	good := byName["goodtexturepack"]
	if !good.ConfOK || good.Title != "Good Texture Pack" || good.Author != "someone" || good.Release != 14 {
		t.Errorf("goodtexturepack: unexpected result %+v", good)
	}

	bare := byName["bare_texturepack"]
	if bare.ConfOK || bare.Name != "bare_texturepack" {
		t.Errorf("bare_texturepack: expected fallback to folder name, got %+v", bare)
	}
}
