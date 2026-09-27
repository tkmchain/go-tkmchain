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

package gui

import (
	"context"
	urlpkg "net/url"
	"os/exec"
	"runtime"
	"strings"

	"github.com/ethereum/go-ethereum/log"
)

// openExternal opens the given URL in the platform default browser.
func openExternal(url string) {
	parsed, err := urlpkg.Parse(strings.TrimSpace(url))
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		log.Info("Dashboard available at", "url", url)
		return
	}
	var cmdName string
	var args []string
	switch runtime.GOOS {
	case "linux":
		cmdName = "xdg-open"
		args = []string{url}
	case "darwin":
		cmdName = "open"
		args = []string{url}
	case "windows":
		cmdName = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		log.Info("Dashboard available at", "url", url)
		return
	}
	validArgs := (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && len(args) == 1 && args[0] == parsed.String()
	if runtime.GOOS == "windows" {
		validArgs = len(args) == 2 && args[0] == "url.dll,FileProtocolHandler" && args[1] == parsed.String()
	}
	if cmdName == "" || !validArgs {
		log.Info("Dashboard available at", "url", url)
		return
	}
	if err := exec.CommandContext(context.Background(), cmdName, args...).Start(); err != nil { // #nosec G204 -- fixed platform opener and validated dashboard URL
		log.Info("Could not open your default browser automatically; open the dashboard manually.", "url", url)
		return
	}
	log.Info("Opened dashboard in the default browser.", "url", url)
}
