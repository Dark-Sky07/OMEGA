package sub

import (
	"bufio"
	"crypto/tls"
	"net"
	"time"
)

// mixedTLSListener supports plain HTTP and TLS on the same subscription port
// when no panel-wide subscription certificate is configured. TLS connections
// are selected by SNI; ordinary HTTP behavior remains unchanged.
type mixedTLSListener struct {
	net.Listener
	config   *tls.Config
	incoming chan mixedListenerResult
	done     chan struct{}
	workers  chan struct{}
}

type mixedListenerResult struct {
	conn net.Conn
	err  error
}

func newMixedTLSListener(listener net.Listener, config *tls.Config) *mixedTLSListener {
	l := &mixedTLSListener{
		Listener: listener,
		config:   config,
		incoming: make(chan mixedListenerResult),
		done:     make(chan struct{}),
		workers:  make(chan struct{}, 128),
	}
	go l.acceptLoop()
	return l
}

func (l *mixedTLSListener) acceptLoop() {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			select {
			case l.incoming <- mixedListenerResult{err: err}:
			case <-l.done:
			}
			return
		}
		select {
		case l.workers <- struct{}{}:
		case <-l.done:
			_ = conn.Close()
			return
		}
		go func(raw net.Conn) {
			defer func() { <-l.workers }()
			_ = raw.SetReadDeadline(time.Now().Add(5 * time.Second))
			reader := bufio.NewReader(raw)
			first, err := reader.Peek(1)
			_ = raw.SetReadDeadline(time.Time{})
			if err != nil {
				_ = raw.Close()
				return
			}
			buffered := &bufferedNetConn{Conn: raw, reader: reader}
			var client net.Conn = buffered
			if first[0] == 0x16 { // TLS handshake record
				client = tls.Server(buffered, l.config)
			}
			select {
			case l.incoming <- mixedListenerResult{conn: client}:
			case <-l.done:
				_ = client.Close()
			}
		}(conn)
	}
}

func (l *mixedTLSListener) Accept() (net.Conn, error) {
	select {
	case result := <-l.incoming:
		return result.conn, result.err
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *mixedTLSListener) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return l.Listener.Close()
}

type bufferedNetConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedNetConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
