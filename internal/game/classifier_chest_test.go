package game

import (
	"image"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"gocv.io/x/gocv"
)

// newChestClassifier builds a Classifier with identity calibration so
// reference (860x732) pixel coordinates map 1:1 onto the synthetic frame.
func newChestClassifier(t *testing.T) *Classifier {
	t.Helper()
	cal := &Calibration{PhysicalW: RefWidth, PhysicalH: RefHeight, ScaleX: 1, ScaleY: 1}
	return NewClassifier(cal, DefaultClassifierConfig(), zerolog.Nop())
}

// loadHammerTemplate loads the hammer template from assets/templates
// onto the classifier. Must be called before ClassifyState in any test
// that expects the template-only ChestReward rule to fire.
func loadHammerTemplate(t *testing.T, c *Classifier) {
	t.Helper()
	templateDir := "../../assets/templates"
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		templateDir = "assets/templates"
	}
	ts, err := NewTemplateStore(templateDir)
	if err != nil {
		t.Fatalf("NewTemplateStore(%s): %v", templateDir, err)
	}
	if err := ts.LoadTemplates(); err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	t.Cleanup(ts.Close)
	c.SetTemplates(ts)
}

// TestClassify_ChestDetectedOnChestFrame asserts that StateChestReward
// fires when the vault room signature and hammer template are present.
func TestClassify_ChestDetectedOnChestFrame(t *testing.T) {
	c := newChestClassifier(t)
	loadHammerTemplate(t, c)

	m := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer m.Close()
	// Fill with a dark neutral background so the hammer stands out.
	m.SetTo(gocv.NewScalar(30, 30, 30, 0))

	// Paint the full vault-room signature. All four probes are required
	// (MinPass 4); the dark background alone only covers the two corner
	// probes, so the arch beam and the wood floor must be painted.
	setRGB(m, 430, 122, 0xF0, 0xEB, 0xCD)
	setRGB(m, 40, 50, 0x36, 0x2A, 0x1F)
	setRGB(m, 820, 50, 0x32, 0x21, 0x11)
	setRGB(m, 760, 690, 0x60, 0x2C, 0x18)

	hammer := gocv.IMRead("../../assets/templates/hammer.png", gocv.IMReadColor)
	defer hammer.Close()
	if hammer.Empty() {
		t.Fatal("hammer.png could not be loaded")
	}
	// Paste the hammer into the frame at the location where it appears
	// on a real chest screen (~center of the "TAP TO OPEN" band).
	r := image.Rect(344, 506, 504, 547)
	roi := m.Region(r)
	hammer.CopyTo(&roi)
	roi.Close()

	state, _ := c.ClassifyState(m)
	if state != StateChestReward {
		t.Fatalf("expected StateChestReward, got %s", state)
	}
}

// TestClassify_RealChestFrames tests that live-captured real event frames
// (closed, breaking, reward) all correctly classify as StateChestReward.
func TestClassify_RealChestFrames(t *testing.T) {
	cal := NewCalibration(1280, 720)
	c := NewClassifier(cal, DefaultClassifierConfig(), zerolog.Nop())

	fixtures := []string{
		"testdata/chest_test_frames/chest_closed.png",
		"testdata/chest_test_frames/chest_hammer.png",
		"testdata/chest_test_frames/chest_breaking.png",
		"testdata/chest_test_frames/chest_reward.png",
	}

	for _, path := range fixtures {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Skipf("fixture %s not present on disk; skipping", path)
		}
		m := gocv.IMRead(path, gocv.IMReadColor)
		if m.Empty() {
			t.Fatalf("failed to read %s", path)
		}
		state, score := c.ClassifyState(m)
		m.Close()
		if state != StateChestReward {
			t.Fatalf("%s: expected StateChestReward, got %s (score=%d)", path, state, score)
		}
	}
}

// TestClassify_NoFalseChestOnBattleFrame guards the MinPass 4 requirement.
// A live 860x732 battle frame passed the two dark-corner probes and the
// wood-floor probe but missed the bright-arch probe, and at MinPass 3 that
// was enough to classify the battle as a chest. This frame reproduces
// exactly those three hits with no arch beam and no hammer template.
func TestClassify_NoFalseChestOnBattleFrame(t *testing.T) {
	c := newChestClassifier(t)

	m := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer m.Close()
	m.SetTo(gocv.NewScalar(30, 30, 30, 0))

	setRGB(m, 40, 50, 0x36, 0x2A, 0x1F)
	setRGB(m, 820, 50, 0x32, 0x21, 0x11)
	setRGB(m, 760, 690, 0x60, 0x2C, 0x18)

	state, _ := c.ClassifyState(m)
	if state == StateChestReward {
		t.Fatalf("chest rule fired on a frame missing the arch beam (state=%s)", state)
	}
}

