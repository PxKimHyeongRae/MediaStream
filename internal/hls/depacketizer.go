package hls

import (
	"encoding/binary"
	"fmt"

	"github.com/pion/rtp"
)

// Depacketizer는 RTP 패킷에서 미디어 데이터를 추출합니다
type Depacketizer interface {
	Depacketize(pkt *rtp.Packet) ([][]byte, error)
}

// H264Depacketizer는 H.264 RTP 디패킷화 (RFC 6184)
type H264Depacketizer struct {
	buffer []byte
}

// NewH264Depacketizer는 새로운 H264 depacketizer를 생성
func NewH264Depacketizer() *H264Depacketizer {
	return &H264Depacketizer{
		buffer: make([]byte, 0, 65536),
	}
}

// Depacketize는 RTP 패킷에서 H.264 NAL units를 추출
func (d *H264Depacketizer) Depacketize(pkt *rtp.Packet) ([][]byte, error) {
	if len(pkt.Payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	payload := pkt.Payload
	nalUnits := make([][]byte, 0)

	// NAL unit type (첫 바이트의 하위 5비트)
	nalType := payload[0] & 0x1F

	switch {
	case nalType >= 1 && nalType <= 23:
		// Single NAL Unit Packet
		nalUnit := make([]byte, len(payload))
		copy(nalUnit, payload)
		nalUnits = append(nalUnits, nalUnit)

	case nalType == 24:
		// STAP-A (Single Time Aggregation Packet)
		offset := 1
		for offset < len(payload) {
			if offset+2 > len(payload) {
				break
			}

			// NAL unit 크기 읽기 (2바이트, big-endian)
			nalSize := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
			offset += 2

			if offset+nalSize > len(payload) {
				break
			}

			// NAL unit 추출
			nalUnit := make([]byte, nalSize)
			copy(nalUnit, payload[offset:offset+nalSize])
			nalUnits = append(nalUnits, nalUnit)
			offset += nalSize
		}

	case nalType == 28:
		// FU-A (Fragmentation Unit)
		if len(payload) < 2 {
			return nil, fmt.Errorf("FU-A payload too short")
		}

		fuHeader := payload[1]
		startBit := (fuHeader & 0x80) != 0
		endBit := (fuHeader & 0x40) != 0
		nalHeader := (payload[0] & 0xE0) | (fuHeader & 0x1F)

		if startBit {
			// 새로운 NAL unit 시작
			d.buffer = d.buffer[:0]
			d.buffer = append(d.buffer, nalHeader)
			d.buffer = append(d.buffer, payload[2:]...)
		} else {
			// NAL unit 계속
			d.buffer = append(d.buffer, payload[2:]...)
		}

		if endBit {
			// NAL unit 완성
			nalUnit := make([]byte, len(d.buffer))
			copy(nalUnit, d.buffer)
			nalUnits = append(nalUnits, nalUnit)
			d.buffer = d.buffer[:0]
		}
	}

	return nalUnits, nil
}

// H265Depacketizer는 H.265/HEVC RTP 디패킷화 (RFC 7798)
type H265Depacketizer struct {
	buffer []byte
}

// NewH265Depacketizer는 새로운 H265 depacketizer를 생성
func NewH265Depacketizer() *H265Depacketizer {
	return &H265Depacketizer{
		buffer: make([]byte, 0, 65536),
	}
}

// Depacketize는 RTP 패킷에서 H.265 NAL units를 추출
func (d *H265Depacketizer) Depacketize(pkt *rtp.Packet) ([][]byte, error) {
	if len(pkt.Payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	payload := pkt.Payload
	nalUnits := make([][]byte, 0)

	// NAL unit type (첫 바이트의 비트 1-6)
	nalType := (payload[0] >> 1) & 0x3F

	switch {
	case nalType <= 47:
		// Single NAL Unit Packet
		nalUnit := make([]byte, len(payload))
		copy(nalUnit, payload)
		nalUnits = append(nalUnits, nalUnit)

	case nalType == 48:
		// Aggregation Packet (AP)
		offset := 2 // Skip PayloadHdr
		for offset < len(payload) {
			if offset+2 > len(payload) {
				break
			}

			// NAL unit 크기 읽기
			nalSize := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
			offset += 2

			if offset+nalSize > len(payload) {
				break
			}

			// NAL unit 추출
			nalUnit := make([]byte, nalSize)
			copy(nalUnit, payload[offset:offset+nalSize])
			nalUnits = append(nalUnits, nalUnit)
			offset += nalSize
		}

	case nalType == 49:
		// Fragmentation Unit (FU)
		if len(payload) < 3 {
			return nil, fmt.Errorf("FU payload too short")
		}

		fuHeader := payload[2]
		startBit := (fuHeader & 0x80) != 0
		endBit := (fuHeader & 0x40) != 0

		// NAL unit type from FU header
		fuType := fuHeader & 0x3F

		if startBit {
			// 새로운 NAL unit 시작
			d.buffer = d.buffer[:0]
			// Reconstruct NAL header
			nalHeader1 := (payload[0] & 0x81) | (fuType << 1)
			nalHeader2 := payload[1]
			d.buffer = append(d.buffer, nalHeader1, nalHeader2)
			d.buffer = append(d.buffer, payload[3:]...)
		} else {
			// NAL unit 계속
			d.buffer = append(d.buffer, payload[3:]...)
		}

		if endBit {
			// NAL unit 완성
			nalUnit := make([]byte, len(d.buffer))
			copy(nalUnit, d.buffer)
			nalUnits = append(nalUnits, nalUnit)
			d.buffer = d.buffer[:0]
		}
	}

	return nalUnits, nil
}
