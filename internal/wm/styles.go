package wm

// Styles used when painting. The terminal layer maps each to real colors
// (with an 8-color fallback for the Linux console).
const (
	StyleDesktop   StyleID = iota
	StyleBar               // top bar and taskbar
	StyleBarActive         // highlighted item in a bar (focused window, open menu)
	StyleFrame
	StyleFrameFocused
	StyleTitle
	StyleTitleFocused
	StyleButton // title-bar buttons [_][^][x]
	StyleBody
	StyleBodyDim
	StyleMenu
	StyleMenuSelected
	StyleMenuHotkey
	StyleMenuSelectedHotkey
	StyleMenuDisabled
	StyleDialog
	StyleDialogButton
	StyleDialogButtonFocused
	StyleToast
	StyleToastError
	StyleScrollbar
	StyleScrollThumb
	StyleSelected // selected list row
	StyleInput
	StyleInputCursor
	StyleDialogError
	StyleHeader // column headers / path bar
	StyleMarked // selected (marked) file rows
	StyleDropTarget
	StyleGutter
	StyleCurLine
	StyleSelection
	StyleBracket
	StyleSynComment
	StyleSynString
	StyleSynKeyword
	StyleSynNumber
	StyleSynVar
	StyleSynKey
	StyleSynHeader
	StyleShadow            // what lies in the shadow of a window, menu or dialog
	StyleHover             // list row or icon under the mouse pointer
	StyleBarHover          // bar item or menu title under the pointer
	StyleButtonHover       // title-bar button under the pointer
	StyleClose             // close button of the focused window
	StyleCloseHover        // close button under the pointer
	StyleDialogButtonHover // dialog button under the pointer
	// StyleTerm cells carry their own colors in Cell.T (terminal windows).
	StyleTerm

	NumStyles
)

// TermColor is a terminal color: 0 = the default color, otherwise
// TermPalette|index (0-255) or TermRGB|0xRRGGBB.
type TermColor uint32

const (
	TermPalette TermColor = 1 << 24
	TermRGB     TermColor = 2 << 24
)

// Terminal text attributes.
const (
	AttrBold uint8 = 1 << iota
	AttrDim
	AttrItalic
	AttrUnderline
	AttrBlink
	AttrReverse
	AttrStrike
)

// TermStyle is the explicit style of a StyleTerm cell.
type TermStyle struct {
	FG, BG TermColor
	Attr   uint8
}
