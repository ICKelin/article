package gbn

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	_ = iota
	cmdPush
	cmdAck
)

var (
	cmdSize      = 1
	seqSize      = 4
	overHead     = cmdSize + seqSize
	defaultRto   = time.Millisecond * 50
	defaultDelay = int64(30) // 默认30ms延迟ack
	errAgain     = fmt.Errorf("EAGAIN")
	defaultWnd   = uint32(1024)
)

type Gbn struct {
	// 接收缓冲区
	rcvMu  sync.Mutex
	rcvbuf *list.List

	// 输出回调函数
	// 上层数据调用SendTo之后
	// 内部进行gbn协议编码之后，执行此函数
	output func([]byte)

	// 预期接收到的序列号
	expectSeq uint32

	// 收到push事件
	recvPush chan struct{}

	// 收到ack事件
	recvAck chan *segment

	// 发送缓冲区
	sndbuf []segment

	// 发送方滑动窗口设计
	//               |------sndwnd------|
	// |-------------|------------------|---------------|
	// | sent&acked  |     sent&unack   |    blocked    |
	// |-------------|------------------|---------------|
	// |           sndbase            nextSeq           |
	// |------------------------------------------------|
	sndBase uint32
	nextSeq uint32
	sndwnd  uint32
	locker  sync.Mutex

	// 可用窗口大小
	availwnd uint32

	// rto
	rto time.Duration

	// mss，超过mss则进行分片
	mss int

	// rtt采样
	rtt    time.Duration
	minrtt time.Duration
	maxrtt time.Duration

	// 超时计时器
	timer *time.Timer

	delayAck int64
	lastAck  time.Time
}

type segment struct {
	cmd  uint8
	seq  uint32
	data []byte
}

func NewGbn(output func([]byte), nodelay bool) *Gbn {
	delayAck := defaultDelay
	if nodelay {
		delayAck = 0
	}

	gbn := &Gbn{
		rcvbuf:    list.New(),
		output:    output,
		recvPush:  make(chan struct{}),
		recvAck:   make(chan *segment),
		sndwnd:    defaultWnd,
		availwnd:  defaultWnd,
		sndBase:   1, // 第一个包序列号为1
		nextSeq:   1,
		expectSeq: 1,
		rto:       defaultRto,
		timer:     time.NewTimer(defaultRto),
		mss:       1400,
		delayAck:  delayAck,
	}

	go gbn.check()
	return gbn
}

func (gbn *Gbn) SendTo(data []byte) error {
	gbn.locker.Lock()
	// 无可用窗口
	if gbn.nextSeq-gbn.sndBase == gbn.sndwnd {
		log.Println("[D] not available buf")
		gbn.locker.Unlock()
		return errAgain
	}

	if gbn.sndBase == gbn.nextSeq {
		gbn.timer.Reset(gbn.rto)
	}

	seg := segment{
		cmd:  cmdPush,
		seq:  gbn.nextSeq,
		data: data,
	}

	enc := gbn.encode(seg)

	gbn.sndbuf = append(gbn.sndbuf, seg)
	gbn.nextSeq += 1

	gbn.locker.Unlock()

	gbn.output(enc)
	return nil
}

