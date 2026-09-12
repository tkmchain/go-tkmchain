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

//go:build !gtkmgui

package gui

// This file provides the browser fallback for builds without the "gtkmgui"
// build tag: the dashboard is opened in the default web browser instead of a
// native desktop window.

// openDesktopWindow opens the dashboard in the default browser and blocks
// until the process is shut down.
func openDesktopWindow(url string, opts Options) error {
	openExternal(url)
	select {}
}
