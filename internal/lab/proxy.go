package lab

import (
	"context"
	"net"
	"net/url"
	"sync"
	"testing"
)

// A Proxy forwards TCP connections to a server, such as the lab's
// PostgreSQL, until it's cut off. Then it forwards nothing more, either
// way, and answers no new connection, but keeps every connection open, as a
// hung host's kernel does, so that the server ends its sessions on its own
// timeouts alone (ADR-0047). Once restored, it forwards what it held back,
// as a host that resumes does.
type Proxy struct {
	ln     net.Listener
	target string
	// ctx ends as the proxy closes.
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	// mu guards cut, restored and conns. restored is closed as the proxy is
	// restored, and conns are the connections that it holds, both ways.
	mu       sync.Mutex
	cut      bool
	restored chan struct{}
	conns    []net.Conn
	wg       sync.WaitGroup
}

// NewProxy starts a proxy to target, a host and port, on a free port of
// 127.0.0.1. It closes as the test ends.
func NewProxy(t testing.TB, target string) *Proxy {
	t.Helper()
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	p := &Proxy{ln: ln, target: target, ctx: ctx, cancel: cancel}
	p.wg.Go(p.accept)
	t.Cleanup(p.Close)
	return p
}

// Proxied returns rawURL, a URL such as a database's, with its host through
// a new proxy, which it returns too.
func Proxied(t testing.TB, rawURL string) (string, *Proxy) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	p := NewProxy(t, u.Host)
	u.Host = p.Addr()
	return u.String(), p
}

// Addr returns the proxy's address.
func (p *Proxy) Addr() string { return p.ln.Addr().String() }

// Cut cuts the proxy off.
func (p *Proxy) Cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.cut {
		p.cut, p.restored = true, make(chan struct{})
	}
}

// Restore has the proxy forward again, once it's cut off.
func (p *Proxy) Restore() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cut {
		p.cut = false
		close(p.restored)
	}
}

// IsCut reports whether the proxy is cut off.
func (p *Proxy) IsCut() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cut
}

// Close closes the proxy, and every connection that it holds.
func (p *Proxy) Close() {
	p.closeOnce.Do(func() {
		p.cancel()
		_ = p.ln.Close()
		p.mu.Lock()
		for _, c := range p.conns {
			_ = c.Close()
		}
		p.conns = nil
		p.mu.Unlock()
		p.wg.Wait()
	})
}

// forwards waits while the proxy is cut off, and reports whether it forwards
// again, or has closed.
func (p *Proxy) forwards() bool {
	for {
		p.mu.Lock()
		cut, restored := p.cut, p.restored
		p.mu.Unlock()
		if !cut {
			return p.ctx.Err() == nil
		}
		select {
		case <-restored:
		case <-p.ctx.Done():
			return false
		}
	}
}

// keep holds c, for Close to close, and reports whether it does: if the
// proxy has closed, it closes c instead.
func (p *Proxy) keep(c net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		_ = c.Close()
		return false
	}
	p.conns = append(p.conns, c)
	return true
}

// accept accepts connections until the proxy closes, and forwards each,
// once the proxy forwards.
func (p *Proxy) accept() {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		if p.keep(c) {
			p.wg.Go(func() {
				if p.forwards() {
					p.forward(c)
				}
			})
		}
	}
}

// forward connects c to the target, and copies between them, both ways.
func (p *Proxy) forward(c net.Conn) {
	var d net.Dialer
	up, err := d.DialContext(p.ctx, "tcp", p.target)
	if err != nil || !p.keep(up) {
		_ = c.Close()
		return
	}
	p.wg.Go(func() { p.pipe(up, c) })
	p.pipe(c, up)
}

// pipe copies from src to dst until either fails, then closes both, so that
// a close at one end reaches the other. While the proxy is cut off, it holds
// what it read, and the end of src, until the proxy is restored, or closes.
func (p *Proxy) pipe(dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if !p.forwards() {
			return
		}
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				err = werr
			}
		}
		if err != nil {
			_ = src.Close()
			_ = dst.Close()
			return
		}
	}
}
