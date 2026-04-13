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

	minVal, maxVal := minMax(values)
	if maxVal == minVal {
		maxVal = minVal + 1
	}

	points := samplePoints(values, width)
	grid := buildASCIIGrid(points, height, minVal, maxVal)
	return renderGrid(grid, minVal, maxVal)
}

func minMax(values []float64) (float64, float64) {
	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	return minVal, maxVal
}

func samplePoints(values []float64, width int) []float64 {
	points := make([]float64, 0, width)
	for i := 0; i < width && i < len(values); i++ {
		idx := int(float64(i) * float64(len(values)-1) / float64(width-1))
		if idx >= len(values) {
			idx = len(values) - 1
		}
		points = append(points, values[idx])
	}
	return points
}

func buildASCIIGrid(points []float64, height int, minVal, maxVal float64) [][]rune {
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
	return grid
}

func renderGrid(grid [][]rune, minVal, maxVal float64) string {
	height := len(grid)
	var sb strings.Builder
	for i, row := range grid {
		label := ""
		switch i {
		case 0:
			label = fmt.Sprintf("%.0f | ", maxVal)
		case height - 1:
			label = fmt.Sprintf("%.0f | ", minVal)
		default:
			label = "    | "
		}
		sb.WriteString(label)
		sb.WriteString(string(row))
		sb.WriteString("\n")
	}
	sb.WriteString("    +")
	for range grid[0] {
		sb.WriteString("-")
	}
	sb.WriteString("\n")
	return sb.String()
}
