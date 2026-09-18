package l2tp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

var (
	gracefulStopTimeout = 7 * time.Second
	forceStopTimeout    = 3 * time.Second
)

type procLogWriter struct {
	mu       sync.Mutex
	label    string
	buf      string
	lastLine string
}

func (w *procLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf += string(p)
	for {
		i := strings.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(strings.TrimRight(w.buf[:i], "\r"))
		w.buf = w.buf[i+1:]
		if line != "" {
			w.lastLine = line
			logger.Infof("l2tp: %s | %s", w.label, line)
		}
	}
	return len(p), nil
}

func (w *procLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	line := strings.TrimSpace(strings.TrimRight(w.buf, "\r"))
	if line != "" {
		w.lastLine = line
		logger.Infof("l2tp: %s | %s", w.label, line)
	}
	w.buf = ""
}

func (w *procLogWriter) LastLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastLine
}

// Process supervises the strongSwan starter and xl2tpd foreground processes.
type Process struct {
	id         int
	configDir  string
	ipsecCmd   *exec.Cmd
	xl2tpdCmd  *exec.Cmd
	ipsecDone  chan struct{}
	xl2tpdDone chan struct{}
	ipsecLog   *procLogWriter
	xl2tpdLog  *procLogWriter
	exitErr    error
	intentional atomic.Bool
	mu         sync.Mutex
}

func newProcess(id int) *Process {
	return &Process{
		id:        id,
		configDir: dataDirForID(id),
		ipsecLog:  &procLogWriter{label: "strongSwan"},
		xl2tpdLog: &procLogWriter{label: "xl2tpd"},
	}
}

