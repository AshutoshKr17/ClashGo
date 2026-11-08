package attack

import "gocv.io/x/gocv"

// lootCorners is the deploy corner for each quadrant of the base, indexed
// [row][col] with row 0 = top. The names match target_edge.
var lootCorners = [2][2]string{
	{"TopLeft", "TopRight"},
	{"BottomLeft", "BottomRight"},
}

// lootSideMinPixels is the smallest blob that counts as a loot side. Below it
// the frame has no readable storages (clouds, a zoomed-in wall) and the caller
// keeps its fallback edge.
const lootSideMinPixels = 200

// lootSide returns the corner whose quadrant holds the most gold-storage and
// elixir-storage pixels, and how many it held. The second result is 0 when the
// frame has nothing readable.
//
// Storages and collectors are the saturated yellow and magenta blobs. Grass,
// walls and the HUD are not: the sample starts below the resource bar
// (yTopMin) and stops above the troop bar, and the available-loot icons at the
// top-left are skipped so they cannot vote for TopLeft.
func lootSide(screen gocv.Mat) (string, int) {
	if screen.Empty() {
		return "", 0
	}
	w, h := screen.Cols(), screen.Rows()
	y0, y1 := yTopMin, int(float64(h)*0.80)
	if y1 <= y0+20 || w < 40 {
		return "", 0
	}

	hsv := gocv.NewMat()
	defer hsv.Close()
	gocv.CvtColor(screen, &hsv, gocv.ColorBGRToHSV)

	gold := gocv.NewMat()
	elixir := gocv.NewMat()
	defer gold.Close()
	defer elixir.Close()
	// OpenCV hue is 0..180. Measured on a zoomed-out 860x732 battle frame:
	// gold storages are a hard yellow, elixir storages a hard magenta. The
	// floor on saturation keeps out the beach, the dirt, and the timber,
	// which share the hue but not the paint.
	// Gold storages and mines sit at hue 12..40. Grass starts past 40.
	// Saturation 180 drops the beach and the dirt, which share the hue.
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(12, 180, 150, 0), gocv.NewScalar(40, 255, 255, 0), &gold)
	gocv.InRangeWithScalar(hsv, gocv.NewScalar(140, 110, 90, 0), gocv.NewScalar(179, 255, 255, 0), &elixir)

	counts := lootQuadrants(gold, elixir, y0, y1, w)
	best, bestN := "", 0
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			if counts[row][col] > bestN {
				bestN = counts[row][col]
				best = lootCorners[row][col]
			}
		}
	}
	if bestN < lootSideMinPixels {
		return "", 0
	}
	return best, bestN
}

func lootQuadrants(gold, elixir gocv.Mat, y0, y1, w int) [2][2]int {
	var counts [2][2]int
	h := gold.Rows()
	mx, my := w/2, (y0+y1)/2
	for y := y0; y < y1; y++ {
		row := 0
		if y >= my {
			row = 1
		}
		for x := 0; x < w; x++ {
			// The available-loot icons and the orange Next button are the
			// same colours as the storages. Both sit off the base.
			if x < 100 && y < 170 {
				continue
			}
			if x > w-230 && y > h-200 {
				continue
			}
			if gold.GetUCharAt(y, x) == 0 && elixir.GetUCharAt(y, x) == 0 {
				continue
			}
			col := 0
			if x >= mx {
				col = 1
			}
			counts[row][col]++
		}
	}
	return counts
}