func (gbn *Gbn) Input(buf []byte) error {
	seg, err := gbn.decode(buf)
	if err != nil {
		return err
	}

	// ack包
	if seg.cmd == cmdAck {
		// 非正常包，探测或者攻击包
		if seg.seq >= gbn.nextSeq {
			log.Printf("[FATAL] unordered packet %d\n", seg.seq)
			return nil
		}

		if seg.seq < gbn.sndBase {
			return nil
		}

		// 调整可用窗口
		gbn.availwnd += (seg.seq - gbn.sndBase + 1)

		// 移动sndbase
		moveSize := seg.seq - gbn.sndBase + 1
		fmt.Printf("send base %d, seg.seq %d\n", gbn.sndBase, seg.seq)
		if moveSize > 0 {
			log.Printf("[D] wnd move %d\n", moveSize)
		}

		// 调整已确认的数据包
		gbn.sndBase = seg.seq + 1

		// 移动窗口，移除已确认数据包
		gbn.locker.Lock()
		if len(gbn.sndbuf) > int(moveSize) {
			gbn.sndbuf = gbn.sndbuf[moveSize:]
		}
		gbn.locker.Unlock()

		log.Println("[D] receive ack: ", seg.seq)
		gbn.recvAck <- &seg
	}

	// 数据包
	if seg.cmd == cmdPush {
		// 非正常包，探测或者攻击包
		if seg.seq > gbn.expectSeq {
			log.Printf("[FATAL] expected seq %d, got %d",
				gbn.expectSeq, seg.seq)

			return nil
		}

		// 已经被确认的包
		if seg.seq < gbn.expectSeq {
			log.Printf("[D] drop segment %d, expectedSeq is %d\n",
				seg.seq, gbn.expectSeq)
			return nil
		}

		log.Printf("[D] receive pkt %d, expectSeq: %d\n", seg.seq, gbn.expectSeq)
		log.Println("[D] push back ", seg.seq, string(seg.data))
		gbn.rcvMu.Lock()
		gbn.rcvbuf.PushBack(seg)
		gbn.rcvMu.Unlock()

		ack := true

		// 通知上层收包
		select {
		case gbn.recvPush <- struct{}{}:
		default:
		}

		gbn.expectSeq += 1

		if ack {
			enc := gbn.encode(segment{
				cmd: cmdAck,
				seq: seg.seq,
			})
			gbn.output(enc)
			gbn.lastAck = time.Now()
		}
	}

	return nil
}

func (gbn *Gbn) RecvFrom() ([]byte, error) {
	gbn.rcvMu.Lock()

	if gbn.rcvbuf.Len() > 0 {
		ele := gbn.rcvbuf.Front()
		seg := ele.Value.(segment)
		gbn.rcvbuf.Remove(ele)
		gbn.rcvMu.Unlock()
		return seg.data, nil
	}
	gbn.rcvMu.Unlock()

	<-gbn.recvPush
	gbn.rcvMu.Lock()
	if gbn.rcvbuf.Len() > 0 {
		ele := gbn.rcvbuf.Front()
		seg := ele.Value.(segment)
		gbn.rcvbuf.Remove(ele)
		gbn.rcvMu.Unlock()
		return seg.data, nil
	}
	gbn.rcvMu.Unlock()

	return nil, errAgain
}

func (gbn *Gbn) check() {
	for {
		select {
		case <-gbn.timer.C:
			gbn.locker.Lock()
			// 重传已发送，但是还未重传的数据
			if gbn.sndBase < gbn.nextSeq {
				log.Printf("[D]resend %d to %d\n", gbn.sndBase, gbn.nextSeq)
				count := gbn.nextSeq - gbn.sndBase
				for i, seg := range gbn.sndbuf {
					enc := gbn.encode(seg)
					gbn.output(enc)
					if uint32(i) >= count {
						break
					}
				}
			}
			gbn.locker.Unlock()
			gbn.timer.Reset(gbn.rto)

		case <-gbn.recvAck:
			// 重置定时器
			gbn.timer.Reset(gbn.rto)
		}

	}
}

func (gbn *Gbn) encode(seg segment) []byte {
	data := make([]byte, 5)
	data[0] = seg.cmd
	binary.BigEndian.PutUint32(data[1:], seg.seq)
	data = append(data, seg.data...)
	return data
}

func (gbn *Gbn) decode(buf []byte) (segment, error) {
	if len(buf) < overHead {
		return segment{}, fmt.Errorf("invalid overhead")
	}

	seq := binary.BigEndian.Uint32(buf[cmdSize:])
	seg := segment{
		cmd:  buf[0],
		seq:  seq,
		data: buf[cmdSize+seqSize:],
	}

	return seg, nil
}