func strongSwanPath() string {
	for _, name := range []string{"ipsec", "strongswan"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func xl2tpdPath() string {
	path, _ := exec.LookPath("xl2tpd")
	return path
}

func stopPIDFile(path, expected string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		return
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || !strings.Contains(strings.ToLower(string(cmdline)), expected) {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(gracefulStopTimeout)
	for time.Now().Before(deadline) {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = process.Kill()
}

// stopOrphan stops a daemon group left behind by a previous panel process.
// The manager is in-memory, so PID/config discovery is required after a host
// or panel restart; without it an old xl2tpd would keep UDP 1701 occupied and
// the old strongSwan starter would own the IKE sockets.
func stopOrphan(id int) {
	if runtime.GOOS == "windows" {
		return
	}
	stopPIDFile(xl2tpdPIDPath(id), "xl2tpd")
	if ipsec := strongSwanPath(); ipsec != "" {
		ctx, cancel := context.WithTimeout(context.Background(), gracefulStopTimeout)
		cmd := exec.CommandContext(ctx, ipsec, "stop")
		cmd.Env = append(os.Environ(),
			"IPSEC_CONFDIR="+dataDirForID(id),
			"IPSEC_PIDDIR="+dataDirForID(id),
		)
		_ = cmd.Run()
		cancel()
	}
}

// Available reports whether both daemon entry points are installed.
func Available() bool {
	return strongSwanPath() != "" && xl2tpdPath() != ""
}

func (p *Process) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return commandRunning(p.ipsecCmd, p.ipsecDone) && commandRunning(p.xl2tpdCmd, p.xl2tpdDone)
}

func commandRunning(cmd *exec.Cmd, done chan struct{}) bool {
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		return false
	}
	if done == nil {
		return true
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

func (p *Process) GetResult() string {
	if line := p.xl2tpdLog.LastLine(); line != "" {
		return line
	}
	if line := p.ipsecLog.LastLine(); line != "" {
		return line
	}
	p.mu.Lock()
	err := p.exitErr
	p.mu.Unlock()
	if err != nil {
		return err.Error()
	}
	return ""
}

func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if commandRunning(p.ipsecCmd, p.ipsecDone) || commandRunning(p.xl2tpdCmd, p.xl2tpdDone) {
		return errors.New("l2tp daemons are already running")
	}
	ipsec := strongSwanPath()
	xl2tpd := xl2tpdPath()
	if ipsec == "" {
		return errors.New("strongSwan ipsec command not found")
	}
	if xl2tpd == "" {
		return errors.New("xl2tpd command not found")
	}

	// IPSEC_CONFDIR is honored by the upstream ipsec wrapper and keeps this
	// singleton's legacy stroke configuration out of /etc/ipsec.conf. PID files
	// are also redirected so a panel restart can identify its own processes.
	ipsecCmd := exec.Command(ipsec, "start", "--nofork")
	ipsecCmd.Env = append(os.Environ(),
		"IPSEC_CONFDIR="+p.configDir,
		"IPSEC_PIDDIR="+p.configDir,
	)
	ipsecCmd.Stdout = p.ipsecLog
	ipsecCmd.Stderr = p.ipsecLog
	if err := ipsecCmd.Start(); err != nil {
		return fmt.Errorf("start strongSwan: %w", err)
	}
	p.ipsecCmd = ipsecCmd
	p.ipsecDone = make(chan struct{})
	p.exitErr = nil
	p.intentional.Store(false)
	go p.wait(ipsecCmd, p.ipsecDone, p.ipsecLog, "strongSwan")

	xl2tpdCmd := exec.Command(xl2tpd, "-D", "-c", xl2tpdConfigPath(p.id), "-p", xl2tpdPIDPath(p.id))
	xl2tpdCmd.Stdout = p.xl2tpdLog
	xl2tpdCmd.Stderr = p.xl2tpdLog
	if err := xl2tpdCmd.Start(); err != nil {
		p.intentional.Store(true)
		_ = ipsecCmd.Process.Signal(syscall.SIGTERM)
		return fmt.Errorf("start xl2tpd: %w", err)
	}
	p.xl2tpdCmd = xl2tpdCmd
	p.xl2tpdDone = make(chan struct{})
	go p.wait(xl2tpdCmd, p.xl2tpdDone, p.xl2tpdLog, "xl2tpd")
	return nil
}

func (p *Process) wait(cmd *exec.Cmd, done chan struct{}, writer *procLogWriter, name string) {
	err := cmd.Wait()
	writer.Flush()
	if err != nil && !p.intentional.Load() {
		p.mu.Lock()
		p.exitErr = fmt.Errorf("%s exited: %w", name, err)
		p.mu.Unlock()
		logger.Errorf("l2tp: %s exited: %v", name, err)
	}
	close(done)
}

func (p *Process) Stop() error {
	p.mu.Lock()
	p.intentional.Store(true)
	ipsecCmd, ipsecDone := p.ipsecCmd, p.ipsecDone
	xl2tpdCmd, xl2tpdDone := p.xl2tpdCmd, p.xl2tpdDone
	p.mu.Unlock()

	if runtime.GOOS == "windows" {
		if xl2tpdCmd != nil && xl2tpdCmd.Process != nil {
			_ = xl2tpdCmd.Process.Kill()
		}
		if ipsecCmd != nil && ipsecCmd.Process != nil {
			_ = ipsecCmd.Process.Kill()
		}
		return nil
	}
	if xl2tpdCmd != nil && xl2tpdCmd.Process != nil {
		_ = xl2tpdCmd.Process.Signal(syscall.SIGTERM)
	}
	if ipsecCmd != nil && ipsecCmd.Process != nil {
		_ = ipsecCmd.Process.Signal(syscall.SIGTERM)
	}
	waitForDone(xl2tpdDone, gracefulStopTimeout)
	waitForDone(ipsecDone, gracefulStopTimeout)

	p.mu.Lock()
	stillXL2TPD := commandRunning(p.xl2tpdCmd, p.xl2tpdDone)
	stillIPsec := commandRunning(p.ipsecCmd, p.ipsecDone)
	if stillXL2TPD && p.xl2tpdCmd != nil && p.xl2tpdCmd.Process != nil {
		_ = p.xl2tpdCmd.Process.Kill()
	}
	if stillIPsec && p.ipsecCmd != nil && p.ipsecCmd.Process != nil {
		_ = p.ipsecCmd.Process.Kill()
	}
	p.mu.Unlock()
	waitForDone(xl2tpdDone, forceStopTimeout)
	waitForDone(ipsecDone, forceStopTimeout)
	return nil
}

func waitForDone(done chan struct{}, timeout time.Duration) {
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
