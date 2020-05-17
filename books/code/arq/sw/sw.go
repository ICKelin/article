package sw

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	_ = iota
	cmdPush
	cmdAck
)

var (
	cmdSize    = 1
	seqSize    = 4
	overHead   = cmdSize + seqSize
	defaultRto = time.Millisecond * 1000
	errAgain   = fmt.Errorf("EAGAIN")
)

type segment struct {
	cmd  uint8
	seq  uint32
	data []byte

	raddr *net.UDPAddr
}

type writeReq struct {
	seg segment
	fin chan struct{}
}

type ackReq struct {
	seg segment
	fin chan struct{}
}

type Sw struct {
	conn *net.UDPConn

	// 发送缓冲区，nonbuf
	// 每次只允许发送一个segment
	// 只有上一个segment确认之后，才会发送下一个segment
	sndMu  sync.Mutex
	sndbuf chan *writeReq

	// 接收缓冲区
	// 上层每次从rcvbuf当中取出数据
	rcvMu        sync.Mutex
	rcvPushEvent chan struct{}
	rcvbuf       *list.List

	// 上层交付函数, TODO://
	deliveryFunc func([]byte, *net.UDPAddr)

	// 收到ack 序列号
	recvAck chan *ackReq

	// 当前发送序列号
	// 首次收到确认之后,sndSeq+1
	sndSeq uint32

	// 上次收到的序列号
	// 如果收到的序列号 < latestSeq，程序bug
	// 如果收到的序列号 = latestSeq，视为重传包，需要进行确认
	// 如果收到的序列号 = latestSeq + 1，视为有效包，需要进行确认并往上层投递
	// 如果收到的序列号 > latestSeq + 1，程序bug
	latestSeq uint32

	// mss，超过mss进行分片
	mss int

	// 重传计时时间
	// 默认100ms
	// 每次收到ack之后，调整为 rtt * 0.6 + minrtt * 0.2 + maxrtt * 0.2
	rto time.Duration

	// rtt采样，用于动态调整rto
	// 微妙级别
	rtt    int64
	minrtt int64
	maxrtt int64

	// 超时计时器
	timer *time.Timer
}

func NewSw(conn *net.UDPConn, deliveryFunc func([]byte, *net.UDPAddr)) *Sw {
	sw := &Sw{
		conn:         conn,
		sndbuf:       make(chan *writeReq),
		rcvbuf:       list.New(),
		rcvPushEvent: make(chan struct{}),
		deliveryFunc: deliveryFunc,
		recvAck:      make(chan *ackReq),
		timer:        time.NewTimer(defaultRto),
		mss:          1400,
		rto:          defaultRto,
		latestSeq:    0,
		sndSeq:       1,
		rtt:          0,
		minrtt:       math.MaxInt64,
		maxrtt:       math.MinInt64,
	}
	go sw.read()
	go sw.sendFragment()
	return sw
}

// 阻塞式读
// 如果当前缓冲区没有数据包，阻塞
// 如果有数据包，返回一个segment
func (sw *Sw) RecvFrom() ([]byte, *net.UDPAddr, error) {
	sw.rcvMu.Lock()
	if sw.rcvbuf.Len() > 0 {
		ele := sw.rcvbuf.Front()
		seg := ele.Value.(segment)
		sw.rcvbuf.Remove(ele)
		sw.rcvMu.Unlock()
		return seg.data, seg.raddr, nil
	}
	sw.rcvMu.Unlock()

	<-sw.rcvPushEvent
	sw.rcvMu.Lock()
	if sw.rcvbuf.Len() > 0 {
		ele := sw.rcvbuf.Front()
		seg := ele.Value.(segment)
		sw.rcvbuf.Remove(ele)
		sw.rcvMu.Unlock()
		return seg.data, seg.raddr, nil
	}
	sw.rcvMu.Unlock()

	return nil, nil, errAgain
}

// 非阻塞式读
// 如果当前缓冲区没有数据包，返回EAGAIN
// 如果有数据包，返回一个segment
func (sw *Sw) RecvFromNonBlock() ([]byte, *net.UDPAddr, error) {
	sw.rcvMu.Lock()
	fmt.Println(sw.rcvbuf.Len())
	if sw.rcvbuf.Len() > 0 {
		ele := sw.rcvbuf.Front()
		seg := ele.Value.(segment)
		sw.rcvbuf.Remove(ele)
		sw.rcvMu.Unlock()
		return seg.data, seg.raddr, nil
	}
	sw.rcvMu.Unlock()
	return nil, nil, errAgain
}

func (sw *Sw) Send(data []byte, raddr *net.UDPAddr) {
	sw.sndMu.Lock()
	defer sw.sndMu.Unlock()

	// 分片
	if len(data) > sw.mss {
		log.Println("[D] fragment")
		for c := 0; c < len(data); {
			pos := c + sw.mss
			if pos > len(data) {
				pos = len(data)
			}

			seg := segment{
				cmd:   cmdPush,
				seq:   sw.sndSeq,
				data:  data[c:pos],
				raddr: raddr,
			}

			req := &writeReq{
				seg: seg,
				fin: make(chan struct{}),
			}

			sw.sndbuf <- req
			<-req.fin
		}
	} else {
		seg := segment{
			cmd:   cmdPush,
			seq:   sw.sndSeq,
			data:  data,
			raddr: raddr,
		}
		req := &writeReq{
			seg: seg,
			fin: make(chan struct{}),
		}

		sw.sndbuf <- req
		<-req.fin
	}

}

