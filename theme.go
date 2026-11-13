package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Frictionless borrows its base language from the genre it lives in — Steam,
// GOG Galaxy, Lutris, Heroic — dark, card-based libraries that each claim one
// confident accent. Most of them also reserve green specifically for
// "ready to play" (Steam's Play button, Heroic's install state), so rather
// than invent a color for "this game is armed," Ready Green just uses the
// signal players already know. Ice Blue is demoted from a general accent to
// the one place ice is actually a narrative rather than a coat of paint: the
// countdown window's cold starting state, which thaws to Melt Amber exactly
// as the game launches. See showLaunchCountdown.
var (
	colorReady     = color.NRGBA{R: 0x3f, G: 0xbe, B: 0x7a, A: 0xff}
	colorIce       = color.NRGBA{R: 0x45, G: 0xc4, B: 0xe5, A: 0xff}
	colorMeltAmber = color.NRGBA{R: 0xff, G: 0x8a, B: 0x42, A: 0xff}
	colorFracture  = color.NRGBA{R: 0xe5, G: 0x53, B: 0x4b, A: 0xff}

	colorVoid  = color.NRGBA{R: 0x0f, G: 0x14, B: 0x19, A: 0xff}
	colorFrost = color.NRGBA{R: 0xf2, G: 0xf4, B: 0xf6, A: 0xff}

	colorSteelOnDark  = color.NRGBA{R: 0xe9, G: 0xf1, B: 0xf3, A: 0xff}
	colorSteelOnLight = color.NRGBA{R: 0x22, G: 0x28, B: 0x2c, A: 0xff}
	colorMuted        = color.NRGBA{R: 0x80, G: 0x8c, B: 0x93, A: 0xff}

	// colorRim is a soft light-catch along the top edge of a raised surface —
	// e.g. a list row's card — rather than a hard border.
	colorRim = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x26}
	// colorSurface{Dark,Light} are the row/card tone — deliberately its own
	// token rather than reusing ColorNameButton, so a card can read as a
	// distinct raised surface without changing every button's fill too.
	colorSurfaceDark  = color.NRGBA{R: 0x1b, G: 0x21, B: 0x29, A: 0xff}
	colorSurfaceLight = color.NRGBA{R: 0xe3, G: 0xe8, B: 0xea, A: 0xff}
)

// colorSurface returns the row/card tone for the current theme variant.
func colorSurface() color.Color {
	if fyne.CurrentApp() != nil && fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantLight {
		return colorSurfaceLight
	}
	return colorSurfaceDark
}

type frictionlessTheme struct{}

func newFrictionlessTheme() fyne.Theme {
	return &frictionlessTheme{}
}

func (frictionlessTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	dark := variant == theme.VariantDark

	switch name {
	case theme.ColorNameBackground:
		if dark {
			return colorVoid
		}
		return colorFrost
	case theme.ColorNameForeground:
		if dark {
			return colorSteelOnDark
		}
		return colorSteelOnLight
	case theme.ColorNameButton:
		if dark {
			return color.NRGBA{R: 0x16, G: 0x1c, B: 0x22, A: 0xff}
		}
		return color.NRGBA{R: 0xe1, G: 0xe6, B: 0xe8, A: 0xff}
	case theme.ColorNameInputBackground:
		if dark {
			return color.NRGBA{R: 0x11, G: 0x16, B: 0x1b, A: 0xff}
		}
		return color.White
	case theme.ColorNameHeaderBackground, theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		if dark {
			return color.NRGBA{R: 0x0d, G: 0x12, B: 0x16, A: 0xff}
		}
		return colorFrost
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return color.NRGBA{R: colorMuted.R, G: colorMuted.G, B: colorMuted.B, A: 0x55}
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink, theme.ColorNameSuccess:
		return colorReady
	case theme.ColorNameSelection:
		return color.NRGBA{R: colorReady.R, G: colorReady.G, B: colorReady.B, A: 0x55}
	case theme.ColorNameHover:
		return color.NRGBA{R: colorReady.R, G: colorReady.G, B: colorReady.B, A: 0x22}
	case theme.ColorNamePressed:
		return color.NRGBA{R: colorReady.R, G: colorReady.G, B: colorReady.B, A: 0x40}
	case theme.ColorNameWarning, theme.ColorNameError:
		return colorFracture
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnSuccess:
		return colorVoid
	case theme.ColorNameForegroundOnError, theme.ColorNameForegroundOnWarning:
		return colorFrost
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled, theme.ColorNameDisabledButton:
		return colorMuted
	}

	return theme.DefaultTheme().Color(name, variant)
}

func (frictionlessTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (frictionlessTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Size mostly defers to the default theme, but pulls corner radii in tight —
// a faceted, cut-ice look rather than the default's softer rounding.
func (frictionlessTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameButtonRadius, theme.SizeNameInputRadius, theme.SizeNameCardRadius,
		theme.SizeNamePopupRadius, theme.SizeNameMenuRadius, theme.SizeNameSelectionRadius,
		theme.SizeNameInnerWindowRadius, theme.SizeNameScrollBarRadius:
		return 2
	case theme.SizeNameDialogRadius:
		return 4
	}

	return theme.DefaultTheme().Size(name)
}

// lerpColor blends a toward b as t goes from 0 to 1.
func lerpColor(a, b color.Color, t float64) color.Color {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()

	blend := func(x, y uint32) uint8 {
		return uint8((float64(x>>8)*(1-t) + float64(y>>8)*t) + 0.5)
	}

	return color.NRGBA{
		R: blend(ar, br),
		G: blend(ag, bg),
		B: blend(ab, bb),
		A: blend(aa, ba),
	}
}
