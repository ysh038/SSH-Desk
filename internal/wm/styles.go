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

	NumStyles
)
