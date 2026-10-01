package udp

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/protocol/udp"
	"github.com/v2fly/v2ray-core/v5/common/session"
	"github.com/v2fly/v2ray-core/v5/common/signal"
	"github.com/v2fly/v2ray-core/v5/common/signal/done"
	"github.com/v2fly/v2ray-core/v5/features/routing"
	"github.com/v2fly/v2ray-core/v5/transport"
)

type ResponseCallback func(ctx context.Context, packet *udp.Packet)

type connEntry struct {
	link   *transport.Link
	timer  signal.ActivityUpdater
	cancel context.CancelFunc
}

type Dispatcher struct {
	sync.RWMutex
	conns      map[net.Destination]*connEntry
	dispatcher routing.Dispatcher
	callback   ResponseCallback
	idle       time.Duration
}

const defaultSplitIdle = 5 * time.Minute

func (v *Dispatcher) Close() error {
	v.Lock()
	entries := make([]*connEntry, 0, len(v.conns))
	for dest, entry := range v.conns {
		entries = append(entries, entry)
		delete(v.conns, dest)
	}
	v.Unlock()
	for _, entry := range entries {
		v.terminateEntry(entry)
	}
	return nil
}

func (v *Dispatcher) terminateEntry(entry *connEntry) {
	if entry == nil {
		return
	}
	if entry.cancel != nil {
		entry.cancel()
	}
	if entry.link != nil {
		common.Close(entry.link.Reader)
		common.Close(entry.link.Writer)
		entry.link = nil
	}
}

func NewSplitDispatcher(dispatcher routing.Dispatcher, callback ResponseCallback) DispatcherI {
	return NewSplitDispatcherWithIdle(dispatcher, callback, defaultSplitIdle)
}

// NewSplitDispatcherWithIdle 为每个目的地出站设置空闲回收时间（TUN 应显著短于默认 5m，避免 ListenPacket 堆积）。
func NewSplitDispatcherWithIdle(dispatcher routing.Dispatcher, callback ResponseCallback, idle time.Duration) DispatcherI {
	if idle <= 0 {
		idle = defaultSplitIdle
	}
	return &Dispatcher{
		conns:      make(map[net.Destination]*connEntry),
		dispatcher: dispatcher,
		callback:   callback,
		idle:       idle,
	}
}

func (v *Dispatcher) RemoveRay(dest net.Destination) {
	v.Lock()
	entry, found := v.conns[dest]
	if found {
		delete(v.conns, dest)
	}
	v.Unlock()
	if found {
		v.terminateEntry(entry)
	}
}

func (v *Dispatcher) getInboundRay(ctx context.Context, dest net.Destination) (*connEntry, error) {
	v.Lock()
	if entry, found := v.conns[dest]; found {
		v.Unlock()
		return entry, nil
	}
	v.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	removeRay := func() {
		v.RemoveRay(dest)
	}
	timer := signal.CancelAfterInactivity(ctx, removeRay, v.idle)
	link, err := v.dispatcher.Dispatch(ctx, dest)
	if err != nil {
		cancel()
		return nil, err
	}
	entry := &connEntry{
		link:   link,
		timer:  timer,
		cancel: cancel,
	}

	v.Lock()
	if existing, found := v.conns[dest]; found {
		v.Unlock()
		cancel()
		common.Close(link.Reader)
		common.Close(link.Writer)
		return existing, nil
	}
	v.conns[dest] = entry
	v.Unlock()
	go handleInput(ctx, entry, dest, v.callback, func() { v.RemoveRay(dest) })
	return entry, nil
}

func (v *Dispatcher) Dispatch(ctx context.Context, destination net.Destination, payload *buf.Buffer) {
	conn, err := v.getInboundRay(ctx, destination)
	if err != nil {
		payload.Release()
		newError("failed to dispatch UDP payload").Base(err).WriteToLog(session.ExportIDToError(ctx))
		return
	}
	outputStream := conn.link.Writer
	if outputStream != nil {
		if err := outputStream.WriteMultiBuffer(buf.MultiBuffer{payload}); err != nil {
			newError("failed to write first UDP payload").Base(err).WriteToLog(session.ExportIDToError(ctx))
			v.RemoveRay(destination)
			return
		}
		conn.timer.Update()
	}
}

func handleInput(ctx context.Context, conn *connEntry, dest net.Destination, callback ResponseCallback, remove func()) {
	defer remove()

	input := conn.link.Reader
	timer := conn.timer

	for {
		mb, err := input.ReadMultiBuffer()
		if err != nil {
			newError("failed to handle UDP input").Base(err).WriteToLog(session.ExportIDToError(ctx))
			return
		}
		if ctx.Err() != nil {
			buf.ReleaseMulti(mb)
			return
		}
		if mb.IsEmpty() {
			// 空读视为出站结束；忙等会在 blackhole/异常链路上打满 CPU
			buf.ReleaseMulti(mb)
			return
		}
		timer.Update()
		for _, b := range mb {
			callback(ctx, &udp.Packet{
				Payload: b,
				Source:  dest,
			})
		}
	}
}

type dispatcherConn struct {
	dispatcher *Dispatcher
	cache      chan *udp.Packet
	done       *done.Instance
}

func DialDispatcher(ctx context.Context, dispatcher routing.Dispatcher) (net.PacketConn, error) {
	c := &dispatcherConn{
		cache: make(chan *udp.Packet, 16),
		done:  done.New(),
	}

	d := NewSplitDispatcher(dispatcher, c.callback)
	c.dispatcher = d.(*Dispatcher)
	return c, nil
}

func (c *dispatcherConn) callback(ctx context.Context, packet *udp.Packet) {
	select {
	case <-c.done.Wait():
		packet.Payload.Release()
		return
	case c.cache <- packet:
	default:
		packet.Payload.Release()
		return
	}
}

func (c *dispatcherConn) ReadFrom(p []byte) (int, net.Addr, error) {
	select {
	case <-c.done.Wait():
		return 0, nil, io.EOF
	case packet := <-c.cache:
		n := copy(p, packet.Payload.Bytes())
		return n, &net.UDPAddr{
			IP:   packet.Source.Address.IP(),
			Port: int(packet.Source.Port),
		}, nil
	}
}

func (c *dispatcherConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	buffer := buf.New()
	raw := buffer.Extend(buf.Size)
	n := copy(raw, p)
	buffer.Resize(0, int32(n))

	ctx := context.Background()
	c.dispatcher.Dispatch(ctx, net.DestinationFromAddr(addr), buffer)
	return n, nil
}

func (c *dispatcherConn) Close() error {
	return c.done.Close()
}

func (c *dispatcherConn) LocalAddr() net.Addr {
	return &net.UDPAddr{
		IP:   []byte{0, 0, 0, 0},
		Port: 0,
	}
}

func (c *dispatcherConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *dispatcherConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *dispatcherConn) SetWriteDeadline(t time.Time) error {
	return nil
}
