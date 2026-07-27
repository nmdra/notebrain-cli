package pdf2md

import "strings"

// TextRect represents a positioned text run extracted from a PDF page.
type TextRect struct {
	Text       string
	FontSize   float64 // RenderedSize from PDFium (points)
	FontWeight int     // 400=normal, 700=bold
	FontName   string
	FontFlags  int     // PDFium font flags bitmask
	Left       float64 // PDF points from left edge
	Top        float64 // PDF points from top edge
	Right      float64
	Bottom     float64
	PageNum    int // 1-indexed
}

// IsBold returns true if the font weight is bold or the force-bold flag is set.
func (r *TextRect) IsBold() bool {
	// Flag bit 18 (262144) is ForceBold
	return r.FontWeight >= 700 || r.FontFlags&(1<<18) != 0
}

// IsItalic returns true if the italic font flag is set.
func (r *TextRect) IsItalic() bool {
	// Flag bit 6 (64) is Italic
	return r.FontFlags&(1<<6) != 0
}

// IsFixedPitch returns true if the font is monospaced (code font).
func (r *TextRect) IsFixedPitch() bool {
	// Flag bit 0 (1) is FixedPitch
	if r.FontFlags&1 != 0 {
		return true
	}
	// Fallback to checking font name
	lowerName := strings.ToLower(r.FontName)
	return strings.Contains(lowerName, "mono") ||
		strings.Contains(lowerName, "nimbusmon") ||
		strings.Contains(lowerName, "courier") ||
		strings.Contains(lowerName, "consolas") ||
		strings.Contains(lowerName, "typewriter") ||
		strings.Contains(lowerName, "menlo") ||
		strings.Contains(lowerName, "monaco")
}

// Height returns the vertical height of this text rect in points.
func (r *TextRect) Height() float64 {
	return r.Bottom - r.Top
}

// DocumentStats holds document-wide font/layout statistics used by heuristics.
type DocumentStats struct {
	BodyFontSize  float64 // most common font size (mode)
	BodyFontName  string  // most common font name
	MedianLineGap float64 // median vertical gap between consecutive lines
	PageWidth     float64 // page width in points (optional)
	PageHeight    float64 // page height in points (optional)
}

// Line represents a single visual line: one or more TextRects at the same Y position.
type Line struct {
	Rects        []TextRect
	Top          float64 // Y position (top edge of the line)
	Bottom       float64
	PageNum      int
	HeadingLevel int // 0 if not a heading, 1-6 for H1-H6
}

// FullText returns the concatenated text of all rects in the line, joined by spaces.
func (l *Line) FullText() string {
	var texts []string
	for _, r := range l.Rects {
		texts = append(texts, strings.TrimSpace(r.Text))
	}
	return strings.Join(texts, " ")
}

// MaxFontSize returns the largest font size among the line's rects.
func (l *Line) MaxFontSize() float64 {
	maxSize := 0.0
	for _, r := range l.Rects {
		if r.FontSize > maxSize {
			maxSize = r.FontSize
		}
	}
	return maxSize
}

// IsBold returns true if the majority of text in the line is bold.
func (l *Line) IsBold() bool {
	if len(l.Rects) == 0 {
		return false
	}
	boldRunes := 0
	totalRunes := 0
	for _, r := range l.Rects {
		runes := len([]rune(r.Text))
		totalRunes += runes
		if r.IsBold() {
			boldRunes += runes
		}
	}
	return totalRunes > 0 && float64(boldRunes)/float64(totalRunes) > 0.5
}

// IsCode returns true if the majority of text in the line is fixed-pitch.
func (l *Line) IsCode() bool {
	if len(l.Rects) == 0 {
		return false
	}
	codeRunes := 0
	totalRunes := 0
	for _, r := range l.Rects {
		runes := len([]rune(r.Text))
		totalRunes += runes
		if r.IsFixedPitch() {
			codeRunes += runes
		}
	}
	return totalRunes > 0 && float64(codeRunes)/float64(totalRunes) > 0.5
}
