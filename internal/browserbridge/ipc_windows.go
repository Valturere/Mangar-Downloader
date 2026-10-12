package browserbridge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"sync"
	"time"
	"unsafe"
)

type Identity struct{ Pipe, Mutex, SDDL string }

func UserIdentity(scope string) (Identity, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return Identity{}, err
	}
	// Scope isolates development/test profiles from the user's running app.
	hash := sha256.Sum256([]byte(scope))
	name := fmt.Sprintf("YTDLPGUIMangar-%s-%x", u.User.Sid.String(), hash[:8])
	return Identity{Pipe: `\\.\pipe\` + name, Mutex: `Global\` + name, SDDL: "D:P(A;;GA;;;" + u.User.Sid.String() + ")"}, nil
}
func Claim(id Identity) (windows.Handle, bool, error) {
	sd, err := windows.SecurityDescriptorFromString(id.SDDL)
	if err != nil {
		return 0, false, err
	}
	p, err := windows.UTF16PtrFromString(id.Mutex)
	if err != nil {
		return 0, false, err
	}
	h, err := windows.CreateMutex(&windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}, false, p)
	if err == windows.ERROR_ALREADY_EXISTS {
		return h, false, nil
	}
	return h, err == nil, err
}

type Server struct {
	once     sync.Once
	listener net.Listener
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	gate     chan struct{}
}

func Listen(id Identity, handler func(context.Context, Message) Reply) (*Server, error) {
	l, err := winio.ListenPipe(id.Pipe, &winio.PipeConfig{SecurityDescriptor: id.SDDL, InputBufferSize: 65536, OutputBufferSize: 65536})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{listener: l, cancel: cancel, gate: make(chan struct{}, 16)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, e := l.Accept()
			if e != nil {
				return
			}
			select {
			case s.gate <- struct{}{}:
			default:
				conn.Close()
				continue
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer func() { <-s.gate }()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(12 * time.Second))
				stop := context.AfterFunc(ctx, func() { conn.Close() })
				defer stop()
				var m Message
				if e := ReadFrame(conn, &m); e != nil {
					WriteFrame(conn, Failure("invalid"))
					return
				}
				valid, e := Validate(m, time.Now())
				if e != nil {
					code := "invalid"
					if e.Error() == "authorization" {
						code = "authorization"
					}
					WriteFrame(conn, Failure(code))
					return
				}
				WriteFrame(conn, handler(ctx, valid))
			}()
		}
	}()
	return s, nil
}
func (s *Server) Close() { s.once.Do(func() { s.cancel(); s.listener.Close(); s.wg.Wait() }) }
func Send(ctx context.Context, id Identity, m Message) (Reply, error) {
	conn, err := winio.DialPipeContext(ctx, id.Pipe)
	if err != nil {
		return Reply{}, err
	}
	defer conn.Close()
	deadline := time.Now().Add(12 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if err = WriteFrame(conn, m); err != nil {
		return Reply{}, err
	}
	var reply Reply
	err = ReadFrame(conn, &reply)
	return reply, err
}

// Only absent endpoint is retried. A message is never resent after delivery.
func AwaitSend(ctx context.Context, id Identity, m Message) (Reply, error) {
	for {
		r, e := Send(ctx, id, m)
		if e == nil || !os.IsNotExist(e) {
			return r, e
		}
		select {
		case <-ctx.Done():
			return Reply{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
