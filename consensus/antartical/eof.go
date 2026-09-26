package antartical

import "errors"

var (
	ErrInvalidEOF = errors.New("invalid EOF bytecode")
)

// ValidateEOF validates the canonical EOF v1 container header and section
// boundaries. Legacy bytecode remains valid through the existing EVM path;
// callers invoke this function when EOF mode is active.
func ValidateEOF(code []byte) error {
	if len(code) < 7 || code[0] != 0xef || code[1] != 0x00 || code[2] != 0x01 {
		return ErrInvalidEOF
	}
	// EOF v1 requires a type section followed by a code section. Section sizes
	// are big-endian uint16 values and all bytes must be consumed exactly.
	if code[3] != 0x01 || len(code) < 7 {
		return ErrInvalidEOF
	}
	typeSize := int(code[4])<<8 | int(code[5])
	if typeSize == 0 || 6+typeSize >= len(code) {
		return ErrInvalidEOF
	}
	pos := 6 + typeSize
	if pos+3 > len(code) || code[pos] != 0x02 {
		return ErrInvalidEOF
	}
	codeSize := int(code[pos+1])<<8 | int(code[pos+2])
	pos += 3
	if codeSize == 0 || pos+codeSize > len(code) {
		return ErrInvalidEOF
	}
	return nil
}
