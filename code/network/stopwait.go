package network

import (
	"encoding/binary"
)

const (
	SW_CMD_PUSH = 1
	SW_CMD_ACK  = 2

	HEAD_SIZE = 9
)

type segment struct {
	cmd  uint8
	ack  uint32
	sn   uint32
	data []byte
}

type Sw struct {
	snd         *segment
	rcv         *segment
	expectedSeq uint32
}

func NewSw() *Sw {
	return &Sw{
		expectedSeq: 0,
		nextSeq:     0,
	}
}

func (sw *Sw) Input(buf []byte) {
	if len(buf) < HEAD_SIZE {
		return
	}

	seg := decodeSegment(buf)

	if seg.cmd == SW_CMD_PUSH {
		if seg.sn != sw.expectedSeq {
			return
		} else {
			sw.rcvBuf = append(sw.rcvBuf, seg)
			sw.expectedSeq += 1
		}
	}

	if seg.cmd == SW_CMD_ACK {
		if len(sw.sndbuf) > 0 && sw.sndbuf[0].sn == seg.ack {
			if len(sw.sndbuf) > 1 {
				sw.sndbuf = sw.sndbuf[1:]
			} else {
				sw.sndbuf = make([]segment, 32)
			}
		}
	}
}

func decodeSegment(buf []byte) segment {
	seg := segment{}
	seg.cmd = buf[0]
	seg.ack = binary.BigEndian.Uint32(buf[1:5])
	seg.sn = binary.BigEndian.Uint32(buf[5:9])
	seg.data = buf[9:]
	return seg
}

func (sw *Sw) Output(buf []byte) {
	seg := segment{
		cmd: SW_CMD_PUSH,
		ack: 0,
		sn:  sw.nextSeq,
	}

	sw.sndbuf = append(sw.sndbuf, seg)
	sw.nextSeq += 1
}