func (sw *Sw) sendFragment() {
	for req := range sw.sndbuf {
		log.Printf("[D]send fragment: %d, size: %d\n", sw.sndSeq, len(req.seg.data))
		sw.tx(req.seg)

		beg := time.Now()
		sw.timer.Reset(sw.rto)
		for {
			select {
			// 死等ack
			case ack := <-sw.recvAck:
				log.Printf("[D] receive ack for segment: %d\n", sw.sndSeq)
				atomic.AddUint32(&sw.sndSeq, 1)

				rtt := time.Now().Sub(beg).Microseconds()
				sw.rtt = rtt
				if rtt < sw.minrtt {
					sw.minrtt = rtt
				}

				if rtt > sw.maxrtt {
					sw.maxrtt = rtt
				}

				// sw.rto = time.Duration(
				// 	int64(float64(sw.rtt)*0.4+
				// 		float64(sw.minrtt)*0.2+
				// 		float64(sw.maxrtt)*0.4)) * time.Microsecond

				log.Printf("[D] rtt %d minrtt: %d maxrtt: %d, rto: %d\n",
					sw.rtt, sw.minrtt, sw.maxrtt, sw.rto.Microseconds())

				sw.timer.Stop()
				close(ack.fin)
				goto fin

			// 超时重传
			case <-sw.timer.C:
				log.Printf("[D] segment %d timeout, resending\n", sw.sndSeq)
				sw.tx(req.seg)
				sw.timer.Reset(sw.rto)
				log.Println("[D] resend segment ", req.seg.seq)
			}
		}
	fin:
		close(req.fin)
	}
}

func (sw *Sw) read() error {
	for {
		seg, err := sw.rx()
		if err != nil {
			log.Println(err)
			return err
		}

		// ack包
		if seg.cmd == cmdAck {
			// 失序包，在sw协议中基本不会发生失序包
			if seg.seq > sw.sndSeq {
				log.Printf("[FATAL] unorder pkt, curseq %d, ack seq %d\n",
					sw.sndSeq, seg.seq)
				continue
			}

			// 由于重传造成的ack重传
			// 由于数据包已经确认
			// 会出现ack的序列号比当前发送的序列号要小的场景
			if seg.seq < sw.sndSeq {
				log.Printf("[D] receive %d, curSeq: %d\n", seg.seq, sw.sndSeq)
				continue
			}

			req := &ackReq{
				seg: seg,
				fin: make(chan struct{}),
			}
			sw.recvAck <- req
			<-req.fin
		}

		// 数据包
		if seg.cmd == cmdPush {
			// 失序包，在sw协议中基本不会发生失序包
			if seg.seq > sw.latestSeq+1 || seg.seq < sw.latestSeq {
				log.Printf("[FATAL] unorder pkt, expectSeq %d, pkt seq %d\n",
					sw.latestSeq, seg.seq)
				continue
			}

			// 重复包
			// 由于不针对ack进行超时重传机制
			// 当ack丢包时，对方会继续重传，造成重复包
			// 如果不再进行确认，对方可能会一直重传
			if seg.seq == sw.latestSeq {
				log.Printf("[D] duplicate segment %d\n", seg.seq)
				// ack
				sw.tx(segment{
					cmd:   cmdAck,
					seq:   seg.seq,
					raddr: seg.raddr,
				})

				continue
			}

			sw.rcvMu.Lock()
			sw.rcvbuf.PushBack(seg)
			sw.rcvMu.Unlock()
			// ack
			sw.tx(segment{
				cmd:   cmdAck,
				seq:   seg.seq,
				raddr: seg.raddr,
			})

			select {
			case sw.rcvPushEvent <- struct{}{}:
			default:
			}
			log.Printf("[D] receive packet %d, size %d, %s\n", seg.seq, len(seg.data), string(seg.data))
			sw.latestSeq = seg.seq
		}
	}
}

func (sw *Sw) tx(buf segment) {
	data := make([]byte, 5)
	data[0] = buf.cmd
	binary.BigEndian.PutUint32(data[1:], buf.seq)
	data = append(data, buf.data...)
	if sw.conn.RemoteAddr() != nil {
		_, err := sw.conn.Write(data)
		if err != nil {
			log.Println(err)
		}
	} else {
		_, err := sw.conn.WriteTo(data, buf.raddr)
		if err != nil {
			log.Println(err)
		}
	}
}

func (sw *Sw) rx() (segment, error) {
	buf := make([]byte, sw.mss)
	nr, raddr, err := sw.conn.ReadFromUDP(buf)
	if err != nil {
		return segment{}, err
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
