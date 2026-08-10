// Copyright 2025 The go-ethereum Authors
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

package nat

import (
	"crypto/rand"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand"
	"net"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

//go:embed stun-list.txt
var stunDefaultServers string

const requestLimit = 3
const (
	stunDefaultPort        = 3478
	stunHeaderSize         = 20
	stunBindingRequest     = 0x0001
	stunBindingSuccess     = 0x0101
	stunMagicCookie        = 0x2112A442
	stunAttrMappedAddress  = 0x0001
	stunAttrXORMappedAddr  = 0x0020
	stunFamilyIPv4         = 0x01
	stunFamilyIPv6         = 0x02
	stunRequestTimeout     = 5 * time.Second
	stunMaxResponseMessage = 1500
)

var errSTUNFailed = errors.New("STUN requests failed")

type stun struct {
	serverList []string
}

func newSTUN(serverAddr string) (Interface, error) {
	s := new(stun)
	if serverAddr == "" {
		s.serverList = strings.Split(stunDefaultServers, "\n")
	} else {
		_, err := net.ResolveUDPAddr("udp4", serverAddr)
		if err != nil {
			return nil, err
		}
		s.serverList = []string{serverAddr}
	}
	return s, nil
}

func (s stun) String() string {
	if len(s.serverList) == 1 {
		return fmt.Sprintf("stun:%s", s.serverList[0])
	}
	return "stun"
}

func (s stun) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func (stun) SupportsMapping() bool {
	return false
}

func (stun) AddMapping(protocol string, extport, intport int, name string, lifetime time.Duration) (uint16, error) {
	return uint16(extport), nil
}

func (stun) DeleteMapping(string, int, int) error {
	return nil
}

func (s *stun) ExternalIP() (net.IP, error) {
	for _, server := range s.randomServers(requestLimit) {
		ip, err := s.externalIP(server)
		if err != nil {
			log.Debug("STUN request failed", "server", server, "err", err)
			continue
		}
		return ip, nil
	}
	return nil, errSTUNFailed
}

func (s *stun) randomServers(n int) []string {
	n = min(n, len(s.serverList))
	m := make(map[int]struct{}, n)
	list := make([]string, 0, n)
	for i := 0; i < len(s.serverList)*2 && len(list) < n; i++ {
		index := mathrand.Intn(len(s.serverList))
		if _, alreadyHit := m[index]; alreadyHit {
			continue
		}
		list = append(list, s.serverList[index])
		m[index] = struct{}{}
	}
	return list
}

func (s *stun) externalIP(server string) (net.IP, error) {
	_, _, err := net.SplitHostPort(server)
	if err != nil {
		server += fmt.Sprintf(":%d", stunDefaultPort)
	}

	log.Trace("Attempting STUN binding request", "server", server)
	addr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(stunRequestTimeout)); err != nil {
		return nil, err
	}
	message, transactionID, err := stunBindingRequestMessage()
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(message); err != nil {
		return nil, err
	}
	buf := make([]byte, stunMaxResponseMessage)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	ip, err := stunMappedAddress(buf[:n], transactionID)
	if err != nil {
		return nil, err
	}
	log.Trace("STUN returned IP", "server", server, "ip", ip)
	return ip, nil
}

func stunBindingRequestMessage() ([]byte, [12]byte, error) {
	var transactionID [12]byte
	if _, err := io.ReadFull(rand.Reader, transactionID[:]); err != nil {
		return nil, transactionID, err
	}
	message := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(message[0:2], stunBindingRequest)
	binary.BigEndian.PutUint16(message[2:4], 0)
	binary.BigEndian.PutUint32(message[4:8], stunMagicCookie)
	copy(message[8:20], transactionID[:])
	return message, transactionID, nil
}

func stunMappedAddress(message []byte, transactionID [12]byte) (net.IP, error) {
	if len(message) < stunHeaderSize {
		return nil, errors.New("short STUN response")
	}
	if binary.BigEndian.Uint16(message[0:2]) != stunBindingSuccess {
		return nil, fmt.Errorf("unexpected STUN response type 0x%04x", binary.BigEndian.Uint16(message[0:2]))
	}
	bodyLen := int(binary.BigEndian.Uint16(message[2:4]))
	if bodyLen > len(message)-stunHeaderSize {
		return nil, errors.New("truncated STUN response")
	}
	if binary.BigEndian.Uint32(message[4:8]) != stunMagicCookie {
		return nil, errors.New("invalid STUN magic cookie")
	}
	if !equalBytes(message[8:20], transactionID[:]) {
		return nil, errors.New("STUN transaction ID mismatch")
	}
	attrs := message[stunHeaderSize : stunHeaderSize+bodyLen]
	for len(attrs) >= 4 {
		attrType := binary.BigEndian.Uint16(attrs[0:2])
		attrLen := int(binary.BigEndian.Uint16(attrs[2:4]))
		if attrLen > len(attrs)-4 {
			return nil, errors.New("truncated STUN attribute")
		}
		value := attrs[4 : 4+attrLen]
		switch attrType {
		case stunAttrXORMappedAddr:
			return parseSTUNAddress(value, transactionID, true)
		case stunAttrMappedAddress:
			return parseSTUNAddress(value, transactionID, false)
		}
		padded := (attrLen + 3) &^ 3
		if padded > len(attrs)-4 {
			break
		}
		attrs = attrs[4+padded:]
	}
	return nil, errors.New("STUN mapped address missing")
}

func parseSTUNAddress(value []byte, transactionID [12]byte, xor bool) (net.IP, error) {
	if len(value) < 4 || value[0] != 0 {
		return nil, errors.New("invalid STUN address attribute")
	}
	switch value[1] {
	case stunFamilyIPv4:
		if len(value) < 8 {
			return nil, errors.New("short STUN IPv4 address")
		}
		ip := make(net.IP, net.IPv4len)
		copy(ip, value[4:8])
		if xor {
			cookie := make([]byte, 4)
			binary.BigEndian.PutUint32(cookie, stunMagicCookie)
			for i := range ip {
				ip[i] ^= cookie[i]
			}
		}
		return ip, nil
	case stunFamilyIPv6:
		if len(value) < 20 {
			return nil, errors.New("short STUN IPv6 address")
		}
		ip := make(net.IP, net.IPv6len)
		copy(ip, value[4:20])
		if xor {
			mask := make([]byte, 16)
			binary.BigEndian.PutUint32(mask[:4], stunMagicCookie)
			copy(mask[4:], transactionID[:])
			for i := range ip {
				ip[i] ^= mask[i]
			}
		}
		return ip, nil
	default:
		return nil, fmt.Errorf("unsupported STUN address family %d", value[1])
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
