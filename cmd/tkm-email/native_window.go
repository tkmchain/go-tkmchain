//go:build emailgui

package main

import webview "github.com/GopeedLab/webview_go"

func runWindow(url string) error {
	window := webview.New(false)
	defer window.Destroy()
	window.SetTitle("TKM Email")
	window.SetSize(1120, 760, webview.HintNone)
	window.Navigate(url)
	window.Run()
	return nil
}
