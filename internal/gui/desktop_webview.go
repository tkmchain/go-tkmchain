// Copyright 2026 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

//go:build gtkmgui

package gui

// This file provides the native desktop window implementation. It is only
// compiled when the "gtkmgui" build tag is set, which requires the system
// WebKit/GTK libraries on Linux (libgtk-3-dev, libwebkit2gtk-4.1-dev).

import (
	webview "github.com/GopeedLab/webview_go"
	"github.com/ethereum/go-ethereum/log"
)

// openDesktopWindow renders the dashboard in a native desktop window and
// blocks until the window is closed.
func openDesktopWindow(url string, opts Options) error {
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(opts.Title)
	w.SetSize(760, 560, webview.HintMin)
	w.SetSize(opts.Width, opts.Height, webview.HintNone)
	log.Info("Opening GUI window", "url", url)
	w.Navigate(url)
	w.Run()
	return nil
}
