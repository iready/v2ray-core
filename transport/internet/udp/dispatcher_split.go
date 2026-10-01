package udp

import (
	"context"
	"io"
	"log"
	"runtime/debug"
	"sync"
	"sync/atomic"
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
	link        atomic.Pointer[transport.Link]
	timer       *signal.ActivityTimer
	cancel      context.CancelFunc
	stopContext func() bool
}

type Dispatcher struct {
	sync.RWMutex
	conns      map[net.Destination]*connEntry
	dispatcher routing.Dispatcher
	callback   ResponseCallback
	idle       time.Duration
	closed     bool
}

const defaultSplitIdle = 5 * time.Minute

var Created, Closed, TimedOut atomic.Uint64

func (v *Dispatcher) Close() error {
	v.Lock()
	v.closed = true
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
	if entry.stopContext != nil {
		entry.stopContext()
	}
	if entry.timer != nil {
		_ = entry.timer.Close()
	}
	if entry.cancel != nil {
		entry.cancel()
	}
	if link := entry.link.Swap(nil); link != nil {
		Closed.Add(1)
		common.Interrupt(link.Reader)
		common.Close(link.Writer)
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

// removeEntry never lets an old reader/timeout remove a replacement connection.
func (v *Dispatcher) removeEntry(dest net.Destination, entry *connEntry) {
	v.Lock()
	if v.conns[dest] == entry {
		delete(v.conns, dest)
	}
	v.Unlock()
	v.terminateEntry(entry)
}

func (v *Dispatcher) getInboundRay(ctx context.Context, dest net.Destination) (*connEntry, error) {
	v.Lock()
	if v.closed || ctx.Err() != nil {
		v.Unlock()
		return nil, io.ErrClosedPipe
	}
	if entry, found := v.conns[dest]; found {
		v.Unlock()
		return entry, nil
	}
	v.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	link, err := v.dispatcher.Dispatch(ctx, dest)
	if err != nil {
		cancel()
		return nil, err
	}
	if link == nil || link.Reader == nil || link.Writer == nil {
		cancel()
		if link != nil {
			common.Interrupt(link.Reader)
			common.Close(link.Writer)
		}
		return nil, io.ErrClosedPipe
	}
	entry := &connEntry{cancel: cancel}
	entry.link.Store(link)
	Created.Add(1)
	remove := func() { v.removeEntry(dest, entry) }
	// Publish only after lifecycle hooks are fully initialized. Callbacks take v.Lock.
	v.Lock()
	if v.closed || ctx.Err() != nil {
		v.Unlock()
		v.terminateEntry(entry)
		return nil, io.ErrClosedPipe
	}
	if existing, found := v.conns[dest]; found {
		v.Unlock()
		v.terminateEntry(entry)
		return existing, nil
	}
	entry.timer = signal.CancelAfterInactivity(ctx, func() { TimedOut.Add(1); remove() }, v.idle)
	entry.stopContext = context.AfterFunc(ctx, remove)
	v.conns[dest] = entry
	v.Unlock()
	go handleInput(ctx, link.Reader, entry.timer, dest, v.callback, remove)
	return entry, nil
}

func (v *Dispatcher) Dispatch(ctx context.Context, destination net.Destination, payload *buf.Buffer) {
	conn, err := v.getInboundRay(ctx, destination)
	if err != nil {
		payload.Release()
		newError("failed to dispatch UDP payload").Base(err).WriteToLog(session.ExportIDToError(ctx))
		return
	}
	link := conn.link.Load()
	if link == nil || link.Writer == nil {
		payload.Release()
		return
	}
	outputStream := link.Writer
	if outputStream != nil {
		if err := outputStream.WriteMultiBuffer(buf.MultiBuffer{payload}); err != nil {
			newError("failed to write first UDP payload").Base(err).WriteToLog(session.ExportIDToError(ctx))
			v.removeEntry(destination, conn)
			return
		}
		conn.timer.Update()
	}
}

func handleInput(ctx context.Context, input buf.Reader, timer signal.ActivityUpdater, dest net.Destination, callback ResponseCallback, remove func()) {
	defer remove()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic udp %s: %v\n%s", dest, r, debug.Stack())
		}
	}()
	if input == nil {
		return
	}

	for {
		mb, err := input.ReadMultiBuffer()
		if err != nil {
			buf.ReleaseMulti(mb)
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
	mu          sync.Mutex
	closeOnce   sync.Once
	dispatcher  *Dispatcher
	cache       chan *udp.Packet
	done        *done.Instance
	ctx         context.Context
	stopContext func() bool
}

func DialDispatcher(ctx context.Context, dispatcher routing.Dispatcher) (net.PacketConn, error) {
	c := &dispatcherConn{
		cache: make(chan *udp.Packet, 16),
		done:  done.New(),
		ctx:   ctx,
	}

	d := NewSplitDispatcher(dispatcher, c.callback)
	c.dispatcher = d.(*Dispatcher)
	c.mu.Lock()
	c.stopContext = context.AfterFunc(ctx, func() { _ = c.Close() })
	c.mu.Unlock()
	return c, nil
}

func (c *dispatcherConn) callback(ctx context.Context, packet *udp.Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done.Wait():
		packet.Payload.Release()
		return
	default:
	}
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
		defer packet.Payload.Release()
		n := copy(p, packet.Payload.Bytes())
		return n, &net.UDPAddr{
			IP:   packet.Source.Address.IP(),
			Port: int(packet.Source.Port),
		}, nil
	}
}

func (c *dispatcherConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	buffer := buf.NewWithSize(int32(len(p)))
	n, err := buffer.Write(p)
	if err != nil {
		buffer.Release()
		return 0, err
	}

	select {
	case <-c.done.Wait():
		buffer.Release()
		return 0, io.ErrClosedPipe
	default:
	}
	c.dispatcher.Dispatch(c.ctx, net.DestinationFromAddr(addr), buffer)
	return n, nil
}

func (c *dispatcherConn) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		if c.stopContext != nil {
			c.stopContext()
		}
		_ = c.done.Close()
		for draining := true; draining; {
			select {
			case p := <-c.cache:
				p.Payload.Release()
			default:
				draining = false
			}
		}
		c.mu.Unlock()
		_ = c.dispatcher.Close()
	})
	return nil
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
