package gbn

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"log"
	"net"
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
	defaultRto   = time.Millisecond * 100
	defaultDelay = int64(300) // 默认30ms延迟ack
	errAgain     = fmt.Errorf("EAGAIN")
	defaultWnd   = uint32(1024)
)

type Gbn struct {
	conn *net.UDPConn

	// 接收缓冲区
	rcvMu  sync.Mutex
	rcvbuf *list.List

	// 预期接收到的序列号
	expectSeq uint32

	// 收到push事件
	recvPush chan struct{}

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
	cmd uint8
	// 序列号
	// cmdPush时，为发送数据的序列号
	// cmdAck时，为确认收到的序列号
	seq  uint32
	data []byte

	raddr *net.UDPAddr
}

func NewGbn(conn *net.UDPConn, nodelay bool) *Gbn {
	delayAck := defaultDelay
	if nodelay {
		delayAck = 0
	}

	gbn := &Gbn{
		conn:     conn,
		rcvbuf:   list.New(),
		recvPush: make(chan struct{}),
		sndwnd:   defaultWnd,
		availwnd: defaultWnd,
		rto:      defaultRto,
		timer:    time.NewTimer(defaultRto),
		mss:      1400,
		delayAck: delayAck,
	}

	go gbn.read()
	go gbn.check()
	return gbn
}

func (gbn *Gbn) Peek() ([]byte, *net.UDPAddr, error) {
	gbn.rcvMu.Lock()

	if gbn.rcvbuf.Len() > 0 {
		ele := gbn.rcvbuf.Front()
		seg := ele.Value.(segment)
		gbn.rcvbuf.Remove(ele)
		gbn.rcvMu.Unlock()
		return seg.data, seg.raddr, nil
	}
	gbn.rcvMu.Unlock()

	select {
	case <-gbn.recvPush:
		gbn.rcvMu.Lock()
		if gbn.rcvbuf.Len() > 0 {
			ele := gbn.rcvbuf.Front()
			seg := ele.Value.(segment)
			gbn.rcvbuf.Remove(ele)
			gbn.rcvMu.Unlock()
			return seg.data, seg.raddr, nil
		}
		gbn.rcvMu.Unlock()
	}

	return nil, nil, errAgain
}

func (gbn *Gbn) Send(data []byte, raddr *net.UDPAddr) {
	gbn.locker.Lock()
	// 无可用窗口
	if gbn.nextSeq-gbn.sndBase == gbn.sndwnd {
		log.Println("[D] not available buf")
		gbn.locker.Unlock()
		return
	}

	if gbn.sndBase == gbn.nextSeq {
		gbn.timer.Reset(gbn.rto)
	}

	gbn.locker.Unlock()

	gbn.sendFragment(data, raddr)
}

func (gbn *Gbn) sendFragment(data []byte, raddr *net.UDPAddr) {
	seg := segment{
		cmd:   cmdPush,
		seq:   gbn.nextSeq,
		data:  data,
		raddr: raddr,
	}
	gbn.tx(seg)
	gbn.sndbuf = append(gbn.sndbuf, seg)
	gbn.nextSeq += 1
}

func (gbn *Gbn) check() {
	for range gbn.timer.C {
		// 重传已发送，但是还未重传的数据
		if gbn.sndBase < gbn.nextSeq {
			log.Printf("[D]resend %d to %d\n", gbn.sndBase, gbn.nextSeq)
			count := gbn.nextSeq - gbn.sndBase
			for i, seg := range gbn.sndbuf {
				gbn.tx(seg)
				if uint32(i) >= count {
					break
				}
			}
		}
		gbn.timer.Reset(gbn.rto)
	}
}

func (gbn *Gbn) read() error {
	for {
		seg, err := gbn.rx()
		if err != nil {
			fmt.Println(err)
			return err
		}

		// ack包
		if seg.cmd == cmdAck {
			if seg.seq < gbn.sndBase {
				continue
			}

			log.Println("[D] receive ack: ", seg.seq)
			gbn.locker.Lock()
			// 调整可用窗口
			gbn.availwnd += seg.seq - gbn.sndBase + 1
			// 移动sndbase
			moveSize := seg.seq - gbn.sndBase + 1
			gbn.sndBase = seg.seq + 1
			if moveSize > 0 {
				log.Printf("[D] wnd move %d\n", moveSize)
			}
			// 移动窗口
			if len(gbn.sndbuf) > int(moveSize) {
				gbn.sndbuf = gbn.sndbuf[moveSize:]
			}

			gbn.locker.Unlock()

			// 重置定时器
			gbn.timer.Reset(gbn.rto)
		} else {
			// 丢弃乱序包
			if seg.seq != gbn.expectSeq {
				fmt.Println("drop ", seg.seq, gbn.expectSeq)
				if gbn.expectSeq > 0 {
					// 响应ack
					gbn.tx(segment{
						cmd:   cmdAck,
						seq:   gbn.expectSeq - 1,
						raddr: seg.raddr,
					})
					gbn.lastAck = time.Now()
				}
				continue
			}

			gbn.rcvMu.Lock()
			gbn.rcvbuf.PushBack(seg)
			gbn.rcvMu.Unlock()

			ack := true
			// 延迟ack
			if gbn.delayAck > 0 {
				diff := time.Now().Sub(gbn.lastAck).Milliseconds()
				if diff < gbn.delayAck {
					ack = false
				}
			}

			if ack {
				// 响应ack
				gbn.tx(segment{
					cmd:   cmdAck,
					seq:   seg.seq,
					raddr: seg.raddr,
				})
				gbn.lastAck = time.Now()
			}
			// 通知上层收包
			select {
			case gbn.recvPush <- struct{}{}:
			default:
			}
			gbn.expectSeq += 1
		}
	}
}

func (gbn *Gbn) tx(buf segment) {
	data := make([]byte, 5)
	data[0] = buf.cmd
	binary.BigEndian.PutUint32(data[1:], buf.seq)
	data = append(data, buf.data...)
	if gbn.conn.RemoteAddr() != nil {
		_, err := gbn.conn.Write(data)
		if err != nil {
			fmt.Println(err)
		}
	} else {
		_, err := gbn.conn.WriteTo(data, buf.raddr)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func (gbn *Gbn) rx() (segment, error) {
	buf := make([]byte, gbn.mss)
	nr, raddr, err := gbn.conn.ReadFromUDP(buf)
	if err != nil {
		return segment{}, err
	}

	// ack不携带数据
	if nr == 1 && buf[0] == cmdAck {
		return segment{cmd: cmdAck}, nil
	}

	if nr < overHead {
		return segment{}, fmt.Errorf("invalid overhead")
	}

	seq := binary.BigEndian.Uint32(buf[cmdSize:])
	seg := segment{
		cmd:   buf[0],
		seq:   seq,
		data:  buf[cmdSize+seqSize : nr],
		raddr: raddr,
	}
	return seg, nil
}
