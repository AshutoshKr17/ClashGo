package attack

import (
	"image"
	"os"
	"testing"

	"gocv.io/x/gocv"
)

func TestLootSidePicksTheQuadrantWithTheStorages(t *testing.T) {
	img := gocv.NewMatWithSize(732, 860, gocv.MatTypeCV8UC3)
	defer img.Close()
	img.SetTo(gocv.NewScalar(40, 120, 40, 0)) // grass, BGR

	// A gold-storage blob in the bottom-left of the field.
	blob := img.Region(image.Rect(60, 460, 220, 580))
	blob.SetTo(gocv.NewScalar(40, 200, 235, 0))
	blob.Close()

	side, n := lootSide(img)
	if side != "BottomLeft" || n < lootSideMinPixels {
		t.Fatalf("lootSide = %q (%d pixels), want BottomLeft", side, n)
	}
}

func TestLootSideOnCapturedBattle(t *testing.T) {
	path := os.Getenv("CLASHGO_LOOT_FRAME")
	if path == "" {
		t.Skip()
	}
	img := gocv.IMRead(path, gocv.IMReadColor)
	if img.Empty() {
		t.Fatal("empty frame")
	}
	defer img.Close()
	side, n := lootSide(img)
	t.Logf("loot side %s (%d pixels)", side, n)
}

func TestLootSideIgnoresTheResourceBar(t *testing.T) {
	img := gocv.NewMatWithSize(732, 860, gocv.MatTypeCV8UC3)
	defer img.Close()
	img.SetTo(gocv.NewScalar(40, 120, 40, 0))

	// Gold painted only where the available-loot icon sits. It must not
	// count as a side.
	icon := img.Region(image.Rect(20, 120, 70, 155))
	icon.SetTo(gocv.NewScalar(40, 200, 235, 0))
	icon.Close()

	if side, n := lootSide(img); side != "" || n != 0 {
		t.Fatalf("lootSide = %q (%d), want no side from the HUD icon alone", side, n)
	}
}
