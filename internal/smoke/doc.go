// Package smoke holds the smoke tests that start the real built binary in
// each of its modes (tray, --manage, --countdown). They only build with
// `-tags smoke` and need a display; see smoke_test.go. The package has no
// code of its own and stays free of Fyne, so compiling these tests is quick
// even on Windows, where the binary under test is the slow part to build.
package smoke
