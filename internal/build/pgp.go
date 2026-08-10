// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// signFile reads the contents of an input file and signs it (in armored format)
// with the key provided, placing the signature into the output file.

package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PGPSignFile parses a PGP private key from the specified string and creates a
// signature file into the output parameter of the input file.
//
// Note, this method assumes a single key will be container in the pgpkey arg,
// furthermore that it is in armored format.
func PGPSignFile(input string, output string, pgpkey string) error {
	home, cleanup, err := newGPGHome()
	if err != nil {
		return err
	}
	defer cleanup()
	if err := importPGPKey(home, pgpkey); err != nil {
		return err
	}
	keyID, err := gpgKeyID(home)
	if err != nil {
		return err
	}
	cmd := exec.Command("gpg",
		"--batch",
		"--yes",
		"--no-tty",
		"--pinentry-mode", "loopback",
		"--homedir", home,
		"--armor",
		"--detach-sign",
		"--local-user", keyID,
		"--output", output,
		input,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gpg detach-sign failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PGPKeyID parses an armored key and returns the key ID.
func PGPKeyID(pgpkey string) (string, error) {
	home, cleanup, err := newGPGHome()
	if err != nil {
		return "", err
	}
	defer cleanup()
	if err := importPGPKey(home, pgpkey); err != nil {
		return "", err
	}
	return gpgKeyID(home)
}

func newGPGHome() (string, func(), error) {
	home, err := os.MkdirTemp("", "tkm-build-gpg-")
	if err != nil {
		return "", nil, err
	}
	return home, func() { _ = os.RemoveAll(home) }, nil
}

func importPGPKey(home string, pgpkey string) error {
	cmd := exec.Command("gpg",
		"--batch",
		"--yes",
		"--no-tty",
		"--pinentry-mode", "loopback",
		"--homedir", home,
		"--import",
	)
	cmd.Stdin = bytes.NewBufferString(pgpkey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gpg import failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gpgKeyID(home string) (string, error) {
	for _, listArgs := range [][]string{
		{"--list-secret-keys"},
		{"--list-keys"},
	} {
		keyID, found, err := gpgKeyIDFromList(home, listArgs)
		if err != nil {
			return "", err
		}
		if found {
			return keyID, nil
		}
	}
	return "", fmt.Errorf("key count mismatch: have %d, want %d", 0, 1)
}

func gpgKeyIDFromList(home string, listArgs []string) (string, bool, error) {
	args := append([]string{
		"--batch",
		"--yes",
		"--no-tty",
		"--homedir", home,
		"--with-colons",
	}, listArgs...)
	cmd := exec.Command("gpg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("gpg %s failed: %w: %s", strings.Join(listArgs, " "), err, strings.TrimSpace(string(out)))
	}
	ids := make([]string, 0, 1)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 5 {
			continue
		}
		if fields[0] != "sec" && fields[0] != "pub" {
			continue
		}
		if fields[4] == "" {
			continue
		}
		if _, ok := seen[fields[4]]; ok {
			continue
		}
		seen[fields[4]] = struct{}{}
		ids = append(ids, fields[4])
	}
	switch len(ids) {
	case 0:
		return "", false, nil
	case 1:
		return ids[0], true, nil
	default:
		return "", false, fmt.Errorf("key count mismatch: have %d, want %d", len(ids), 1)
	}
}
