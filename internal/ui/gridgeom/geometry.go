// Package gridgeom contains pure grid calculations shared by the Gio renderer.
package gridgeom

import "strings"

const captionHeight = 76

func Geometry(width, minCard, gap, pad int) (cols, cardWidth, coverHeight, rowHeight int) {
	if width <= 0 {
		return 1, minCard, minCard * 4 / 3, minCard*4/3 + captionHeight + gap
	}
	cols = max(2, (width-pad+gap)/(minCard+gap))
	cardWidth = max(80, (width-pad-gap*(cols-1))/cols)
	coverHeight = cardWidth * 4 / 3
	rowHeight = coverHeight + captionHeight + gap
	return
}

func ViewportHeight(requested, width, total, minCard, gap, pad int) int {
	if total <= 0 {
		return requested
	}
	cols, _, _, rowH := Geometry(width, minCard, gap, pad)
	rows := (total + cols - 1) / cols
	contentH := rows*rowH - gap + pad
	return min(requested, max(rowH, contentH))
}

func VisibleRows(first, count, rows, pad int) (int, int) {
	if rows <= 0 {
		return 0, -1
	}
	start := max(0, first-pad)
	end := min(rows-1, first+max(count, 1)+pad)
	return start, end
}

func TitleInitials(title string) string {
	words := strings.Fields(title)
	var b strings.Builder
	for i := 0; i < len(words) && i < 2; i++ {
		runes := []rune(words[i])
		if len(runes) > 0 {
			b.WriteRune(runes[0])
		}
	}
	if b.Len() == 0 {
		return "?"
	}
	return strings.ToUpper(b.String())
}