// TestClassify_Searching fires only when the matchmaking screen's Return
// Home button is present. A plain white frame is what SearchMap already
// matches; this screen must not be that.
func TestClassify_Searching(t *testing.T) {
	c := newChestClassifier(t)

	white := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer white.Close()
	white.SetTo(gocv.NewScalar(255, 255, 255, 0))
	if state, _ := c.ClassifyState(white); state == StateSearching {
		t.Fatal("white frame classified as Searching")
	}

	m := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer m.Close()
	m.SetTo(gocv.NewScalar(255, 255, 255, 0))
	setRGB(m, 48, 632, 0xD3, 0xC6, 0x96)
	setRGB(m, 60, 640, 0xBA, 0x48, 0x2A)

	state, _ := c.ClassifyState(m)
	if state != StateSearching {
		t.Fatalf("expected StateSearching, got %s", state)
	}
}

// TestClassify_NoConnectionLostOnTwoLightProbes pins MinPass 3: a battle
// frame whose light-blue walls hit two glyph probes must not read as the
// disconnect dialog.
func TestClassify_NoConnectionLostOnTwoLightProbes(t *testing.T) {
	c := newChestClassifier(t)

	m := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer m.Close()
	m.SetTo(gocv.NewScalar(60, 110, 70, 0))
	setRGB(m, 300, 478, 0xCB, 0xE6, 0xFF)
	setRGB(m, 431, 581, 0xCB, 0xE6, 0xFF)

	if state, _ := c.ClassifyState(m); state == StateConnectionLost {
		t.Fatal("ConnectionLost fired on two light probes with no dark panel")
	}
}

// TestClassify_NoSearchMapOnSeaAlone pins SearchMap's MinPass 2: a frame
// whose only hit is the dark bottom-left probe (village sea) must not read
// as search clouds.
func TestClassify_NoSearchMapOnSeaAlone(t *testing.T) {
	c := newChestClassifier(t)

	m := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer m.Close()
	m.SetTo(gocv.NewScalar(60, 110, 70, 0))
	setRGB(m, 38, 603, 0x0B, 0x2B, 0x3E)

	if state, _ := c.ClassifyState(m); state == StateSearchMap {
		t.Fatal("SearchMap fired on the dark corner probe alone")
	}
}

// TestClassify_SearchingFrames classifies captured frames listed in
// CLASHGO_SEARCH_FRAMES (comma-separated paths). Skips otherwise, so CI
// never needs the pngs (those are gitignored).
func TestClassify_SearchingFrames(t *testing.T) {
	list := os.Getenv("CLASHGO_SEARCH_FRAMES")
	if list == "" {
		t.Skip("CLASHGO_SEARCH_FRAMES not set")
	}
	c := newChestClassifier(t)
	for _, path := range strings.Split(list, ",") {
		m := gocv.IMRead(path, gocv.IMReadColor)
		if m.Empty() {
			t.Fatalf("could not read %s", path)
		}
		state, score := c.ClassifyState(m)
		m.Close()
		if state != StateSearching {
			t.Fatalf("%s: expected StateSearching, got %s (score=%d)", path, state, score)
		}
	}
}

// TestClassify_NoFalseChestOnMainVillage asserts the chest rule does NOT
// fire on a MainVillage-looking frame — even with the hammer template
// loaded, it must not match on a screen that lacks the TAP TO OPEN
// prompt. This guards against the template matching latching onto random
// pixel patterns in normal gameplay.
func TestClassify_NoFalseChestOnMainVillage(t *testing.T) {
	c := newChestClassifier(t)
	loadHammerTemplate(t, c)

	m := gocv.NewMatWithSize(RefHeight, RefWidth, gocv.MatTypeCV8UC3)
	defer m.Close()
	// Bright grass bottom (no dark band).
	m.SetTo(gocv.NewScalar(0x2A, 0x5F, 0x3A, 0))
	// Neutral center (village ground, not a warm chest glow).
	// Some MainVillage HUD pixels so the frame isn't "unknown".
	setRGB(m, 830, 35, 0xB2, 0x90, 0x0F)
	setRGB(m, 830, 95, 0x54, 0x19, 0x59)

	state, _ := c.ClassifyState(m)
	if state == StateChestReward {
		t.Fatalf("chest rule falsely fired on a MainVillage frame (state=%s)", state)
	}
}

// setRGB writes an RGB triple at (x,y) of a 3-channel BGR Mat.
func setRGB(m gocv.Mat, x, y int, r, g, b uint8) {
	m.SetUCharAt(y, x*3, b)
	m.SetUCharAt(y, x*3+1, g)
	m.SetUCharAt(y, x*3+2, r)
}
