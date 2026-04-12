package reporting

import (
	"fmt"
	"math"
	"strings"
)

// RenderASCIIChart draws a simple ASCII line chart for an equity curve.
func RenderASCIIChart(values []float64, width int, height int) string {
	if len(values) == 0 {
		return "(no data)"
	}
	if width < 10 {
		width = 40
	}
	if height < 3 {
		height = 10
	}

	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	// Sample points to fit width
	step := float64(len(values)-1) / float64(width)
	if step < 1 {
		step = 1
	}

	points := make([]float64, 0, width)
	for i := 0; i < width && i < len(values); i++ {
		idx := int(float64(i) * float64(len(values)-1) / float64(width-1))
		if idx >= len(values) {
			idx = len(values) - 1
		}
		points = append(points, values[idx])
	}

	// Build grid
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, len(points))
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	for x, v := range points {
		y := int(math.Round(float64(height-1) * (maxVal - v) / (maxVal - minVal)))
		if y < 0 {
			y = 0
		}
		if y >= height {
			y = height - 1
		}
		grid[y][x] = '*'
	}

	var sb strings.Builder
	for i, row := range grid {
		label := ""
		if i == 0 {
			label = fmt.Sprintf("%.0f | ", maxVal)
		} else if i == height-1 {
			label = fmt.Sprintf("%.0f | ", minVal)
		} else {
			label = "    | "
		}
		sb.WriteString(label)
		sb.WriteString(string(row))
		sb.WriteString("\n")
	}
	sb.WriteString("    +")
	for range points {
		sb.WriteString("-")
	}
	sb.WriteString("\n")
	return sb.String()
}
