package tui

func emptyCard(width int, height int) []string {
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		lines = append(lines, fitLine("", width))
	}
	return lines
}
